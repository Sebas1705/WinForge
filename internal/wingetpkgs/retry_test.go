package wingetpkgs_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/wingetpkgs"
)

// acmeServer serves a valid Acme.App package, but answers the version
// listing with fail(n) for the first requests (n counts from 1) until fail
// returns false.
func acmeServer(t *testing.T, fail func(w http.ResponseWriter, n int64) bool) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/microsoft/winget-pkgs/contents/manifests/a/Acme/App", func(w http.ResponseWriter, r *http.Request) {
		if fail(w, calls.Add(1)) {
			return
		}
		w.Write([]byte(`[{"name":"1.0.0","type":"dir"}]`))
	})
	base := "/microsoft/winget-pkgs/master/manifests/a/Acme/App/1.0.0/Acme.App"
	mux.HandleFunc(base+".installer.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("InstallerUrl: https://acme.example/a.exe\nInstallerType: exe\n"))
	})
	mux.HandleFunc(base+".locale.en-US.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("Publisher: Acme\n"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &calls
}

func client(srv *httptest.Server, maxWait time.Duration) *wingetpkgs.Client {
	return &wingetpkgs.Client{HTTP: srv.Client(), APIURL: srv.URL, RawURL: srv.URL,
		BaseDelay: 5 * time.Millisecond, MaxWait: maxWait}
}

func TestRetriesTooManyRequestsThenSucceeds(t *testing.T) {
	srv, calls := acmeServer(t, func(w http.ResponseWriter, n int64) bool {
		if n <= 2 {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return true
		}
		return false
	})
	p, err := client(srv, time.Minute).Package(context.Background(), "Acme.App")
	if err != nil || p.Publisher != "Acme" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if calls.Load() != 3 {
		t.Fatalf("version listing requested %d times, want 3", calls.Load())
	}
}

func TestRetriesSecondaryRateLimit(t *testing.T) {
	srv, _ := acmeServer(t, func(w http.ResponseWriter, n int64) bool {
		if n == 1 {
			http.Error(w, `{"message":"You have exceeded a secondary rate limit"}`, http.StatusForbidden)
			return true
		}
		return false
	})
	if _, err := client(srv, time.Minute).Package(context.Background(), "Acme.App"); err != nil {
		t.Fatal(err)
	}
}

func TestRetriesServerErrors(t *testing.T) {
	srv, _ := acmeServer(t, func(w http.ResponseWriter, n int64) bool {
		if n == 1 {
			http.Error(w, "boom", http.StatusBadGateway)
			return true
		}
		return false
	})
	if _, err := client(srv, time.Minute).Package(context.Background(), "Acme.App"); err != nil {
		t.Fatal(err)
	}
}

func TestPermanentRateLimitIsRateLimitError(t *testing.T) {
	srv, calls := acmeServer(t, func(w http.ResponseWriter, _ int64) bool {
		http.Error(w, "gitmon refuses to schedule us: too-many-processes", http.StatusTooManyRequests)
		return true
	})
	start := time.Now()
	_, err := client(srv, 200*time.Millisecond).Package(context.Background(), "Acme.App")
	if !errors.Is(err, wingetpkgs.ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	var rl *wingetpkgs.RateLimitError
	if !errors.As(err, &rl) || rl.Status != http.StatusTooManyRequests || !strings.Contains(rl.Body, "too-many-processes") {
		t.Fatalf("err = %#v", err)
	}
	if calls.Load() < 2 {
		t.Fatalf("gave up without retrying (%d calls)", calls.Load())
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("waited %s, beyond the wait budget", d)
	}
}

func TestNotFoundIsNotRateLimited(t *testing.T) {
	srv, calls := acmeServer(t, func(w http.ResponseWriter, _ int64) bool {
		http.NotFound(w, nil)
		return true
	})
	_, err := client(srv, time.Minute).Package(context.Background(), "Acme.App")
	if err == nil || errors.Is(err, wingetpkgs.ErrRateLimited) || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want a not-found failure", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("404 was retried (%d calls)", calls.Load())
	}
}

func TestPlainForbiddenIsNotRetried(t *testing.T) {
	srv, calls := acmeServer(t, func(w http.ResponseWriter, _ int64) bool {
		http.Error(w, `{"message":"Resource not accessible"}`, http.StatusForbidden)
		return true
	})
	_, err := client(srv, time.Minute).Package(context.Background(), "Acme.App")
	if err == nil || errors.Is(err, wingetpkgs.ErrRateLimited) || calls.Load() != 1 {
		t.Fatalf("err = %v after %d calls", err, calls.Load())
	}
}

func TestHonoursRetryAfter(t *testing.T) {
	var first time.Time
	var gap time.Duration
	srv, _ := acmeServer(t, func(w http.ResponseWriter, n int64) bool {
		if n == 1 {
			first = time.Now()
			w.Header().Set("Retry-After", "1")
			http.Error(w, "wait", http.StatusTooManyRequests)
			return true
		}
		if n == 2 {
			gap = time.Since(first)
		}
		return false
	})
	if _, err := client(srv, time.Minute).Package(context.Background(), "Acme.App"); err != nil {
		t.Fatal(err)
	}
	// The backoff alone (5ms base) would retry almost at once.
	if gap < 900*time.Millisecond {
		t.Fatalf("retried after %s, Retry-After asked for 1s", gap)
	}
}

func TestRetryAfterBeyondBudgetGivesUpAtOnce(t *testing.T) {
	srv, calls := acmeServer(t, func(w http.ResponseWriter, _ int64) bool {
		w.Header().Set("Retry-After", "3600")
		http.Error(w, "wait an hour", http.StatusTooManyRequests)
		return true
	})
	start := time.Now()
	_, err := client(srv, 2*time.Minute).Package(context.Background(), "Acme.App")
	if !errors.Is(err, wingetpkgs.ErrRateLimited) || calls.Load() != 1 || time.Since(start) > 5*time.Second {
		t.Fatalf("err = %v after %d calls and %s", err, calls.Load(), time.Since(start))
	}
}

func TestTokenSentToAPIAndRaw(t *testing.T) {
	var missing atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t0k" {
			missing.Add(1)
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/"):
			w.Write([]byte(`[{"name":"1.0.0","type":"dir"}]`))
		case strings.HasSuffix(r.URL.Path, ".installer.yaml"):
			w.Write([]byte("InstallerUrl: https://acme.example/a.exe\n"))
		case strings.HasSuffix(r.URL.Path, ".locale.en-US.yaml"):
			w.Write([]byte("Publisher: Acme\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := client(srv, time.Minute)
	c.Token = "t0k"
	if _, err := c.Package(context.Background(), "Acme.App"); err != nil {
		t.Fatal(err)
	}
	if missing.Load() != 0 {
		t.Fatalf("%d requests without the token", missing.Load())
	}
}

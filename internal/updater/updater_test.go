package updater_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sebas1705/WinForge/internal/updater"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		cur, latest string
		want        bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"0.1.0", "v0.1.1", true},
		{"v1.9.0", "v1.10.0", true}, // numeric, not lexical
		{"v0.2.0", "v0.2.0", false},
		{"v0.3.0", "v0.2.9", false},
		{"v0.2.0-rc1", "v0.2.0", true},
		{"v0.2.0", "v0.2.0-rc1", false},
		{"dev", "v9.0.0", false},
		{"", "v9.0.0", false},
		{"v0.1.0", "nightly", false},
	}
	for _, c := range cases {
		if got := updater.Newer(c.cur, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.cur, c.latest, got, c.want)
		}
	}
}

// server serves a fake release whose installer bytes are payload and whose
// SHA256SUMS claims sum for it.
func server(t *testing.T, payload []byte, sums string, withSums bool) (*updater.Updater, func()) {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		assets := fmt.Sprintf(`{"name":"WinForge-v1.0.0-windows-installer.exe","browser_download_url":"%s/dl/installer"}`, srv.URL)
		if withSums {
			assets += fmt.Sprintf(`,{"name":"SHA256SUMS","browser_download_url":"%s/dl/sums"}`, srv.URL)
		}
		fmt.Fprintf(w, `{"tag_name":"v1.0.0","html_url":"https://x","body":"notes","assets":[%s]}`, assets)
	})
	mux.HandleFunc("/dl/installer", func(w http.ResponseWriter, _ *http.Request) { w.Write(payload) })
	mux.HandleFunc("/dl/sums", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, sums) })
	srv = httptest.NewServer(mux)
	return &updater.Updater{Repo: "o/r", APIURL: srv.URL, Client: srv.Client(), InstallerSuffix: "-windows-installer.exe"}, srv.Close
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestLatestAndVerifiedDownload(t *testing.T) {
	payload := []byte("installer-bytes")
	u, stop := server(t, payload, sum(payload)+"  WinForge-v1.0.0-windows-installer.exe\n", true)
	defer stop()

	rel, err := u.Latest(context.Background())
	if err != nil || rel.Tag != "v1.0.0" || rel.Notes != "notes" {
		t.Fatalf("%+v %v", rel, err)
	}
	var last int64
	path, err := u.Download(context.Background(), rel, t.TempDir(), func(done, _ int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != string(payload) || last != int64(len(payload)) {
		t.Fatalf("content %q progress %d", b, last)
	}
}

func TestDownloadRefusesTamperedOrUnverifiableInstallers(t *testing.T) {
	payload := []byte("installer-bytes")
	good := "WinForge-v1.0.0-windows-installer.exe"
	cases := map[string]struct {
		sums     string
		withSums bool
	}{
		"hash mismatch": {sum([]byte("other")) + "  " + good + "\n", true},
		"no entry":      {sum(payload) + "  something-else.exe\n", true},
		"no sums asset": {"", false},
		"garbled sums":  {"not a checksum file", true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			u, stop := server(t, payload, c.sums, c.withSums)
			defer stop()
			rel, _ := u.Latest(context.Background())
			dir := t.TempDir()
			if _, err := u.Download(context.Background(), rel, dir, nil); err == nil {
				t.Fatal("expected an error")
			}
			if left, _ := filepath.Glob(filepath.Join(dir, "*")); len(left) != 0 {
				t.Fatalf("unverified file left behind: %v", left)
			}
		})
	}
}

func TestDownloadRejectsForeignHosts(t *testing.T) {
	u := updater.New()
	rel := &updater.Release{Tag: "v1", Assets: map[string]string{
		"WinForge-v1-windows-installer.exe": "https://evil.example/x.exe",
		"SHA256SUMS":                        "https://evil.example/sums",
	}}
	if _, err := u.Download(context.Background(), rel, t.TempDir(), nil); err == nil {
		t.Fatal("assets outside the repository's release downloads must be refused")
	}
}

func TestLatestWithoutReleases(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	u := &updater.Updater{Repo: "o/r", APIURL: srv.URL, Client: srv.Client()}
	if _, err := u.Latest(context.Background()); err == nil {
		t.Fatal("expected an error for a repository without releases")
	}
}

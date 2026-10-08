package games_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/games"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// serve returns an https test server that answers every request with body.
func serve(t *testing.T, body []byte) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(srv.Close)
	return srv, srv.Client()
}

func game(srv *httptest.Server, body []byte, kind string) catalog.Game {
	return catalog.Game{ID: "demo", Name: "Demo", System: "scumm", URL: srv.URL + "/demo.zip", SHA256: sum(body), Size: int64(len(body)), Kind: kind}
}

func TestInstallVerifiesExtractsAndLeavesAReceipt(t *testing.T) {
	z := makeZip(t, map[string]string{"demo/data.bin": "DATA", "demo/readme.txt": "hi", "top.txt": "T"})
	srv, client := serve(t, z)
	g := game(srv, z, "zip")
	g.Entry = "demo/data.bin"
	root := t.TempDir()

	var last int64
	r, err := games.Install(context.Background(), client, root, g, func(done, total int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "demo" || r.Files != 3 || last != int64(len(z)) {
		t.Fatalf("%+v progress=%d", r, last)
	}
	if b, _ := os.ReadFile(games.EntryPath(root, g)); string(b) != "DATA" {
		t.Fatalf("entry: %q", b)
	}
	if st := games.State(root, []catalog.Game{g}); len(st) != 1 || st["demo"].SHA256 != g.SHA256 {
		t.Fatalf("state: %+v", st)
	}
	// Nothing but the game's folder is left in the games folder.
	left, _ := os.ReadDir(root)
	if len(left) != 1 || left[0].Name() != "scumm" {
		t.Fatalf("leftovers: %v", left)
	}
	// Installing again changes nothing and does not download.
	srv.Close()
	if _, err := games.Install(context.Background(), client, root, g, nil); err != nil {
		t.Fatalf("an installed game must not need the network: %v", err)
	}
}

func TestInstallSingleFileGame(t *testing.T) {
	rom := []byte("GBROM")
	srv, client := serve(t, rom)
	g := game(srv, rom, "file")
	g.File, g.Entry = "demo.gb", "demo.gb"
	root := t.TempDir()
	if _, err := games.Install(context.Background(), client, root, g, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "scumm", "demo", "demo.gb")); string(b) != "GBROM" {
		t.Fatalf("%q", b)
	}
}

func TestADownloadThatIsNotThePinnedFileIsDiscarded(t *testing.T) {
	z := makeZip(t, map[string]string{"a": "1"})
	srv, client := serve(t, z)
	root := t.TempDir()

	wrongHash := game(srv, z, "zip")
	wrongHash.SHA256 = strings.Repeat("0", 64)
	if _, err := games.Install(context.Background(), client, root, wrongHash, nil); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("a wrong checksum must be refused: %v", err)
	}
	tooBig := game(srv, z, "zip")
	tooBig.Size = int64(len(z)) - 5
	if _, err := games.Install(context.Background(), client, root, tooBig, nil); err == nil {
		t.Fatal("a download larger than declared must be refused")
	}
	tooSmall := game(srv, z, "zip")
	tooSmall.Size = int64(len(z)) + 100
	if _, err := games.Install(context.Background(), client, root, tooSmall, nil); err == nil {
		t.Fatal("a download smaller than declared must be refused")
	}
	missingEntry := game(srv, z, "zip")
	missingEntry.Entry = "nope.bin"
	if _, err := games.Install(context.Background(), client, root, missingEntry, nil); err == nil || !strings.Contains(err.Error(), "does not contain") {
		t.Fatalf("a missing entry must be reported: %v", err)
	}
	if left, _ := os.ReadDir(root); len(left) != 0 {
		t.Fatalf("failed installs left files behind: %v", left)
	}
}

func TestPlainHTTPAndInsecureRedirectsAreRefused(t *testing.T) {
	z := makeZip(t, map[string]string{"a": "1"})
	srv, client := serve(t, z)
	root := t.TempDir()
	g := game(srv, z, "zip")
	g.URL = strings.Replace(g.URL, "https://", "http://", 1)
	if _, err := games.Install(context.Background(), client, root, g, nil); err == nil {
		t.Fatal("http must be refused")
	}
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(z) }))
	defer plain.Close()
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/file", http.StatusFound)
	}))
	defer redirector.Close()
	g2 := game(redirector, z, "zip")
	if _, err := games.Install(context.Background(), redirector.Client(), root, g2, nil); err == nil {
		t.Fatal("a redirect to plain http must be refused")
	}
	notFound := httptest.NewTLSServer(http.NotFoundHandler())
	defer notFound.Close()
	if _, err := games.Install(context.Background(), notFound.Client(), root, game(notFound, z, "zip"), nil); err == nil {
		t.Fatal("a 404 must be an error")
	}
}

func TestHostileZipsCannotWriteOutsideTheGameFolder(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"parent":    {"../escape.txt": "x"},
		"deep":      {"a/../../escape.txt": "x"},
		"absolute":  {"/etc/passwd": "x"},
		"backslash": {`..\escape.txt`: "x"},
		"drive":     {"C:/Windows/evil.dll": "x"},
		"dots":      {"a/ ./b": "x"},
	} {
		z := makeZip(t, files)
		srv, client := serve(t, z)
		root := filepath.Join(t.TempDir(), "games")
		if _, err := games.Install(context.Background(), client, root, game(srv, z, "zip"), nil); err == nil {
			t.Errorf("%s: a hostile path must be refused", name)
		}
		outside, _ := filepath.Glob(filepath.Join(filepath.Dir(root), "*"))
		for _, o := range outside {
			if filepath.Base(o) != "games" {
				t.Errorf("%s: wrote outside the games folder: %s", name, o)
			}
		}
		if left, _ := filepath.Glob(filepath.Join(root, "*", "*")); len(left) != 0 {
			t.Errorf("%s: left files behind: %v", name, left)
		}
	}
}

func TestInstallRefusesToTakeOverAFolderItDidNotMake(t *testing.T) {
	z := makeZip(t, map[string]string{"a": "1"})
	srv, client := serve(t, z)
	g := game(srv, z, "zip")
	root := t.TempDir()
	mine := filepath.Join(root, "scumm", "demo")
	if err := os.MkdirAll(mine, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mine, "my-saves.sav"), []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := games.Install(context.Background(), client, root, g, nil); err == nil {
		t.Fatal("must not overwrite a folder WinForge did not create")
	}
	if b, _ := os.ReadFile(filepath.Join(mine, "my-saves.sav")); string(b) != "precious" {
		t.Fatal("the person's files were touched")
	}
	if err := games.Remove(root, g); err == nil {
		t.Fatal("must not delete a folder WinForge did not create")
	}
	if _, err := os.Stat(filepath.Join(mine, "my-saves.sav")); err != nil {
		t.Fatal("the person's files were deleted")
	}
}

func TestRemoveDeletesOnlyThatGame(t *testing.T) {
	z := makeZip(t, map[string]string{"a": "1"})
	srv, client := serve(t, z)
	g := game(srv, z, "zip")
	other := g
	other.ID = "other"
	root := t.TempDir()
	for _, x := range []catalog.Game{g, other} {
		if _, err := games.Install(context.Background(), client, root, x, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := games.Remove(root, g); err != nil {
		t.Fatal(err)
	}
	st := games.State(root, []catalog.Game{g, other})
	if _, still := st["demo"]; still || len(st) != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestCancelledDownloadLeavesNothing(t *testing.T) {
	z := makeZip(t, map[string]string{"a": "1"})
	srv, client := serve(t, z)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := t.TempDir()
	if _, err := games.Install(ctx, client, root, game(srv, z, "zip"), nil); err == nil {
		t.Fatal("a cancelled install must fail")
	}
	if left, _ := os.ReadDir(root); len(left) != 0 {
		t.Fatalf("leftovers: %v", left)
	}
}

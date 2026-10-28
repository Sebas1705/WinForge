package wingetpkgs_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/wingetpkgs"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A Mirror must answer exactly like the HTTP client: newest version with an
// installer manifest, default locale, nested packages ignored, 404s.
func TestMirrorReadsLikeHTTP(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	src := t.TempDir()
	files := map[string]string{
		"manifests/a/Acme/App/1.9.0/Acme.App.installer.yaml":     "InstallerUrl: https://acme.example/old.exe\n",
		"manifests/a/Acme/App/1.9.0/Acme.App.locale.en-US.yaml":  "Publisher: Old\n",
		"manifests/a/Acme/App/1.10.0/Acme.App.installer.yaml":    "InstallerType: msi\nInstallers:\n- InstallerUrl: https://acme.example/a.msi\n  InstallerSha256: ABC\n",
		"manifests/a/Acme/App/1.10.0/Acme.App.yaml":              "DefaultLocale: es-ES\n",
		"manifests/a/Acme/App/1.10.0/Acme.App.locale.es-ES.yaml": "Publisher: Acme\nPackageName: Acme App\n",
		// 2.0.0 has no installer manifest yet, so 1.10.0 is the newest usable.
		"manifests/a/Acme/App/2.0.0/Acme.App.yaml": "DefaultLocale: en-US\n",
		// A nested package, not a version.
		"manifests/a/Acme/App/Beta/1.0/Acme.App.Beta.installer.yaml": "InstallerUrl: https://acme.example/b.exe\n",
		"manifests/o/Other/Tool/3.0/Other.Tool.installer.yaml":       "InstallerUrl: http://insecure.example/t.exe\n",
		"manifests/o/Other/Tool/3.0/Other.Tool.locale.en-US.yaml":    "Publisher: Other\n",
	}
	for p, body := range files {
		full := filepath.Join(src, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, src, "init", "-q", "-b", "master")
	gitIn(t, src, "config", "uploadpack.allowFilter", "true")
	gitIn(t, src, "add", ".")
	gitIn(t, src, "commit", "-q", "-m", "manifests")

	ctx := context.Background()
	url := "file://" + filepath.ToSlash(src)
	if !strings.HasPrefix(url, "file:///") {
		url = "file:///" + strings.TrimPrefix(url, "file://")
	}
	m, err := wingetpkgs.OpenMirror(ctx, filepath.Join(t.TempDir(), "clone"), url, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Prefetch(ctx, []string{"Acme.App", "No.Such"}); err != nil {
		t.Fatal(err)
	}
	// Prefetch checked out the chosen version folder, nothing else.
	if _, err := os.Stat(filepath.Join(m.Dir, "manifests/a/Acme/App/1.10.0/Acme.App.yaml")); err != nil {
		t.Fatal("version folder not checked out:", err)
	}
	if _, err := os.Stat(filepath.Join(m.Dir, "manifests/a/Acme/App/1.9.0")); err == nil {
		t.Fatal("an older version was checked out too")
	}

	c := &wingetpkgs.Client{Mirror: m}
	p, err := c.Package(ctx, "Acme.App")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != "1.10.0" || p.Publisher != "Acme" || p.Name != "Acme App" ||
		len(p.Installers) != 1 || p.Installers[0].Type != "msi" || p.Installers[0].SHA256 != "ABC" {
		t.Fatalf("%+v", p)
	}
	// Not prefetched: read from the clone on demand.
	if p, err := c.Package(ctx, "Other.Tool"); err != nil || p.Publisher != "Other" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if _, err := c.Package(ctx, "No.Such"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
}

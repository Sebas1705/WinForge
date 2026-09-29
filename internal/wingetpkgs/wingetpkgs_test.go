package wingetpkgs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/wingetpkgs"
)

func TestCompareVersions(t *testing.T) {
	if wingetpkgs.CompareVersions("1.9.0", "1.10.0") >= 0 || wingetpkgs.CompareVersions("2.0", "2.0") != 0 {
		t.Fatal("numeric comparison broken")
	}
}

func TestManifestDir(t *testing.T) {
	if got := wingetpkgs.ManifestDir("Microsoft.DotNet.SDK.9"); got != "manifests/m/Microsoft/DotNet/SDK/9" {
		t.Fatal(got)
	}
}

func TestPackageMergesManifests(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/microsoft/winget-pkgs/contents/manifests/a/Acme/App", func(w http.ResponseWriter, _ *http.Request) {
		// "Beta" is a nested package, "1.10.0" must beat "1.9.0" numerically.
		w.Write([]byte(`[{"name":"1.9.0","type":"dir"},{"name":"1.10.0","type":"dir"},{"name":"Beta","type":"dir"}]`))
	})
	base := "/microsoft/winget-pkgs/master/manifests/a/Acme/App/1.10.0/Acme.App"
	mux.HandleFunc(base+".installer.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("InstallerType: exe\nInstallers:\n- InstallerUrl: https://acme.example/a.exe\n  InstallerSha256: ABC\n  AppsAndFeaturesEntries:\n  - DisplayName: Acme App 1.10\n"))
	})
	mux.HandleFunc(base+".yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("DefaultLocale: es-ES\n"))
	})
	mux.HandleFunc(base+".locale.es-ES.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("Publisher: Acme\nPackageName: Acme App\nLicense: MIT\nShortDescription: Does things.\nPackageUrl: https://acme.example\nTags: [a, b]\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &wingetpkgs.Client{HTTP: srv.Client(), APIURL: srv.URL, RawURL: srv.URL}
	p, err := c.Package(context.Background(), "Acme.App")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != "1.10.0" || p.Publisher != "Acme" || p.License != "MIT" || len(p.Tags) != 2 ||
		len(p.Installers) != 1 || p.Installers[0].Type != "exe" || !strings.HasPrefix(p.Installers[0].URL, "https://") ||
		len(p.ARP) != 1 || p.ARP[0].DisplayName != "Acme App 1.10" {
		t.Fatalf("%+v", p)
	}
}

func TestMissingPackage(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	c := &wingetpkgs.Client{HTTP: srv.Client(), APIURL: srv.URL, RawURL: srv.URL}
	if _, err := c.Package(context.Background(), "No.Such"); err == nil {
		t.Fatal("expected error")
	}
}

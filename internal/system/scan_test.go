package system_test

import (
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/system"
)

func TestParseWingetExport(t *testing.T) {
	got, err := system.ParseWingetExport([]byte(`{"Sources":[{"Packages":[
		{"PackageIdentifier":"Git.Git","Version":"2.50.0"},
		{"PackageIdentifier":"Microsoft.VisualStudioCode"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got["git.git"] != "2.50.0" {
		t.Fatalf("got %v", got)
	}
	if _, ok := got["microsoft.visualstudiocode"]; !ok {
		t.Fatal("package without version must still be listed")
	}
}

func TestDetectCombinesAllSignals(t *testing.T) {
	cat, _ := catalog.Load(catalogdata.FS)
	inv := system.Inventory{
		Winget:       map[string]string{"git.git": "2.50.0"},
		DisplayNames: []string{"Eclipse Temurin JDK with Hotspot 21.0.4+7 (x64)"},
		LookPath:     func(c string) bool { return c == "node" },
		Exists:       func(string) bool { return false },
	}
	got := system.Detect(cat, inv)
	if got["git"].Version != "2.50.0" || got["git"].Sources[0] != "winget" {
		t.Fatalf("git: %+v", got["git"])
	}
	if _, ok := got["temurin-21"]; !ok {
		t.Fatal("temurin-21 should be found through the registry")
	}
	if _, ok := got["temurin-17"]; ok {
		t.Fatal("temurin-17 must not match a JDK 21 entry")
	}
	if got["nodejs-lts"].Sources[0] != "path" {
		t.Fatalf("node should be found on PATH: %+v", got["nodejs-lts"])
	}
	if _, ok := got["firefox"]; ok {
		t.Fatal("firefox is not installed")
	}
}

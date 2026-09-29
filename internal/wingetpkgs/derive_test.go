package wingetpkgs_test

import (
	"regexp"
	"testing"

	"github.com/Sebas1705/WinForge/internal/wingetpkgs"
)

func matches(t *testing.T, pats []string, s string) bool {
	t.Helper()
	for _, p := range pats {
		if regexp.MustCompile("(?i)" + p).MatchString(s) {
			return true
		}
	}
	return false
}

func TestRegistryPatterns(t *testing.T) {
	git := wingetpkgs.RegistryPatterns(&wingetpkgs.Package{ARP: []wingetpkgs.ARPEntry{{DisplayName: "Git version 2.50.0"}}})
	for _, s := range []string{"Git version 2.55.0.3", "Git", "git (x64)"} {
		if !matches(t, git, s) {
			t.Errorf("git pattern %v should match %q", git, s)
		}
	}
	for _, s := range []string{"Git LFS", "GitHub Desktop", "TortoiseGit 2.1"} {
		if matches(t, git, s) {
			t.Errorf("git pattern %v must not match %q", git, s)
		}
	}
	ff := wingetpkgs.RegistryPatterns(&wingetpkgs.Package{ARP: []wingetpkgs.ARPEntry{{DisplayName: "Mozilla Firefox (x64 en-US)"}}})
	if !matches(t, ff, "Mozilla Firefox (x64 es-ES)") || matches(t, ff, "Mozilla Firefox ESR Nightly") {
		t.Errorf("firefox: %v", ff)
	}
	// no ARP entries: fall back to the name; GUID display names are ignored
	fb := wingetpkgs.RegistryPatterns(&wingetpkgs.Package{Name: "7-Zip"})
	if !matches(t, fb, "7-Zip 24.08 (x64)") {
		t.Errorf("fallback: %v", fb)
	}
	if got := wingetpkgs.RegistryPatterns(&wingetpkgs.Package{Name: "Acme Tool", ARP: []wingetpkgs.ARPEntry{{DisplayName: "{1234-ABCD}"}}}); !matches(t, got, "Acme Tool 3.1") {
		t.Errorf("guid entries should fall back to the name: %v", got)
	}
}

func TestSlugAndOneLine(t *testing.T) {
	if wingetpkgs.Slug("Notepad++") != "notepadplusplus" || wingetpkgs.Slug("  Visual Studio Code!! ") != "visual-studio-code" || wingetpkgs.Slug("Ünï") != "n" {
		t.Fatalf("%q %q %q", wingetpkgs.Slug("Notepad++"), wingetpkgs.Slug("  Visual Studio Code!! "), wingetpkgs.Slug("Ünï"))
	}
	p := &wingetpkgs.Package{ShortDescription: "A very useful tool.  It also does other things that nobody asked for."}
	if got := wingetpkgs.OneLine(p, 160); got != "A very useful tool." {
		t.Fatal(got)
	}
	long := &wingetpkgs.Package{Description: "word " + string(make([]byte, 0)) + "aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff gggggggggg hhhhhhhhhh"}
	if got := wingetpkgs.OneLine(long, 40); len(got) > 44 || got[len(got)-3:] != "…" {
		t.Fatal(got)
	}
}

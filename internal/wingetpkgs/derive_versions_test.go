package wingetpkgs_test

import (
	"testing"

	"github.com/Sebas1705/WinForge/internal/wingetpkgs"
)

func pkg(name string, arp ...string) *wingetpkgs.Package {
	p := &wingetpkgs.Package{Name: name}
	for _, a := range arp {
		p.ARP = append(p.ARP, wingetpkgs.ARPEntry{DisplayName: a})
	}
	return p
}

func TestVersionedProductsDoNotMatchOtherLines(t *testing.T) {
	cases := []struct {
		p       *wingetpkgs.Package
		yes, no []string
	}{
		{pkg("Eclipse Temurin JDK with Hotspot 17", "Eclipse Temurin JDK with Hotspot 17.0.12+7 (x64)"),
			[]string{"Eclipse Temurin JDK with Hotspot 17.0.13+11 (x64)"},
			[]string{"Eclipse Temurin JDK with Hotspot 21.0.4+7 (x64)", "Eclipse Temurin JDK with Hotspot 170.0 (x64)"}},
		{pkg("Python 3.13", "Python 3.13.1 (64-bit)"),
			[]string{"Python 3.13.9 (64-bit)"}, []string{"Python 3.12.4 (64-bit)", "Python 3.1.2 (64-bit)"}},
		{pkg("PostgreSQL 17", "PostgreSQL 17"),
			[]string{"PostgreSQL 17"}, []string{"PostgreSQL 16"}},
		{pkg("Microsoft .NET SDK 9.0", "Microsoft .NET SDK 9.0.100 (x64)"),
			[]string{"Microsoft .NET SDK 9.0.305 (x64)"}, []string{"Microsoft .NET SDK 8.0.100 (x64)"}},
	}
	for _, c := range cases {
		pats := wingetpkgs.RegistryPatterns(c.p)
		if len(pats) == 0 {
			t.Errorf("%s: no patterns", c.p.Name)
		}
		for _, s := range c.yes {
			if !matches(t, pats, s) {
				t.Errorf("%s: %v should match %q", c.p.Name, pats, s)
			}
		}
		for _, s := range c.no {
			if matches(t, pats, s) {
				t.Errorf("%s: %v must not match %q", c.p.Name, pats, s)
			}
		}
	}
}

func TestPatternThatCannotMatchItsOwnEntryIsDropped(t *testing.T) {
	// "v14" in the package name, "2015-2022" in the entry: a loose fallback
	// would match every Visual C++ redistributable, so nothing is emitted.
	p := pkg("Microsoft Visual C++ v14 Redistributable (x64)", "Microsoft Visual C++ 2015-2022 Redistributable (x64) - 14.44.35211")
	if got := wingetpkgs.RegistryPatterns(p); len(got) != 0 {
		t.Fatalf("expected no patterns, got %v", got)
	}
}

func TestArchitectureVariantsAreDistinguished(t *testing.T) {
	x64 := &wingetpkgs.Package{ID: "Microsoft.VCRedist.2013.x64", Name: "Microsoft Visual C++ 2013 Redistributable (x64)",
		ARP: []wingetpkgs.ARPEntry{{DisplayName: "Microsoft Visual C++ 2013 Redistributable (x64) - 12.0.40664"}}}
	pats := wingetpkgs.RegistryPatterns(x64)
	if !matches(t, pats, "Microsoft Visual C++ 2013 Redistributable (x64) - 12.0.40664") {
		t.Fatalf("x64 should match itself: %v", pats)
	}
	if matches(t, pats, "Microsoft Visual C++ 2013 Redistributable (x86) - 12.0.40664") || matches(t, pats, "Microsoft Visual C++ 2012 Redistributable (x64) - 11.0.61030") {
		t.Fatalf("x64 rule too loose: %v", pats)
	}
}

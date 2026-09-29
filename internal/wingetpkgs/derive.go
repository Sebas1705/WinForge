package wingetpkgs

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// A version or architecture marker ends the product name part of an
	// Apps & features entry: "Git version 2.50", "Firefox (x64 en-US)", "Foo 1.2".
	cutAt = regexp.MustCompile(`(?i)\s+(version\s+)?v?\d.*$|\s*[\(\[].*$|\s+x(64|86)\b.*$`)
	// What may follow a product name in an entry, so "Git" matches
	// "Git version 2.50" but not "Git LFS".
	suffix = `(\s+(version\s+)?v?\d|\s*[\(\[]|\s+x(64|86)\b|\s*$)`
	// A product-line version in a package name: "Temurin ... 17", "Python 3.13".
	// Packages that ship one identifier per architecture (Foo.x64, Foo.x86).
	archID      = regexp.MustCompile(`(?i)\.(x64|x86|arm64)$`)
	nameVersion = regexp.MustCompile(`(?:^|\s)v?(\d+(?:\.\d+)*)(?:\s|\)|$)`)
)

// RegistryPatterns derives case-insensitive regular expressions that match
// this package's Apps & features DisplayName, tolerant of version suffixes.
// Manifests without such entries fall back to the package name.
//
// When the package name carries a product-line version ("Python 3.13",
// "Temurin JDK 17") the pattern requires it, so 3.12 or JDK 21 are not
// mistaken for it. Every pattern must match the entry it was derived from;
// one that would not (for instance "v14" against "2015-2022") is dropped,
// because a missing rule only costs a detection while a loose one produces
// false positives.
func RegistryPatterns(p *Package) []string {
	line := ""
	if m := nameVersion.FindStringSubmatch(p.Name); m != nil {
		line = m[1]
	}
	arch := ""
	if m := archID.FindStringSubmatch(p.ID); m != nil {
		arch = strings.ToLower(m[1])
	}
	var arp []string
	for _, e := range p.ARP {
		arp = append(arp, e.DisplayName)
	}
	if out, usable := patterns(arp, line, arch); usable {
		return out
	}
	out, _ := patterns([]string{p.Name}, line, arch)
	return out
}

// patterns returns the derived patterns and whether any name was usable as a
// product name at all (GUID-only entries are not), which decides whether the
// caller may fall back to the package name.
func patterns(names []string, line, arch string) (out []string, usable bool) {
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		base := strings.TrimSpace(cutAt.ReplaceAllString(n, ""))
		if len(base) < 3 || strings.ContainsAny(base, "{}$") { // GUIDs and product codes are per-version
			continue
		}
		usable = true
		tail := suffix
		if line != "" {
			tail = `\s+(version\s+)?v?` + regexp.QuoteMeta(line) + `([.+\-\s\)]|$)`
		}
		pat := "^" + regexp.QuoteMeta(base) + tail
		if arch != "" {
			// x64 and x86 builds share a product name; the entry says which.
			pat += `.*\b` + arch + `\b`
		}
		re, err := regexp.Compile("(?i)" + pat)
		if err != nil || !re.MatchString(n) {
			continue
		}
		if !seen[pat] {
			seen[pat] = true
			out = append(out, pat)
		}
		if len(out) == 3 {
			break
		}
	}
	return out, usable
}

// Slug turns a display name into a catalog id: lowercase letters, digits,
// dashes.
func Slug(name string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLower(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case r == '+':
			b.WriteString("plus")
			dash = false
		default:
			if !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// OneLine returns a single-sentence description of at most max characters.
func OneLine(p *Package, max int) string {
	s := strings.TrimSpace(p.ShortDescription)
	if s == "" {
		s = strings.TrimSpace(p.Description)
	}
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	if i := strings.Index(s, ". "); i > 10 {
		s = s[:i+1]
	}
	if len(s) > max {
		s = strings.TrimSpace(s[:max-1])
		if i := strings.LastIndex(s, " "); i > max/2 {
			s = s[:i]
		}
		s += "…"
	}
	return s
}

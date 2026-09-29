package catalog

import "strings"

// IsOpenSource reports whether a license string, as written in a catalog
// entry or a winget manifest, names an OSI-style open-source license.
// It is deliberately conservative: anything proprietary, source-available or
// unknown is false, and a wrong "false" only hides a badge while a wrong
// "true" would mislead.
func IsOpenSource(license string) bool {
	l := strings.ToLower(license)
	for _, no := range []string{"proprietary", "freeware", "shareware", "commercial", "trial", "eula",
		"source-available", "source available", "busl", "business source", "sspl", "elastic", "commons clause", "polyform", "cc-by-nc", "noncommercial"} {
		if strings.Contains(l, no) {
			return false
		}
	}
	for _, yes := range []string{"mit", "apache", "bsd", "gpl", "lgpl", "agpl", "gnu ", "mpl", "mozilla public",
		"isc", "unlicense", "zlib", "cc0", "epl", "eclipse public", "artistic", "bsl-1.0", "boost", "postgresql",
		"ofl", "open font", "wtfpl", "python software foundation", "psf", "cddl", "openssl", "libpng", "ncsa", "x11", "curl"} {
		if hasToken(l, yes) {
			return true
		}
	}
	return false
}

// hasToken matches yes as a whole word or identifier, so "mit" does not match
// "permit" or "limited".
func hasToken(s, yes string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], yes)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(yes)
		leftOK := start == 0 || !isAlnum(s[start-1])
		rightOK := end >= len(s) || !isAlnum(s[end]) || strings.HasSuffix(yes, " ")
		if leftOK && rightOK {
			return true
		}
		i = start + 1
	}
}

func isAlnum(b byte) bool { return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' }

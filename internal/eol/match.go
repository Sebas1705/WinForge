package eol

import (
	"strconv"
	"strings"
)

// versionParts turns an installed version string into dotted components, or nil when there is
// nothing to compare. It accepts what installers really report:
//
//	"v22.4.0"            a leading v
//	"21.0.4+7"           a build suffix after + (also - and a space: "17.6-1", "3.13.0 (64-bit)")
//	"10.0.19045.1"       four parts
//	"3.13.0rc1"          a pre-release glued to the last number (kept as "0rc1")
//	"1.8.0_402"          underscores count as dots; with SchemeJava "1.8" is Java 8
func versionParts(v, scheme string) []string {
	v = strings.TrimSpace(v)
	if len(v) > 1 && (v[0] == 'v' || v[0] == 'V') && v[1] >= '0' && v[1] <= '9' {
		v = v[1:]
	}
	if i := strings.IndexAny(v, "+- ("); i >= 0 {
		v = v[:i]
	}
	v = strings.ReplaceAll(v, "_", ".")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ".")
	for _, p := range parts {
		if p == "" {
			return nil
		}
	}
	if scheme == SchemeJava && len(parts) >= 2 && parts[0] == "1" {
		// Java 1.8 is Java 8. Only 1.1 to 1.8 ever existed, so "1.17" stays what it is.
		if n, err := strconv.Atoi(parts[1]); err == nil && n >= 1 && n <= 8 {
			parts = parts[1:]
		}
	}
	return parts
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// hasPrefix reports whether the version's leading components are exactly the cycle's: every
// component of the cycle must be present in the version and be the same whole number. "3.1" is
// therefore not a prefix of "3.10.2", and "17" is not a prefix of "1.17".
func hasPrefix(parts []string, cycle string) bool {
	cs := strings.Split(cycle, ".")
	if len(parts) < len(cs) {
		return false
	}
	for i, c := range cs {
		if !isDigits(parts[i]) || compareNumbers(trimZeros(parts[i]), trimZeros(c)) != 0 {
			return false
		}
	}
	return true
}

// Match returns the index in p.Cycles of the cycle the version belongs to, or -1. When several
// cycles are prefixes of the version (say "3" and "3.1"), the longest wins.
func (p *Product) Match(version string) int {
	parts := versionParts(version, p.Scheme)
	if parts == nil {
		return -1
	}
	best, bestLen := -1, 0
	for i, c := range p.Cycles {
		if !hasPrefix(parts, c.Cycle) {
			continue
		}
		if n := strings.Count(c.Cycle, ".") + 1; n > bestLen {
			best, bestLen = i, n
		}
	}
	return best
}

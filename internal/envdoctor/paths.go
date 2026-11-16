package envdoctor

import (
	"path"
	"regexp"
	"strings"
)

// splitPath splits a PATH value on ';', except inside double quotes, the way Windows reads it.
// Joining the result with ";" gives back the original value byte for byte. An empty value has no
// segments at all; "a;b;" has three, the last one empty.
func splitPath(v string) []string {
	if v == "" {
		return []string{}
	}
	out := []string{}
	start, quoted := 0, false
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '"':
			quoted = !quoted
		case ';':
			if !quoted {
				out = append(out, v[start:i])
				start = i + 1
			}
		}
	}
	return append(out, v[start:])
}

// cleanSegment drops the whitespace around a PATH segment and its quotes.
func cleanSegment(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, `"`, ""))
}

var varRef = regexp.MustCompile(`%([^%]+)%`)

// expand replaces every %NAME% that lookup knows. A reference it does not know stays as written,
// like cmd does.
func expand(s string, lookup func(name string) (string, bool)) string {
	if !strings.Contains(s, "%") {
		return s
	}
	return varRef.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := lookup(m[1 : len(m)-1]); ok {
			return v
		}
		return m
	})
}

// unresolved reports whether an expanded value still holds a %NAME% reference.
func unresolved(s string) bool { return varRef.MatchString(s) }

// isAbs is true for "C:\..." (or "C:/...") and UNC paths, the only forms that do not depend on the
// current directory.
func isAbs(s string) bool {
	if len(s) >= 3 && isLetter(s[0]) && s[1] == ':' && (s[2] == '\\' || s[2] == '/') {
		return true
	}
	return strings.HasPrefix(s, `\\`) || strings.HasPrefix(s, "//")
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// normDir turns an expanded absolute directory into one canonical spelling: backslashes, no
// trailing separator (except on a drive root), no "." or ".." parts. Case is kept; use dirKey to
// compare. A relative path comes back cleaned of quotes and spaces only.
func normDir(expanded string) string {
	s := strings.ReplaceAll(cleanSegment(expanded), "/", `\`)
	if !isAbs(s) {
		return s
	}
	if strings.HasPrefix(s, `\\`) {
		return `\` + strings.ReplaceAll(path.Clean("/"+strings.ReplaceAll(s[2:], `\`, "/")), "/", `\`)
	}
	rest := path.Clean("/" + strings.ReplaceAll(s[2:], `\`, "/"))
	return s[:2] + strings.ReplaceAll(rest, "/", `\`)
}

// dirKey is what two spellings of the same directory share.
func dirKey(norm string) string { return strings.ToLower(norm) }

// join appends a file name to a directory without doubling the separator.
func join(dir, name string) string {
	return strings.TrimRight(dir, `\/`) + `\` + name
}

package diag

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Scrubber removes the person's identity from text: their Windows user name and their profile
// folder. Everything that goes into a diagnostic report passes through it, so a path such as
// C:\Users\ana\AppData\Roaming\WinForge\history.json becomes %USERPROFILE%\AppData\Roaming\...
// and a stray "ana" becomes <user>.
type Scrubber struct {
	homes []*regexp.Regexp
	users []*regexp.Regexp
}

// userFolder matches any C:\Users\<name> (or /Users/<name>, /home/<name>) in case the name that
// was given to NewScrubber is not the one that appears (another account, a short 8.3 name).
var userFolder = regexp.MustCompile(`(?i)((?:[a-z]:)?[\\/]{1,4}(?:users|home)[\\/]{1,4})[^\\/\s"'<>|:*?]+`)

// NewScrubber builds a scrubber for these user names and profile folders. Empty values are
// ignored; "DOMAIN\ana" also scrubs "ana".
func NewScrubber(users, homes []string) Scrubber {
	var s Scrubber
	seen := map[string]bool{}
	var names []string
	for _, u := range users {
		u = strings.TrimSpace(u)
		if i := strings.LastIndexAny(u, `\/`); i >= 0 {
			names = append(names, u[:i]) // the domain is as identifying as the name
			u = u[i+1:]
		}
		if i := strings.Index(u, "@"); i > 0 {
			names = append(names, u[i+1:])
			u = u[:i]
		}
		names = append(names, u)
	}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		s.users = append(s.users, regexp.MustCompile(`(?i)`+regexp.QuoteMeta(n)))
	}
	// Longer names first so "ana.garcia" is replaced before "ana".
	sort.SliceStable(s.users, func(i, j int) bool { return len(s.users[i].String()) > len(s.users[j].String()) })

	hs := map[string]bool{}
	var homeList []string
	for _, h := range homes {
		h = strings.TrimRight(strings.TrimSpace(h), `\/`)
		h = strings.TrimPrefix(h, `\\?\`)
		if h == "" || hs[strings.ToLower(h)] {
			continue
		}
		hs[strings.ToLower(h)] = true
		homeList = append(homeList, h)
	}
	sort.SliceStable(homeList, func(i, j int) bool { return len(homeList[i]) > len(homeList[j]) })
	for _, h := range homeList {
		s.homes = append(s.homes, pathRegexp(h))
	}
	return s
}

// pathRegexp matches a folder in any separator style, JSON-escaped backslashes included, and with
// an optional \\?\ prefix.
func pathRegexp(p string) *regexp.Regexp {
	parts := regexp.MustCompile(`[\\/]+`).Split(strings.TrimRight(p, `\/`), -1)
	for i := range parts {
		parts[i] = regexp.QuoteMeta(parts[i])
	}
	return regexp.MustCompile(`(?i)(?:\\{1,4}\?\\{1,4})?` + strings.Join(parts, `[\\/]{1,4}`))
}

// anyPath finds an absolute Windows path (a drive letter or a \\server\share) in a log line.
// Spaces are allowed inside folder names; the last segment ends at the next space.
var anyPath = regexp.MustCompile(`(?:[A-Za-z]:[\\/]|\\\\[^\s\\/"'<>|*?]+[\\/])(?:[^\\/\s"'<>|*?]+(?: [^\\/\s"'<>|*?]+)*[\\/]+)*[^\s"'<>|*?]*`)

// HidePaths replaces every absolute path in text with <path>, except the ones under dataDir, which
// is WinForge's own folder and is shown as %AppData%\WinForge. A log line may mention a file the
// person opened; its name has no place in a report.
func HidePaths(text, dataDir string) string {
	const mark = "\x00DATA\x00"
	text = strings.ReplaceAll(text, `\\?\`, "")
	if dataDir != "" {
		text = pathRegexp(dataDir).ReplaceAllString(text, mark)
	}
	text = anyPath.ReplaceAllString(text, "<path>")
	return strings.ReplaceAll(text, mark, DataFolderLabel)
}

// CleanPaths removes only profile folders (not bare user names): it is the last pass over finished
// JSON, where a user name that is also a field name must not break the document.
func (s Scrubber) CleanPaths(text string) string {
	for _, re := range s.homes {
		text = re.ReplaceAllString(text, `%USERPROFILE%`)
	}
	return userFolder.ReplaceAllString(text, `${1}<user>`)
}

// Clean returns text with the profile folder and the user name removed.
func (s Scrubber) Clean(text string) string {
	for _, re := range s.homes {
		text = re.ReplaceAllString(text, `%USERPROFILE%`)
	}
	text = userFolder.ReplaceAllString(text, `${1}<user>`)
	for _, re := range s.users {
		text = replaceWords(re, text)
	}
	return text
}

// replaceWords replaces whole-word matches only, so a user called "al" does not turn "Local" into
// "Loc<user>". A name inside a longer word is left alone unless it is the whole word.
func replaceWords(re *regexp.Regexp, text string) string {
	locs := re.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, l := range locs {
		before, after := rune(' '), rune(' ')
		if l[0] > 0 {
			before = lastRune(text[:l[0]])
		}
		if l[1] < len(text) {
			after = firstRune(text[l[1]:])
		}
		if isWordRune(before) || isWordRune(after) {
			continue
		}
		b.WriteString(text[last:l[0]])
		b.WriteString("<user>")
		last = l[1]
	}
	b.WriteString(text[last:])
	return b.String()
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return ' '
}

func lastRune(s string) rune {
	r := []rune(s)
	if len(r) == 0 {
		return ' '
	}
	return r[len(r)-1]
}

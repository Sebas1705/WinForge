package startup

import (
	"path"
	"strings"
)

// Parsed is what a startup command line means: the program it starts and what it passes.
type Parsed struct {
	// Path is the program with environment variables expanded. Empty when the command
	// starts a generic launcher (rundll32, cmd...) or is empty.
	Path string
	// Args is the rest of the command line, as written. For a launcher it is the whole
	// command, since the launcher's own parameters are what it runs.
	Args string
	// Launcher is true for generic launchers. Their target is a parameter, not a file, so
	// nothing is checked for them and they have no publisher of their own.
	Launcher bool
	// Checkable is true when Path is an absolute path whose existence can be tested. A bare
	// name such as "notepad.exe" is found through PATH and is not reported as missing.
	Checkable bool
}

// launchers are programs that only run something else named in their arguments.
var launchers = map[string]bool{
	"rundll32": true, "cmd": true, "powershell": true, "pwsh": true, "wscript": true,
	"cscript": true, "mshta": true, "regsvr32": true, "msiexec": true, "conhost": true,
}

// programExts are the endings a Run command's program normally has; they let an unquoted path
// with spaces and arguments be split when no prefix of it exists on disk.
var programExts = []string{".exe", ".bat", ".cmd", ".com", ".lnk", ".scr", ".msc", ".cpl", ".vbs", ".js", ".ps1", ".url"}

// ExpandPercent replaces %NAME% with lookup(NAME). A variable that is not set (empty answer) is
// left as written, as Windows does.
func ExpandPercent(s string, lookup func(string) string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '%' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i+1:], '%')
		if end < 0 {
			b.WriteString(s[i:])
			break
		}
		name := s[i+1 : i+1+end]
		if v := lookup(name); name != "" && v != "" {
			b.WriteString(v)
			i += end + 2
			continue
		}
		// Not a variable: keep the percent sign and look again from the next character.
		b.WriteByte('%')
		i++
	}
	return b.String()
}

// baseName is the lower-case file name of a Windows or slash path without its extension.
func baseName(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	b := strings.ToLower(path.Base(p))
	if i := strings.LastIndexByte(b, '.'); i > 0 {
		b = b[:i]
	}
	return b
}

func isAbsolute(p string) bool {
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
		return true
	}
	return strings.HasPrefix(p, `\\`)
}

func finish(p, args string) Parsed {
	args = strings.TrimSpace(args)
	if launchers[baseName(p)] {
		return Parsed{Args: strings.TrimSpace(p + " " + args), Launcher: true}
	}
	return Parsed{Path: p, Args: args, Checkable: isAbsolute(p)}
}

func hasProgramExt(p string) bool {
	l := strings.ToLower(p)
	for _, e := range programExts {
		if strings.HasSuffix(l, e) {
			return true
		}
	}
	return false
}

// wordEnds returns, for each whitespace-separated word of s, the offset where it ends.
func wordEnds(s string) []int {
	var ends []int
	inWord := false
	for i := 0; i < len(s); i++ {
		space := s[i] == ' ' || s[i] == '\t'
		if inWord && space {
			ends = append(ends, i)
		}
		inWord = !space
	}
	if inWord {
		ends = append(ends, len(s))
	}
	return ends
}

// ParseCommand splits a Run value or startup command line into program and arguments. Variables
// like %ProgramFiles% are expanded with lookup. exists tells whether a file is there; it is only
// used to find where an unquoted path with spaces ends.
func ParseCommand(cmd string, lookup func(string) string, exists func(string) bool) Parsed {
	s := strings.TrimSpace(ExpandPercent(cmd, lookup))
	if s == "" {
		return Parsed{}
	}
	if s[0] == '"' {
		rest := s[1:]
		if end := strings.IndexByte(rest, '"'); end >= 0 {
			return finish(rest[:end], rest[end+1:])
		}
		return finish(rest, "")
	}
	ends := wordEnds(s)
	first := s[:ends[0]]
	// A launcher's first word is enough; its arguments are not a path.
	if launchers[baseName(first)] {
		return finish(first, s[ends[0]:])
	}
	// Try progressively longer prefixes until a file exists ("C:\Program Files\A\a.exe /x").
	for _, e := range ends {
		p := s[:e]
		if exists == nil {
			break
		}
		if exists(p) {
			return finish(p, s[e:])
		}
		// Windows adds ".exe" to a program given without an extension.
		if !hasProgramExt(p) && exists(p+".exe") {
			return finish(p+".exe", s[e:])
		}
	}
	// Nothing exists: end the path at the first word that looks like a program name, or at the
	// first word when none does.
	for _, e := range ends {
		if p := s[:e]; hasProgramExt(p) {
			return finish(p, s[e:])
		}
	}
	return finish(first, s[ends[0]:])
}

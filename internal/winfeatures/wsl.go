package winfeatures

import (
	"strconv"
	"strings"
	"unicode/utf16"
)

// Distro is one installed WSL distribution.
type Distro struct {
	Name      string `json:"name"`
	State     string `json:"state"` // "Running", "Stopped", or what wsl.exe said for a transitional state
	Version   int    `json:"version"`
	IsDefault bool   `json:"isDefault"`
}

// WSLInfo is what WSL has on this PC.
type WSLInfo struct {
	Installed      bool     `json:"installed"`      // wsl.exe is there and the WSL feature is on or a distribution exists
	DefaultVersion int      `json:"defaultVersion"` // 1 or 2; 2 when the person never chose
	Distros        []Distro `json:"distros"`        // never null
}

// WSL lists the distributions with `wsl.exe -l -v` and `wsl.exe -l --running -q`. Neither starts a
// distribution or runs anything inside one. A missing wsl.exe, WSL turned off, or "no distributions
// installed" all come back as an empty list rather than an error.
func WSL(sys System) WSLInfo {
	info := WSLInfo{DefaultVersion: 2, Distros: []Distro{}}
	if v, ok := sys.WSLDefaultVersion(); ok && (v == 1 || v == 2) {
		info.DefaultVersion = v
	}
	out, _, err := sys.WSL("-l", "-v")
	if err != nil {
		return info // ErrWSLMissing, or wsl.exe could not run
	}
	info.Distros = ParseDistros(out)
	if len(info.Distros) > 0 {
		running, code, rerr := sys.WSL("-l", "--running", "-q")
		if rerr == nil && code == 0 {
			applyRunning(info.Distros, ParseNames(running))
		}
	}
	states, serr := sys.OptionalFeatures()
	info.Installed = len(info.Distros) > 0 || (serr == nil && states[WindowsName("wsl")] == installEnabled)
	return info
}

// applyRunning replaces the localized state text with "Running" or "Stopped", which is all a
// listed distribution can be apart from the short-lived Installing, Uninstalling and Converting.
func applyRunning(ds []Distro, running []string) {
	up := make(map[string]bool, len(running))
	for _, n := range running {
		up[strings.ToLower(n)] = true
	}
	for i := range ds {
		switch {
		case up[strings.ToLower(ds[i].Name)]:
			ds[i].State = "Running"
		default:
			if c, ok := canonicalState(ds[i].State); ok {
				ds[i].State = c
			} else {
				ds[i].State = "Stopped"
			}
		}
	}
}

func canonicalState(s string) (string, bool) {
	for _, c := range []string{"Running", "Stopped", "Installing", "Uninstalling", "Converting"} {
		if strings.EqualFold(s, c) {
			return c, true
		}
	}
	return "", false
}

// DecodeOutput turns the bytes wsl.exe wrote into text. It writes UTF-16LE (usually without a BOM,
// with NULs between ASCII letters) unless WSL_UTF8 is set, so both are accepted. Carriage returns
// and a BOM are dropped.
func DecodeOutput(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:]
	} else if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
		return strings.ReplaceAll(string(b), "\r", "")
	}
	if !looksUTF16(b) {
		return strings.ReplaceAll(string(b), "\r", "")
	}
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	s := string(utf16.Decode(u))
	s = strings.TrimPrefix(s, string(rune(0xFEFF)))
	return strings.ReplaceAll(s, "\r", "")
}

// looksUTF16 guesses UTF-16LE from NUL bytes in the odd positions, which text in any Latin script
// has; UTF-8 text never contains NULs.
func looksUTF16(b []byte) bool {
	if len(b) < 2 {
		return false
	}
	nul, n := 0, 0
	for i := 1; i < len(b); i += 2 {
		n++
		if b[i] == 0 {
			nul++
		}
	}
	return n > 0 && nul*2 >= n
}

// ParseNames reads the output of `wsl -l -q`: one distribution name per line.
func ParseNames(out []byte) []string {
	var names []string
	for _, l := range strings.Split(DecodeOutput(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			names = append(names, l)
		}
	}
	return names
}

// ParseDistros reads the table `wsl -l -v` prints. The headers are localized ("NAME STATE VERSION",
// "NOMBRE ESTADO VERSIÓN"), so the columns are found by where the header words start, not by what
// they say. Output that has no table (wsl not installed, no distributions) gives an empty list.
func ParseDistros(out []byte) []Distro {
	lines := strings.Split(DecodeOutput(out), "\n")
	var cols []int // rune offsets where the name, state and version columns start
	distros := []Distro{}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		r := []rune(strings.TrimRight(line, " \t"))
		if cols == nil {
			cols = headerColumns(r)
			continue
		}
		if d, ok := parseRow(r, cols); ok {
			distros = append(distros, d)
		}
	}
	return distros
}

// headerColumns returns the start of each column when r is a header of exactly three words (words
// are separated by two or more spaces, so a header such as "EN EJECUCIÓN" would still hold together),
// and nil for any other line.
func headerColumns(r []rune) []int {
	var starts []int
	i := 0
	for i < len(r) {
		if r[i] == ' ' || r[i] == '\t' {
			i++
			continue
		}
		starts = append(starts, i)
		for i < len(r) && !(r[i] == ' ' && i+1 < len(r) && r[i+1] == ' ') && r[i] != '\t' {
			i++
		}
	}
	if len(starts) != 3 || starts[0] > 2 {
		return nil
	}
	return starts
}

// parseRow reads one table row. The two first characters are "* " for the default distribution.
// When the columns do not line up (a long name), it falls back to reading the state and version
// from the end of the line.
func parseRow(r []rune, cols []int) (Distro, bool) {
	if len(r) < 3 {
		return Distro{}, false
	}
	d := Distro{IsDefault: r[0] == '*'}
	if len(r) > cols[2] {
		name := strings.TrimSpace(string(r[min(2, cols[0]):cols[1]]))
		state := strings.TrimSpace(string(r[cols[1]:cols[2]]))
		ver, err := strconv.Atoi(strings.TrimSpace(string(r[cols[2]:])))
		if err == nil && name != "" && !strings.ContainsAny(name, "\t") {
			d.Name, d.State, d.Version = name, state, ver
			return d, true
		}
	}
	f := strings.Fields(string(r[2:]))
	if len(f) < 3 {
		return Distro{}, false
	}
	ver, err := strconv.Atoi(f[len(f)-1])
	if err != nil {
		return Distro{}, false
	}
	d.Name, d.State, d.Version = strings.Join(f[:len(f)-2], " "), f[len(f)-2], ver
	return d, true
}

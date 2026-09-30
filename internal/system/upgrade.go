package system

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"strings"
)

// Upgrade is one installed package that has a newer version available.
type Upgrade struct {
	ID        string `json:"id"` // winget PackageIdentifier
	Name      string `json:"name"`
	Current   string `json:"current"`
	Available string `json:"available"`
}

var (
	rule    = regexp.MustCompile(`^-{4,}\s*$`)
	idShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]*$`)
)

// ParseUpgrade reads the table `winget upgrade` prints. winget has no JSON
// output for it and the headers are localized, so columns are located from the
// header row's layout (a column starts after two or more spaces) rather than
// by header text. Rows whose Id column does not look like a package identifier
// are ignored, which also skips progress spinners, footers and truncated lines.
func ParseUpgrade(out string) []Upgrade {
	out = strings.ReplaceAll(strings.ReplaceAll(out, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(out, "\n")
	var res []Upgrade
	for i := 1; i < len(lines); i++ {
		if !rule.MatchString(lines[i]) || strings.TrimSpace(lines[i-1]) == "" {
			continue
		}
		cols := columnStarts(lines[i-1])
		if len(cols) < 4 { // Name, Id, Version, Available (+ Source)
			continue
		}
		for _, line := range lines[i+1:] {
			if strings.TrimSpace(line) == "" {
				break
			}
			l := []rune(line) // columns are character positions, not bytes
			cell := func(n int) string {
				start, end := cols[n], len(l)
				if n+1 < len(cols) {
					end = cols[n+1]
				}
				if start >= len(l) {
					return ""
				}
				if end > len(l) {
					end = len(l)
				}
				return strings.TrimSpace(string(l[start:end]))
			}
			// winget marks an unknown lower bound as "< 1.2.3"; that is still
			// the installed version for comparison purposes.
			u := Upgrade{Name: cell(0), ID: cell(1), Current: strings.TrimLeft(cell(2), "<> "), Available: cell(3)}
			if idShape.MatchString(u.ID) && u.Available != "" && u.Current != "" && u.Available != u.Current {
				res = append(res, u)
			}
		}
	}
	return res
}

// columnStarts returns the character offset where each header column begins:
// a non-space character at the start of the line or after two or more spaces.
func columnStarts(header string) []int {
	var starts []int
	r := []rune(header)
	for i, c := range r {
		if c == ' ' || c == '\t' {
			continue
		}
		if i == 0 || (r[i-1] == ' ' && (i == 1 || r[i-2] == ' ')) {
			starts = append(starts, i)
		}
	}
	return starts
}

// WingetUpgrades lists packages with updates available.
func WingetUpgrades(ctx context.Context) ([]Upgrade, error) {
	cmd := exec.CommandContext(ctx, "winget", "upgrade", "--accept-source-agreements", "--disable-interactivity")
	hideWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	// A non-zero exit is normal when nothing is upgradable; only a missing
	// binary is an error.
	if err := cmd.Run(); err != nil {
		if _, isExit := err.(*exec.ExitError); !isExit {
			return nil, ErrNoWinget
		}
	}
	return ParseUpgrade(out.String()), nil
}

package install

import (
	"regexp"
	"strconv"
	"strings"
)

// Reasons a step can fail, as keys the UI translates. They name what the
// person can do about it, not what the installer said.
const (
	ReasonAdmin     = "admin"     // needs administrator rights
	ReasonBusy      = "busy"      // another installer is running
	ReasonNetwork   = "network"   // the download failed
	ReasonHash      = "hash"      // the file did not match its published hash
	ReasonCancelled = "cancelled" // someone closed the installer
	ReasonReboot    = "reboot"    // finished, but Windows has to restart
	ReasonDisk      = "disk"      // out of space
	ReasonInstaller = "installer" // the installer itself failed, cause unknown
)

// winget's own exit codes (winget-cli doc/windows/package-manager/winget/returncodes.md).
const (
	wingetDownloadFailed = 0x8A150008
	wingetHashMismatch   = 0x8A150011
	wingetInstallFailed  = 0x8A150049
	wingetCancelled      = 0x8A15010C
)

var innerCode = regexp.MustCompile(`(?i)(?:exit code|código de salida|codigo de salida)[:\s]+(-?\d+)`)

// Explain turns a failed step's exit code and last output lines into a Reason.
// The installer's own code, which winget prints, says more than winget's:
// 0x8A150049 only means "the installer failed", while 1603 or 740 say why.
func Explain(code int, lines []string) string {
	switch uint32(code) {
	case wingetDownloadFailed:
		return ReasonNetwork
	case wingetHashMismatch:
		return ReasonHash
	case wingetCancelled:
		return ReasonCancelled
	}
	text := strings.ToLower(strings.Join(lines, "\n"))
	inner := -1
	for _, l := range lines {
		if m := innerCode.FindStringSubmatch(l); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				inner = n
			}
		}
	}
	switch inner {
	case 740, 5, 1925: // elevation required, access denied, cannot run as this user
		return ReasonAdmin
	case 1618:
		return ReasonBusy
	case 1602, 1223:
		return ReasonCancelled
	case 3010, 1641, 1614:
		return ReasonReboot
	case 112:
		return ReasonDisk
	}
	switch {
	case strings.Contains(text, "como administrador") || strings.Contains(text, "as administrator") ||
		strings.Contains(text, "requires elevation") || strings.Contains(text, "elevated"):
		return ReasonAdmin
	case strings.Contains(text, "no space left") || strings.Contains(text, "espacio suficiente") || strings.Contains(text, "not enough space"):
		return ReasonDisk
	}
	if inner == 1603 {
		// A generic MSI failure. Without administrator rights it is the usual cause.
		return ReasonAdmin
	}
	if uint32(code) == wingetInstallFailed || inner > 0 {
		return ReasonInstaller
	}
	return ""
}

var (
	percentRe = regexp.MustCompile(`(\d{1,3})\s?%`)
	sizeRe    = regexp.MustCompile(`(?i)([\d.,]+)\s*(KB|MB|GB)\s*/\s*([\d.,]+)\s*(KB|MB|GB)`)
)

var unit = map[string]float64{"KB": 1 << 10, "MB": 1 << 20, "GB": 1 << 30}

// ParsePercent reads download progress from a winget output line, either "52%"
// or "10.0 MB / 25.0 MB". It returns -1 for lines that carry none.
func ParsePercent(line string) int {
	if m := sizeRe.FindStringSubmatch(line); m != nil {
		done, e1 := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		total, e2 := strconv.ParseFloat(strings.ReplaceAll(m[3], ",", "."), 64)
		if e1 == nil && e2 == nil && total > 0 {
			p := int(done * unit[strings.ToUpper(m[2])] * 100 / (total * unit[strings.ToUpper(m[4])]))
			if p >= 0 && p <= 100 {
				return p
			}
		}
	}
	if m := percentRe.FindStringSubmatch(line); m != nil {
		if p, err := strconv.Atoi(m[1]); err == nil && p <= 100 {
			return p
		}
	}
	return -1
}

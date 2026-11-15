package startup

import "strings"

// cautionWords mark names and commands of Windows and security components. Matching is on
// lower-case text.
var cautionWords = []string{
	"securityhealth", "windows security", "windowsdefender", "windows defender", "defender",
	"msmpeng", "smartscreen", "sechealthui",
}

// Caution reports whether an item looks like part of Windows or of security software, so the page
// can warn that switching it off may reduce protection. It is a warning, never a block.
func Caution(name, publisher, command string) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(publisher)), "microsoft") {
		return true
	}
	text := strings.ToLower(name + " " + command)
	for _, w := range cautionWords {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

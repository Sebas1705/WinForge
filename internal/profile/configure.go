package profile

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

// resourceID turns a catalog id into something safe to use as a configuration resource id: letters,
// digits, dashes and underscores only.
var notResourceID = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// quote writes a YAML double-quoted scalar. Every value goes through it, whatever it contains,
// so a profile name or an id can never break out of its line or add keys of its own.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029:
			b.WriteString(`\u` + fmt.Sprintf("%04x", r))
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// comment makes text safe inside a one-line YAML comment.
func comment(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == 0x85 || r == 0x2028 || r == 0x2029 {
			return ' '
		}
		return r
	}, s)
}

// WingetConfigure renders a profile's apps as a `winget configure` file (the WinGet Configuration
// format Microsoft documents), so the setup can be replayed on a PC that does not have WinForge,
// or kept next to a project. Only apps are written: WinForge recipes run PowerShell, which does
// not belong in a file somebody else will apply, so they are listed in a comment and left out.
func WingetConfigure(name string, apps []catalog.ProfileApp, recipes []string, cat *catalog.Catalog) ([]byte, error) {
	var b strings.Builder
	b.WriteString("# yaml-language-server: $schema=https://aka.ms/configuration-dsc-schema/0.2\n")
	fmt.Fprintf(&b, "# Made by WinForge from the profile %s.\n", quote(comment(name)))
	b.WriteString("# Apply it with:  winget configure -f <this file>\n")
	b.WriteString("# A configuration file changes the PC it runs on: read it before you apply it.\n")
	if len(recipes) > 0 {
		b.WriteString("# Left out, because WinForge recipes run scripts: ")
		for i, r := range recipes {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(comment(r))
		}
		b.WriteString("\n")
	}
	b.WriteString("properties:\n")
	if len(apps) == 0 {
		b.WriteString("  resources: []\n")
	} else {
		b.WriteString("  resources:\n")
	}
	used := map[string]bool{}
	for _, a := range apps {
		app := cat.Apps[a.ID]
		if app == nil {
			return nil, fmt.Errorf("unknown app %q", a.ID)
		}
		if a.Version != "" && !appVersion.MatchString(a.Version) {
			return nil, fmt.Errorf("invalid version %q for app %q", a.Version, a.ID)
		}
		id := notResourceID.ReplaceAllString(a.ID, "-")
		if id == "" {
			id = "app"
		}
		for n, base := 2, id; used[id]; n++ { // two catalog ids can collapse to the same text
			id = base + "-" + strconv.Itoa(n)
		}
		used[id] = true
		b.WriteString("    - resource: Microsoft.WinGet.DSC/WinGetPackage\n")
		fmt.Fprintf(&b, "      id: %s\n", quote(id))
		b.WriteString("      directives:\n")
		fmt.Fprintf(&b, "        description: %s\n", quote("Install "+app.Name))
		b.WriteString("        allowPrerelease: true\n")
		b.WriteString("      settings:\n")
		fmt.Fprintf(&b, "        id: %s\n", quote(app.Winget))
		b.WriteString("        source: winget\n")
		if a.Version != "" {
			fmt.Fprintf(&b, "        version: %s\n", quote(a.Version))
		}
	}
	b.WriteString("  configurationVersion: 0.2.0\n")
	return []byte(b.String()), nil
}

package system_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/system"
)

// table lays rows out in fixed-width columns, the way winget does.
func table(eol string, head [5]string, rows ...[5]string) string {
	line := func(c [5]string) string { return fmt.Sprintf("%-22s%-30s%-12s%-12s%s", c[0], c[1], c[2], c[3], c[4]) }
	out := "   - " + eol + "   \\ " + eol + line(head) + eol + strings.Repeat("-", 84) + eol
	for _, r := range rows {
		out += line(r) + eol
	}
	return out + eol
}

func TestParseUpgradeEnglish(t *testing.T) {
	out := table("\r\n", [5]string{"Name", "Id", "Version", "Available", "Source"},
		[5]string{"Git", "Git.Git", "2.50.0", "2.55.0", "winget"},
		[5]string{"Visual Studio Code", "Microsoft.VisualStudioCode", "1.99.0", "1.139.0", "winget"},
		[5]string{"Already current", "Vendor.Odd", "1.0", "1.0", "winget"})
	got := system.ParseUpgrade(out + "2 upgrades available.\r\n")
	if len(got) != 2 || got[0].ID != "Git.Git" || got[0].Current != "2.50.0" || got[0].Available != "2.55.0" || got[0].Name != "Git" ||
		got[1].ID != "Microsoft.VisualStudioCode" || got[1].Available != "1.139.0" {
		t.Fatalf("%+v", got)
	}
}

func TestParseUpgradeLocalizedHeadersAndSpinner(t *testing.T) {
	out := table("\r", [5]string{"Nombre", "Id", "Versión", "Disponible", "Origen"},
		[5]string{"Node.js", "OpenJS.NodeJS", "24.1.0", "24.18.1", "winget"})
	got := system.ParseUpgrade(out)
	if len(got) != 1 || got[0].ID != "OpenJS.NodeJS" || got[0].Available != "24.18.1" {
		t.Fatalf("columns are located by layout, not header text: %+v", got)
	}
}

func TestParseUpgradeNothingToDo(t *testing.T) {
	if got := system.ParseUpgrade("No installed package found matching input criteria.\n"); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestParseUpgradeIgnoresLowerBoundMarkerWhenVersionsMatch(t *testing.T) {
	out := table("\n", [5]string{"Name", "Id", "Version", "Available", "Source"},
		[5]string{"Ubisoft Connect", "Ubisoft.Connect", "< 173.1.0", "173.1.0", "winget"},
		[5]string{"Real update", "Vendor.App", "< 1.0", "2.0", "winget"})
	got := system.ParseUpgrade(out)
	if len(got) != 1 || got[0].ID != "Vendor.App" || got[0].Current != "1.0" {
		t.Fatalf("%+v", got)
	}
}

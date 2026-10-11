package health_test

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/health"
)

func TestEveryFixPointsToARealRepairRecipe(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(health.RepairIDs()) < 5 {
		t.Fatal("too few repairs")
	}
	for _, id := range health.RepairIDs() {
		r := cat.Recipes[id]
		if r == nil {
			t.Errorf("%s: no such recipe", id)
			continue
		}
		if !strings.HasPrefix(id, "repair-") {
			t.Errorf("%s: repairs must be named repair-*", id)
		}
		if !r.Admin {
			t.Errorf("%s: a repair changes the system and must ask for administrator rights", id)
		}
	}
	// A fix is only offered for a finding the analyzer can produce.
	known := map[string]bool{}
	for _, f := range health.Analyze(&health.Report{}, nil, time.Now()) {
		known[f.Key] = true
	}
	for key := range map[string]bool{"device-problems": true, "wu-drivers": true, "wu-software": true, "av-signatures": true, "antivirus": true, "firewall": true} {
		if len(health.FixesFor(key)) == 0 {
			t.Errorf("%s has no fix", key)
		}
	}
	if got := health.FixesFor("bios-age"); got != nil {
		t.Errorf("a BIOS update must never be automatic: %v", got)
	}
	f := health.FixesFor("firewall")
	f[0] = "changed"
	if health.FixesFor("firewall")[0] == "changed" {
		t.Error("FixesFor must return a copy")
	}
}

// Every repair script is parsed by PowerShell itself, so a typo cannot ship.
func TestRepairScriptsAreValidPowerShell(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell parser is checked on Windows")
	}
	ps, err := exec.LookPath("powershell")
	if err != nil {
		t.Skip("no powershell")
	}
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for id, r := range cat.Recipes {
		if !strings.HasPrefix(id, "repair-") {
			continue
		}
		for part, body := range map[string]string{"powershell": r.PowerShell, "check": r.Check} {
			if strings.TrimSpace(body) == "" {
				continue
			}
			file := filepath.Join(dir, id+"-"+part+".ps1")
			if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			script := `$e=$null;$t=$null;[void][System.Management.Automation.Language.Parser]::ParseFile('` + file + `',[ref]$t,[ref]$e);if($e.Count){$e|ForEach-Object{$_.Message};exit 1}`
			u := utf16.Encode([]rune(script))
			buf := make([]byte, 0, len(u)*2)
			for _, c := range u {
				buf = append(buf, byte(c), byte(c>>8))
			}
			out, err := exec.Command(ps, "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(buf)).CombinedOutput()
			if err != nil {
				t.Errorf("%s (%s) does not parse: %s", id, part, strings.TrimSpace(string(out)))
			}
		}
	}
}

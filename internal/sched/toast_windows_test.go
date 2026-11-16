//go:build windows

package sched

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The toast script is parsed, never run: ParseInput builds the syntax tree and reports errors without
// executing a line, so no notification can appear and nothing on the PC changes.
func TestToastScriptParses(t *testing.T) {
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("no Windows PowerShell")
	}
	cmd := exec.Command(ps, "-NoProfile", "-NonInteractive", "-Command",
		`$e=$null; $t=$null; [void][System.Management.Automation.Language.Parser]::ParseInput($env:WF_SCRIPT,[ref]$t,[ref]$e); if ($e.Count -gt 0) { $e | ForEach-Object { $_.Message }; exit 1 }`)
	cmd.Env = append(os.Environ(), "WF_SCRIPT="+ToastScript())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the toast script has syntax errors:\n%s", out)
	}
}

func TestToastScriptTakesItsDataOnlyFromTheEnvironment(t *testing.T) {
	s := ToastScript()
	for _, want := range []string{"$env:WINFORGE_TOAST_TITLE", "$env:WINFORGE_TOAST_BODY", "$env:WINFORGE_TOAST_LAUNCH", ".InnerText ="} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, never := range []string{"%s", "Invoke-Expression", "iex ", "Invoke-WebRequest", "Invoke-RestMethod", "Start-Process", "http://", "https://"} {
		if strings.Contains(s, never) {
			t.Errorf("the toast script must not contain %q: it makes no network calls and runs nothing", never)
		}
	}
}

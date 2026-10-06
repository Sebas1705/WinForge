package install_test

import (
	"context"
	"testing"

	"github.com/Sebas1705/WinForge/internal/install"
)

func TestExplainNamesWhatThePersonCanDo(t *testing.T) {
	cases := []struct {
		name  string
		code  int
		lines []string
		want  string
	}{
		{"epic games as seen in the field", int(int32(-0x75EAFFB7 + 0)), []string{"El instalador solicitará que se ejecute como administrador.", "Error del instalador con el código de salida: 1603"}, install.ReasonAdmin},
		{"elevation required", 1, []string{"Installer failed with exit code: 740"}, install.ReasonAdmin},
		{"another installer running", 1, []string{"Installer failed with exit code: 1618"}, install.ReasonBusy},
		{"closed the installer", 1, []string{"Installer failed with exit code: 1602"}, install.ReasonCancelled},
		{"restart needed", 1, []string{"Installer failed with exit code: 3010"}, install.ReasonReboot},
		{"download failed", int(int32(-0x75EAFFF8)), nil, install.ReasonNetwork},
		{"hash mismatch", int(int32(-0x75EAFFEF)), nil, install.ReasonHash},
		{"unknown installer failure", 1, []string{"Installer failed with exit code: 9999"}, install.ReasonInstaller},
		{"nothing to go on", 1, []string{"hello"}, ""},
	}
	for _, c := range cases {
		if got := install.Explain(c.code, c.lines); got != c.want {
			t.Errorf("%s: Explain = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParsePercentReadsWingetProgress(t *testing.T) {
	for line, want := range map[string]int{
		"  ██████████▒▒▒▒▒▒▒▒  52%": 52,
		"  10.0 MB / 40.0 MB": 25,
		"  512 KB / 1.00 MB":  50,
		"  1,5 GB / 3,0 GB":   50,
		"Found Git [Git.Git]": -1,
		"  100%":              100,
		"  250%":              -1,
	} {
		if got := install.ParsePercent(line); got != want {
			t.Errorf("ParsePercent(%q) = %d, want %d", line, got, want)
		}
	}
}

func TestFailedStepCarriesItsReasonAndProgressIsParsed(t *testing.T) {
	cat, p := plan(t, "essentials", nil)
	ex := &fakeExec{exit: map[string]int{"7zip.7zip": 0x8A150049}}
	var failedEv, progress install.Event
	install.Run(context.Background(), cat, p, ex, func(e install.Event) {
		if e.Status == install.StatusFailed {
			failedEv = e
		}
	})
	// The fake prints "ok" only; give it a real-looking failure to explain.
	ex2 := &lineExec{lines: []string{"  35%", "Installer failed with exit code: 1603"}, code: 0x8A150049}
	install.Run(context.Background(), cat, p, ex2, func(e install.Event) {
		if e.Status == install.StatusFailed {
			failedEv = e
		}
		if e.Percent == 35 {
			progress = e
		}
	})
	if failedEv.Reason != install.ReasonAdmin {
		t.Fatalf("1603 should read as needing administrator rights: %+v", failedEv)
	}
	if progress.Percent != 35 {
		t.Fatal("download percent was not reported on the output event")
	}
}

type lineExec struct {
	lines []string
	code  int
}

func (l *lineExec) Run(_ context.Context, _ string, _ []string, onLine func(string)) (int, error) {
	for _, s := range l.lines {
		onLine(s)
	}
	return l.code, nil
}

func TestRebootExitCodeIsSuccessWithAReason(t *testing.T) {
	cat, p := plan(t, "essentials", nil)
	ex := &lineExec{lines: []string{"done"}, code: 3010}
	var reboot bool
	failed := install.Run(context.Background(), cat, p, ex, func(e install.Event) { reboot = reboot || e.Reason == install.ReasonReboot })
	if len(failed) != 0 || !reboot {
		t.Fatalf("3010 means finished, restart to complete: failed=%v reboot=%v", failed, reboot)
	}
}

func TestUninstallPlanOnlyNamesCatalogApps(t *testing.T) {
	cat, _ := plan(t, "essentials", nil)
	p := install.UninstallPlan(cat, []string{"git", "not-in-the-catalog"})
	if len(p.Steps) != 1 || p.Steps[0].Kind != install.StepUninstall || p.Steps[0].ID != "git" {
		t.Fatalf("%+v", p.Steps)
	}
	ex := &fakeExec{}
	install.Run(context.Background(), cat, p, ex, func(install.Event) {})
	if len(ex.calls) != 1 || !contains(ex.calls[0], "uninstall --id Git.Git") {
		t.Fatalf("calls: %v", ex.calls)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

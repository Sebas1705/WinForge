package sched

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/system"
)

func cli(t *testing.T, r *Runner, args ...string) (code int, out, errw string) {
	t.Helper()
	var o, e bytes.Buffer
	code = RunCLI(args, &o, &e, Deps{Runner: r})
	return code, o.String(), e.String()
}

func TestHelpDocumentsEverythingThatMatters(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		code, out, _ := cli(t, nil, flag)
		if code != ExitOK || out != Help {
			t.Fatalf("%s: %d", flag, code)
		}
	}
	flat := strings.Join(strings.Fields(Help), " ")
	for _, want := range []string{
		"--mode notify", "--mode apply", "--scheduled",
		"never runs recipes", "never updates WinForge itself", "chose to ignore", "administrator rights",
		"never restarts the PC", "Only one run happens at a time", "45 minutes", "last 30 are kept",
		"Exit codes:", "0 the check", "1 some updates", "2 the command line", "3 another run", "4 winget is missing", "5 the ignore list", "6 the run hit",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("--help does not mention %q", want)
		}
	}
}

func TestCommandLineMistakesAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"--mode"}, {"--mode", "delete"}, {"--mode=notify", "extra"}, {"--nope"}, {"apply"}} {
		x := &fakeExec{}
		r := runner(t, x, &fakeSys{})
		code, out, errw := cli(t, &r, args...)
		if code != ExitUsage || out != "" || !strings.Contains(errw, "usage: winforge-cli updates") {
			t.Errorf("%v: %d %q %q", args, code, out, errw)
		}
		if len(x.calls) != 0 {
			t.Errorf("%v: started something", args)
		}
	}
}

func TestACheckPrintsAndStoresNothing(t *testing.T) {
	sys := &fakeSys{}
	r := runner(t, &fakeExec{}, sys)
	code, out, _ := cli(t, &r)
	if code != ExitOK || !strings.Contains(out, "found") || !strings.Contains(out, "Git") || !strings.Contains(out, "1.0 -> 2.0") {
		t.Errorf("%d\n%s", code, out)
	}
	if len(sys.toasts) != 0 {
		t.Error("only --scheduled raises a notification")
	}
	if runs, _ := r.Logs.List(); len(runs) != 0 {
		t.Error("only --scheduled writes the log")
	}
}

func TestScheduledRunsLogAndNotify(t *testing.T) {
	sys := &fakeSys{}
	r := runner(t, &fakeExec{}, sys)
	code, _, _ := cli(t, &r, "--scheduled", "--mode", "notify")
	if code != ExitOK || len(sys.toasts) != 1 {
		t.Fatalf("%d %v", code, sys.toasts)
	}
	if runs, _ := r.Logs.List(); len(runs) != 1 {
		t.Error("logged")
	}
}

func TestExitCodes(t *testing.T) {
	// failures
	r := runner(t, &fakeExec{exit: map[string]int{"Git.Git": 1}}, &fakeSys{})
	if code, _, _ := cli(t, &r, "--mode", "apply"); code != ExitFailures {
		t.Errorf("failed update: %d", code)
	}
	// busy
	r = runner(t, &fakeExec{}, &fakeSys{})
	release, _ := r.Lock.Acquire()
	code, out, errw := cli(t, &r, "--scheduled", "--mode", "apply")
	release()
	if code != ExitBusy || out != "" || !strings.Contains(errw, "already in progress") {
		t.Errorf("busy: %d %q %q", code, out, errw)
	}
	// winget missing
	r = runner(t, &fakeExec{}, &fakeSys{})
	r.Env.Upgrades = func(context.Context) ([]system.Upgrade, error) { return nil, system.ErrNoWinget }
	if code, _, _ := cli(t, &r); code != ExitCheck {
		t.Errorf("no winget: %d", code)
	}
}

func TestTheDesktopProgramAnswersTheSameCommand(t *testing.T) {
	if !IsUpdatesCommand([]string{`C:\x\WinForge.exe`, "updates", "--scheduled"}) || !IsUpdatesCommand([]string{"x", "UPDATES"}) {
		t.Error("`WinForge.exe updates ...` is the headless run")
	}
	for _, args := range [][]string{{"x"}, {"x", "scan"}, {"x", "winforge://updates"}, nil} {
		if IsUpdatesCommand(args) {
			t.Errorf("%v", args)
		}
	}
}

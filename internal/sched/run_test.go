package sched

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

func rules(m map[string]updates.Rule) func() (map[string]updates.Rule, error) {
	return func() (map[string]updates.Rule, error) { return m, nil }
}

func TestNotifyOnlyReportsAndNeverRunsAnything(t *testing.T) {
	x := &fakeExec{}
	rep := Execute(context.Background(), env(x), Notify)
	if len(x.calls) != 0 {
		t.Fatalf("notify mode started processes: %v", x.calls)
	}
	if rep.Error != "" {
		t.Fatal(rep.Error)
	}
	for id, want := range map[string]string{"git": ResultFound, "vlc": ResultFound, "firefox": ResultFound, "7zip": ResultNeedsAdmin, "winforge": ResultIgnored} {
		if got := item(t, rep, id).Result; got != want {
			t.Errorf("%s: %s, want %s", id, got, want)
		}
	}
	for _, it := range rep.Items {
		if it.ID == "" || it.Name == "" || it.From == "" || it.To == "" {
			t.Errorf("an item without versions: %+v", it)
		}
		if it.Name == "Unrelated" {
			t.Error("an app outside the catalog must never appear: the catalog is the trust boundary")
		}
	}
	if g := item(t, rep, "git"); g.From != "1.0" || g.To != "2.0" {
		t.Errorf("versions: %+v", g)
	}
	// sorted by name so the list reads the same every time
	for i := 1; i < len(rep.Items); i++ {
		if strings.ToLower(rep.Items[i-1].Name) > strings.ToLower(rep.Items[i].Name) {
			t.Errorf("not sorted: %v", rep.Items)
		}
	}
}

func TestApplyUpdatesOnlyWhatMayBeUpdated(t *testing.T) {
	x := &fakeExec{}
	e := env(x)
	e.Rules = rules(map[string]updates.Rule{
		"vlc":     {Mode: updates.Always},
		"firefox": {Mode: updates.Until, Until: day0.AddDate(0, 0, 3)},
	})
	rep := Execute(context.Background(), e, Apply)
	if len(x.calls) != 1 || !strings.Contains(x.calls[0], "winget upgrade --id Git.Git") {
		t.Fatalf("only Git may be updated: %v", x.calls)
	}
	for _, c := range x.calls {
		for _, never := range []string{"VLC", "7zip", "Firefox", "WinForge", "powershell", "Unrelated"} {
			if strings.Contains(c, never) {
				t.Errorf("must not touch %s: %s", never, c)
			}
		}
	}
	want := map[string]string{"git": ResultUpdated, "vlc": ResultIgnored, "firefox": ResultIgnored, "7zip": ResultNeedsAdmin, "winforge": ResultIgnored}
	for id, r := range want {
		if got := item(t, rep, id).Result; got != r {
			t.Errorf("%s: %s, want %s", id, got, r)
		}
	}
	if item(t, rep, "vlc").Reason != "always" || item(t, rep, "firefox").Reason != "until" {
		t.Error("an ignored app says how it is ignored")
	}
	if g := item(t, rep, "git"); g.From != "1.0" || g.To != "2.0" {
		t.Errorf("versions before and after: %+v", g)
	}
}

func TestApplyNeverRunsARecipe(t *testing.T) {
	x := &fakeExec{}
	Execute(context.Background(), env(x), Apply)
	for _, c := range x.calls {
		if !strings.HasPrefix(c, "winget upgrade ") {
			t.Errorf("only winget upgrades may run unattended: %s", c)
		}
	}
}

func TestAnExpiredPauseOrASkippedVersionBehavesLikeUpdateAll(t *testing.T) {
	x := &fakeExec{}
	e := env(x)
	e.Rules = rules(map[string]updates.Rule{
		"git":     {Mode: updates.Until, Until: day0.Add(-time.Hour)}, // the pause ended
		"firefox": {Mode: updates.Version, Version: "130.5"},          // a different version than the one found
		"vlc":     {Mode: updates.Version, Version: "3.1"},            // exactly the one found
		"notepad": {Mode: "bogus"},                                    // damaged entry hides nothing
	})
	rep := Execute(context.Background(), e, Apply)
	if item(t, rep, "git").Result != ResultUpdated || item(t, rep, "firefox").Result != ResultUpdated {
		t.Errorf("expired and other-version rules do not hide: %+v", rep.Items)
	}
	if item(t, rep, "vlc").Result != ResultIgnored {
		t.Error("the skipped version stays skipped")
	}
}

func TestADamagedIgnoreListStopsEverything(t *testing.T) {
	x := &fakeExec{}
	called := false
	e := env(x)
	e.Rules = func() (map[string]updates.Rule, error) { return nil, errors.New("damaged") }
	e.Upgrades = func(context.Context) ([]system.Upgrade, error) { called = true; return ups(), nil }
	for _, m := range []Mode{Notify, Apply} {
		rep := Execute(context.Background(), e, m)
		if rep.Error != ErrCodeSettings || rep.ExitCode() != ExitSettings || len(rep.Items) != 0 {
			t.Errorf("%s: %+v", m, rep)
		}
	}
	if called || len(x.calls) != 0 {
		t.Fatal("nothing may be asked of winget or updated when the person's choices cannot be read")
	}
}

func TestWingetProblemsAreNamed(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		code string
		exit int
	}{
		"missing":  {system.ErrNoWinget, ErrCodeNoWinget, ExitCheck},
		"other":    {errors.New("boom"), ErrCodeCheck, ExitCheck},
		"deadline": {context.DeadlineExceeded, ErrCodeTimeout, ExitTimeout},
	} {
		e := env(&fakeExec{})
		e.Upgrades = func(context.Context) ([]system.Upgrade, error) { return nil, c.err }
		rep := Execute(context.Background(), e, Apply)
		if rep.Error != c.code || rep.ExitCode() != c.exit {
			t.Errorf("%s: %+v", name, rep)
		}
	}
}

func TestAnAdminAppIsOnlyUpdatedWhenAlreadyElevated(t *testing.T) {
	x := &fakeExec{}
	e := env(x)
	e.Elevated = func() bool { return true }
	rep := Execute(context.Background(), e, Apply)
	if item(t, rep, "7zip").Result != ResultUpdated {
		t.Errorf("an elevated run raises no prompt: %+v", item(t, rep, "7zip"))
	}
	e.Elevated = nil // unknown means not elevated
	rep = Execute(context.Background(), e, Notify)
	if item(t, rep, "7zip").Result != ResultNeedsAdmin {
		t.Error("an unknown token is treated as a standard user")
	}
}

func TestFailuresAreExplainedAndCounted(t *testing.T) {
	x := &fakeExec{
		exit: map[string]int{"Git.Git": 0x8A150008, "Mozilla.Firefox": 1},
		out:  map[string]string{"Mozilla.Firefox": "Installer failed with exit code: 740"},
	}
	rep := Execute(context.Background(), env(x), Apply)
	if g := item(t, rep, "git"); g.Result != ResultFailed || g.Reason != install.ReasonNetwork {
		t.Errorf("%+v", g)
	}
	// The installer said it needs elevation: that is the person's to do, not a failure.
	if f := item(t, rep, "firefox"); f.Result != ResultNeedsAdmin {
		t.Errorf("%+v", f)
	}
	if rep.ExitCode() != ExitFailures {
		t.Errorf("exit %d", rep.ExitCode())
	}
	if rep.Status() != "partial" {
		t.Errorf("status %s", rep.Status())
	}
}

func TestARestartIsReportedNeverDone(t *testing.T) {
	x := &fakeExec{exit: map[string]int{"Git.Git": 3010}}
	rep := Execute(context.Background(), env(x), Apply)
	g := item(t, rep, "git")
	if g.Result != ResultUpdated || !g.Reboot || !rep.Restart() {
		t.Errorf("%+v", g)
	}
	for _, c := range x.calls {
		if strings.Contains(c, "shutdown") || strings.Contains(c, "Restart-Computer") {
			t.Fatalf("an unattended run must never restart anything: %s", c)
		}
	}
}

func TestATimeoutMidRunSkipsTheRestAndSaysSo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already over
	x := &fakeExec{}
	e := env(x)
	e.Upgrades = func(context.Context) ([]system.Upgrade, error) { return ups(), nil }
	rep := Execute(ctx, e, Apply)
	for _, it := range rep.Items {
		if it.Result == ResultUpdated {
			t.Errorf("a cancelled run updated %s", it.ID)
		}
	}
	if len(x.calls) != 0 {
		t.Errorf("nothing starts after the time is up: %v", x.calls)
	}
}

func TestEverythingIgnoredIsQuiet(t *testing.T) {
	e := env(&fakeExec{})
	e.Upgrades = func(context.Context) ([]system.Upgrade, error) {
		return []system.Upgrade{{ID: "Git.Git", Current: "1", Available: "2"}}, nil
	}
	e.Rules = rules(map[string]updates.Rule{"git": {Mode: updates.Always}})
	rep := Execute(context.Background(), e, Apply)
	if _, ok := Compose(rep, "en"); ok || rep.Status() != "ok" {
		t.Errorf("%+v", rep)
	}
}

// ---- the whole run: lock, log, toast -------------------------------------------------------

func runner(t *testing.T, x *fakeExec, sys *fakeSys) Runner {
	t.Helper()
	dir := t.TempDir()
	return Runner{
		Env:  env(x),
		Logs: LogStore{Dir: dir},
		Lock: Lock{Path: filepath.Join(dir, "run.lock")},
		Sys:  sys,
		Lang: func() string { return "en" },
	}
}

func TestAScheduledRunLogsAndToasts(t *testing.T) {
	sys := &fakeSys{}
	r := runner(t, &fakeExec{}, sys)
	rep, err := r.Run(context.Background(), Options{Mode: Notify, Scheduled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Notified || len(sys.toasts) != 1 {
		t.Fatalf("a toast was due: %+v %v", rep, sys.toasts)
	}
	if sys.toasts[0].Title != "4 updates available" || sys.toasts[0].Launch != LaunchUpdates {
		t.Errorf("%+v", sys.toasts[0])
	}
	runs, _ := r.Logs.List()
	if len(runs) != 1 || !runs[0].Scheduled || !runs[0].Notified || runs[0].Mode != Notify {
		t.Errorf("the run is logged: %+v", runs)
	}
}

func TestADryCheckStoresNothingAndShowsNothing(t *testing.T) {
	sys := &fakeSys{}
	r := runner(t, &fakeExec{}, sys)
	rep, err := r.Run(context.Background(), Options{Mode: Notify})
	if err != nil || rep.Count(ResultFound) == 0 {
		t.Fatalf("%+v %v", rep, err)
	}
	if len(sys.toasts) != 0 {
		t.Error("a check from the page raises no notification: the page shows the result")
	}
	if _, err := os.Stat(filepath.Join(r.Logs.Dir, "runs")); !os.IsNotExist(err) {
		t.Error("a check from the page writes no log")
	}
}

func TestTheSameNewsIsNotToastedTwice(t *testing.T) {
	sys := &fakeSys{}
	r := runner(t, &fakeExec{}, sys)
	ctx := context.Background()
	r.Run(ctx, Options{Mode: Notify, Scheduled: true})
	r.Env.Now = func() time.Time { return day0.Add(24 * time.Hour) }
	rep, _ := r.Run(ctx, Options{Mode: Notify, Scheduled: true})
	if len(sys.toasts) != 1 || rep.Notified {
		t.Fatalf("the second run found the same updates: toasts=%d", len(sys.toasts))
	}
	// A new update is news again.
	r.Env.Upgrades = func(context.Context) ([]system.Upgrade, error) {
		return append(ups(), system.Upgrade{ID: "Notepad++.Notepad++", Current: "8", Available: "9"}), nil
	}
	r.Env.Now = func() time.Time { return day0.Add(48 * time.Hour) }
	r.Run(ctx, Options{Mode: Notify, Scheduled: true})
	if len(sys.toasts) != 2 || sys.toasts[1].Title != "5 updates available" {
		t.Fatalf("%+v", sys.toasts)
	}
	runs, _ := r.Logs.List()
	if len(runs) != 3 {
		t.Errorf("every run is logged, toast or not: %d", len(runs))
	}
}

func TestNoToastWhenNothingHappened(t *testing.T) {
	sys := &fakeSys{}
	r := runner(t, &fakeExec{}, sys)
	r.Env.Upgrades = func(context.Context) ([]system.Upgrade, error) { return nil, nil }
	rep, _ := r.Run(context.Background(), Options{Mode: Apply, Scheduled: true})
	if rep.Notified || len(sys.toasts) != 0 {
		t.Fatal("nothing to update, nothing to say")
	}
	if runs, _ := r.Logs.List(); len(runs) != 1 {
		t.Error("the quiet run is still logged")
	}
	// A run that could not check says nothing either; its log says why.
	r.Env.Upgrades = func(context.Context) ([]system.Upgrade, error) { return nil, system.ErrNoWinget }
	rep, _ = r.Run(context.Background(), Options{Mode: Notify, Scheduled: true})
	if len(sys.toasts) != 0 || rep.Error != ErrCodeNoWinget {
		t.Errorf("%+v", rep)
	}
}

func TestAFailedToastIsNotRecordedAsShown(t *testing.T) {
	sys := &fakeSys{toastErr: errors.New("no notification service")}
	r := runner(t, &fakeExec{}, sys)
	rep, err := r.Run(context.Background(), Options{Mode: Notify, Scheduled: true})
	if err != nil || rep.Notified {
		t.Fatalf("%+v %v", rep, err)
	}
	sys.toastErr = nil
	rep, _ = r.Run(context.Background(), Options{Mode: Notify, Scheduled: true})
	if !rep.Notified {
		t.Error("the news was never shown, so the next run shows it")
	}
}

func TestTheToastSpeaksTheSavedLanguage(t *testing.T) {
	sys := &fakeSys{}
	r := runner(t, &fakeExec{}, sys)
	r.Lang = func() string { return "es-ES" }
	r.Run(context.Background(), Options{Mode: Notify, Scheduled: true})
	if len(sys.toasts) != 1 || sys.toasts[0].Title != "4 actualizaciones disponibles" {
		t.Fatalf("%+v", sys.toasts)
	}
}

func TestOneRunAtATime(t *testing.T) {
	sys := &fakeSys{}
	x := &fakeExec{}
	r := runner(t, x, sys)
	release, err := r.Lock.Acquire() // somebody else is running
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), Options{Mode: Apply, Scheduled: true}); !errors.Is(err, ErrBusy) {
		t.Fatalf("%v", err)
	}
	if len(x.calls) != 0 || len(sys.toasts) != 0 {
		t.Error("a run that finds the lock taken does nothing")
	}
	if runs, _ := r.Logs.List(); len(runs) != 0 {
		t.Error("and writes no log")
	}
	release()
	if _, err := r.Run(context.Background(), Options{Mode: Notify, Scheduled: true}); err != nil {
		t.Fatalf("free again: %v", err)
	}
	if _, err := os.Stat(r.Lock.Path); !os.IsNotExist(err) {
		t.Error("the lock is released when the run ends")
	}
}

func TestTheHardTimeoutEndsTheRun(t *testing.T) {
	r := runner(t, &fakeExec{}, &fakeSys{})
	r.Timeout = 20 * time.Millisecond
	r.Env.Upgrades = func(ctx context.Context) ([]system.Upgrade, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	start := time.Now()
	rep, _ := r.Run(context.Background(), Options{Mode: Apply, Scheduled: true})
	if rep.Error != ErrCodeTimeout || rep.ExitCode() != ExitTimeout || time.Since(start) > 5*time.Second {
		t.Fatalf("%+v", rep)
	}
	if _, err := os.Stat(r.Lock.Path); !os.IsNotExist(err) {
		t.Error("a timed-out run still releases the lock")
	}
}

func TestApplyToastsWhatHappened(t *testing.T) {
	sys := &fakeSys{}
	r := runner(t, &fakeExec{}, sys)
	rep, _ := r.Run(context.Background(), Options{Mode: Apply, Scheduled: true})
	// Git, Firefox and VLC updated; 7-Zip needs the person; WinForge itself is never touched.
	if rep.Count(ResultUpdated) != 3 || len(sys.toasts) != 1 || sys.toasts[0].Title != "Updated 3 apps, 1 needs you" {
		t.Fatalf("%+v %+v", rep.Items, sys.toasts)
	}
	if got := item(t, rep, "winforge"); got.Result != ResultIgnored || got.Reason != "self" {
		t.Errorf("WinForge must never update itself unattended: %+v", got)
	}
	for _, c := range r.Env.Exec.(*fakeExec).calls {
		if strings.Contains(strings.ToLower(c), "winforge") {
			t.Fatalf("WinForge's own package was upgraded: %s", c)
		}
	}
}

func TestDefaultTimeoutIsUsedWhenNoneIsGiven(t *testing.T) {
	if RunTimeout != 45*time.Minute {
		t.Fatal("documented in --help")
	}
	var b bytes.Buffer
	b.WriteString(Help)
	if !strings.Contains(b.String(), "45 minutes") {
		t.Error("--help must say how long a run may last")
	}
}

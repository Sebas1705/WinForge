package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// autoFake is a Windows with nothing on it. No test can create a real scheduled task or a toast.
type autoFake struct {
	info      sched.TaskInfo
	created   []sched.Task
	deletes   int
	protocol  string
	unproto   int
	toasts    int
	createErr error
	protoErr  error
}

func (f *autoFake) Query() (sched.TaskInfo, error) { return f.info, nil }
func (f *autoFake) Create(t sched.Task) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, t)
	f.info = sched.TaskInfo{Exists: true, Enabled: true, Settings: t.Settings, Exe: t.Exe, Args: strings.Join(sched.Args(t.Settings.Mode), " ")}
	return nil
}
func (f *autoFake) Delete() error { f.deletes++; f.info = sched.TaskInfo{}; return nil }
func (f *autoFake) Toast(sched.Toast) error {
	f.toasts++
	return nil
}
func (f *autoFake) RegisterProtocol(exe string) error {
	if f.protoErr != nil {
		return f.protoErr
	}
	f.protocol = exe
	return nil
}
func (f *autoFake) UnregisterProtocol() error { f.unproto++; f.protocol = ""; return nil }

const autoDesktop = `C:\Program Files\Sebas1705\WinForge\WinForge.exe`

// autoApp is the Updates page's backend over a fake machine: WinForge.exe is installed, Git and 7-Zip
// have updates, nothing is ignored, and the data folder is a temp dir.
func autoApp(t *testing.T) (*App, *autoFake) {
	t.Helper()
	a, _ := updatesApp(t)
	fake := &autoFake{}
	a.sched = schedSession{
		sys:    fake,
		dir:    filepath.Join(t.TempDir(), "auto-updates"),
		exe:    func() (string, error) { return autoDesktop, nil },
		exists: func(p string) bool { return p == autoDesktop },
		upgrades: func(context.Context) ([]system.Upgrade, error) {
			return []system.Upgrade{{ID: "Git.Git", Current: "1", Available: "2"}, {ID: "7zip.7zip", Current: "23", Available: "24"}}, nil
		},
		args: []string{},
	}
	a.elevated = func() bool { return false }
	return a, fake
}

var autoGood = AutoSettings{Mode: "notify", Frequency: "daily", Time: "09:30"}

func TestAutoStateOffByDefault(t *testing.T) {
	a, _ := autoApp(t)
	st, err := a.AutoState()
	if err != nil {
		t.Fatal(err)
	}
	if st.On || !st.CanEnable || st.Problem != "" || len(st.Runs) != 0 || st.Mode != "notify" || st.Time != "09:00" {
		t.Errorf("%+v", st)
	}
}

func TestAutoRunnerMissingIsSaidNotHidden(t *testing.T) {
	a, fake := autoApp(t)
	a.sched.exists = func(string) bool { return false }
	st, _ := a.AutoState()
	if st.CanEnable || st.Problem != "runner-missing" {
		t.Errorf("%+v", st)
	}
	_, err := a.AutoEnable(autoGood, "en")
	if !errors.Is(err, sched.ErrRunnerMissing) {
		t.Fatalf("%v", err)
	}
	if len(fake.created) != 0 || fake.protocol != "" {
		t.Fatal("a task must not be created without a program to run: it would be broken from the start")
	}
}

func TestAutoEnableBuildsTheTaskFromAllowedChoicesOnly(t *testing.T) {
	a, fake := autoApp(t)
	st, err := a.AutoEnable(AutoSettings{Mode: "apply", Frequency: "weekly", Weekday: 5, Time: "07:05"}, "es")
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.created) != 1 {
		t.Fatalf("%d tasks created", len(fake.created))
	}
	got := fake.created[0]
	want := sched.Task{Exe: autoDesktop, Settings: sched.Settings{Mode: sched.Apply, Frequency: sched.Weekly, Weekday: 5, Hour: 7, Minute: 5}}
	if got != want {
		t.Errorf("%+v, want %+v", got, want)
	}
	if fake.protocol != autoDesktop {
		t.Errorf("the toast's link needs winforge:// to open WinForge: %q", fake.protocol)
	}
	if !st.On || st.Mode != "apply" || st.Frequency != "weekly" || st.Weekday != 5 || st.Time != "07:05" || st.NextRun == "" {
		t.Errorf("%+v", st)
	}
	// the clock is Saturday 2026-11-14 09:00: the next Friday 07:05
	if next, _ := time.Parse(time.RFC3339, st.NextRun); !next.Equal(time.Date(2026, 11, 20, 7, 5, 0, 0, time.UTC)) {
		t.Errorf("next run %v", st.NextRun)
	}
	if lang := sched.Language(a.sched.dir, nil); lang != "es" {
		t.Errorf("the unattended run needs the interface language: %q", lang)
	}
}

func TestAutoEnableRejectsAnythingOutsideTheAllowList(t *testing.T) {
	a, fake := autoApp(t)
	bad := map[string]AutoSettings{
		"mode":          {Mode: "apply --elevate", Frequency: "daily", Time: "09:00"},
		"empty mode":    {Frequency: "daily", Time: "09:00"},
		"frequency":     {Mode: "notify", Frequency: "hourly", Time: "09:00"},
		"weekday":       {Mode: "notify", Frequency: "weekly", Weekday: 9, Time: "09:00"},
		"time":          {Mode: "notify", Frequency: "daily", Time: "9am"},
		"time injected": {Mode: "notify", Frequency: "daily", Time: "09:00\" /RU SYSTEM"},
		"hour":          {Mode: "notify", Frequency: "daily", Time: "25:00"},
		"empty time":    {Mode: "notify", Frequency: "daily"},
	}
	for name, s := range bad {
		if _, err := a.AutoEnable(s, "en"); !errors.Is(err, sched.ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, lang := range []string{"", "auto", "fr", "es-ES", "en; calc"} {
		if _, err := a.AutoEnable(autoGood, lang); !errors.Is(err, sched.ErrInvalid) {
			t.Errorf("language %q: %v", lang, err)
		}
		if err := a.AutoSetLanguage(lang); !errors.Is(err, sched.ErrInvalid) {
			t.Errorf("AutoSetLanguage %q: %v", lang, err)
		}
	}
	if len(fake.created) != 0 || fake.protocol != "" {
		t.Fatal("nothing invalid may reach the machine")
	}
	if _, err := os.Stat(a.sched.dir); !os.IsNotExist(err) {
		t.Error("nothing invalid may create files either")
	}
}

func TestAutoDailyIgnoresAStrayWeekday(t *testing.T) {
	a, fake := autoApp(t)
	if _, err := a.AutoEnable(AutoSettings{Mode: "notify", Frequency: "daily", Weekday: 42, Time: "09:00"}, "en"); err != nil {
		t.Fatal(err)
	}
	if fake.created[0].Settings.Weekday != 0 {
		t.Errorf("%+v", fake.created[0])
	}
}

func TestAutoEnableErrorsAreCodesAndLeaveNothingBehind(t *testing.T) {
	a, fake := autoApp(t)
	fake.createErr = errors.Join(sched.ErrTask, errors.New("access denied"))
	_, err := a.AutoEnable(autoGood, "en")
	if !errors.Is(err, sched.ErrTask) || !strings.HasPrefix(err.Error(), "auto:task-failed") {
		t.Fatalf("%v", err)
	}
	if fake.protocol != "" || fake.unproto != 1 {
		t.Errorf("a failed task must not leave the link handler registered: %q %d", fake.protocol, fake.unproto)
	}
	fake.createErr = nil
	fake.protoErr = errors.New("registry locked")
	if _, err := a.AutoEnable(autoGood, "en"); err == nil || len(fake.created) != 0 {
		t.Errorf("%v %d", err, len(fake.created))
	}
}

func TestAutoDisableRemovesTheTaskAndKeepsTheLog(t *testing.T) {
	a, fake := autoApp(t)
	if _, err := a.AutoEnable(autoGood, "en"); err != nil {
		t.Fatal(err)
	}
	if err := (sched.LogStore{Dir: a.sched.dir}).Save(sched.Report{Started: day0, Mode: sched.Notify, Scheduled: true}); err != nil {
		t.Fatal(err)
	}
	st, err := a.AutoDisable()
	if err != nil {
		t.Fatal(err)
	}
	if st.On || fake.deletes != 1 || fake.unproto != 1 {
		t.Errorf("%+v %d %d", st, fake.deletes, fake.unproto)
	}
	if len(st.Runs) != 1 {
		t.Error("turning it off keeps the log")
	}
	// turning off what is not on is fine
	if _, err := a.AutoDisable(); err != nil {
		t.Errorf("%v", err)
	}
}

func TestAutoStateReadsWhatTheTaskReallyIs(t *testing.T) {
	a, fake := autoApp(t)
	fake.info = sched.TaskInfo{Exists: true, Enabled: false, Foreign: true, Exe: `C:\Gone\WinForge.exe`,
		Settings: sched.Settings{Mode: sched.Apply, Frequency: sched.Daily, Hour: 4, Minute: 0}}
	st, _ := a.AutoState()
	if !st.On || st.Enabled || !st.Foreign || st.Problem != "task-program-missing" || st.NextRun != "" || st.Mode != "apply" || st.Time != "04:00" {
		t.Errorf("a task Windows has switched off, edited elsewhere and pointing at a deleted program: %+v", st)
	}
	fake.info.Exe = "C:\\caf\uFFFD\\WinForge.exe" // mangled by the console: not judged
	if st, _ := a.AutoState(); st.Problem != "" {
		t.Errorf("%+v", st)
	}
}

func TestAutoRunNowIsADryCheckWithTheUpdatePageRules(t *testing.T) {
	a, fake := autoApp(t)
	if err := a.ignored.Set("git", updates.Rule{Mode: updates.Always}); err != nil {
		t.Fatal(err)
	}
	rep, err := a.AutoRunNow()
	if err != nil {
		t.Fatal(err)
	}
	results := map[string]string{}
	for _, it := range rep.Items {
		results[it.ID] = it.Result
	}
	if results["git"] != sched.ResultIgnored || results["7zip"] != sched.ResultNeedsAdmin || len(results) != 2 {
		t.Errorf("%v", results)
	}
	if rep.Mode != sched.Notify || rep.Scheduled || fake.toasts != 0 {
		t.Errorf("a dry check does not update, does not log as a run and does not notify: %+v", rep)
	}
	if runs, _ := (sched.LogStore{Dir: a.sched.dir}).List(); len(runs) != 0 {
		t.Error("nothing stored")
	}
}

func TestAutoRunNowRefusesADamagedIgnoreList(t *testing.T) {
	a, _ := autoApp(t)
	os.WriteFile(a.ignored.Path, []byte("{broken"), 0o644)
	rep, err := a.AutoRunNow()
	if err != nil || rep.Error != sched.ErrCodeSettings || len(rep.Items) != 0 {
		t.Errorf("%+v %v", rep, err)
	}
}

func TestAutoRunNowWaitsForARunInProgress(t *testing.T) {
	a, _ := autoApp(t)
	lock := sched.Lock{Path: filepath.Join(a.sched.dir, "run.lock")}
	release, err := lock.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := a.AutoRunNow(); !errors.Is(err, sched.ErrBusy) || !strings.HasPrefix(err.Error(), "auto:busy") {
		t.Fatalf("%v", err)
	}
}

func TestAutoSetLanguageOnlyFollowsAnExistingSchedule(t *testing.T) {
	a, _ := autoApp(t)
	if err := a.AutoSetLanguage("es"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(a.sched.dir); !os.IsNotExist(err) {
		t.Fatal("starting the app must not create files before the schedule was turned on")
	}
	if _, err := a.AutoEnable(autoGood, "en"); err != nil {
		t.Fatal(err)
	}
	if err := a.AutoSetLanguage("es"); err != nil {
		t.Fatal(err)
	}
	if sched.Language(a.sched.dir, nil) != "es" {
		t.Error("the notification follows the interface language")
	}
}

func TestLaunchTarget(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"winforge://updates"}, "updates"},
		{[]string{"winforge://updates/"}, "updates"},
		{[]string{"WINFORGE://Updates"}, "updates"},
		{[]string{"--flag", "winforge://updates"}, "updates"},
		{[]string{"winforge://uninstall"}, ""},
		{[]string{"winforge://updates?x=1"}, ""},
		{[]string{"https://winforge://updates"}, ""},
		{[]string{"updates"}, ""},
	} {
		if got := launchTargetFrom(c.args); got != c.want {
			t.Errorf("%v -> %q", c.args, got)
		}
	}
	a, _ := autoApp(t)
	a.sched.args = []string{"winforge://updates"}
	if a.LaunchTarget() != "updates" {
		t.Error("the app reads its own command line")
	}
}

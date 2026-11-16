package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/system"
)

// schedSession is what the Updates page's "Automatic updates" card talks to. The fields are
// replaced by tests (nil means the real machine), so no test ever creates a task or a toast.
type schedSession struct {
	op       sync.Mutex // one change of the schedule at a time
	sys      sched.System
	dir      string
	exe      func() (string, error)
	exists   func(string) bool
	upgrades func(context.Context) ([]system.Upgrade, error)
	args     []string // the command line, for LaunchTarget
}

// AutoSettings is what the person chooses on the card.
type AutoSettings struct {
	Mode      string `json:"mode"`      // "notify" or "apply"
	Frequency string `json:"frequency"` // "daily" or "weekly"
	Weekday   int    `json:"weekday"`   // 0 = Sunday ... 6; only for weekly
	Time      string `json:"time"`      // "HH:MM", 24 hours
}

// AutoState is what the card shows. Problem is a code the interface words: "runner-missing" when no
// program is there to run the task (so it cannot be turned on), "task-program-missing" when a task
// exists but its program is gone.
type AutoState struct {
	On        bool           `json:"on"`
	Enabled   bool           `json:"enabled"` // the task exists but Windows has it switched off when false
	Foreign   bool           `json:"foreign"` // the task was changed outside WinForge
	Mode      string         `json:"mode"`
	Frequency string         `json:"frequency"`
	Weekday   int            `json:"weekday"`
	Time      string         `json:"time"`
	NextRun   string         `json:"nextRun,omitempty"` // RFC 3339
	CanEnable bool           `json:"canEnable"`
	Problem   string         `json:"problem,omitempty"`
	Runs      []sched.Report `json:"runs"`
}

func (s *schedSession) system() sched.System {
	if s.sys != nil {
		return s.sys
	}
	return sched.NewSystem()
}

func (s *schedSession) dataDir() (string, error) {
	if s.dir != "" {
		return s.dir, nil
	}
	d, err := sched.DefaultDir()
	if err != nil {
		return "", fmt.Errorf("%w: %v", sched.ErrTask, err)
	}
	return d, nil
}

func (s *schedSession) desktopExe() (string, error) {
	if s.exe != nil {
		return s.exe()
	}
	return os.Executable()
}

func (s *schedSession) fileExists(p string) bool {
	if s.exists != nil {
		return s.exists(p)
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (s *schedSession) runner() (string, error) {
	exe, err := s.desktopExe()
	if err != nil {
		return "", sched.ErrRunnerMissing
	}
	return sched.RunnerPath(exe, s.fileExists)
}

// AutoState reads the scheduled task and the last runs. Nothing is changed.
func (a *App) AutoState() (AutoState, error) {
	info, err := a.sched.system().Query()
	if err != nil {
		return AutoState{}, err
	}
	dir, err := a.sched.dataDir()
	if err != nil {
		return AutoState{}, err
	}
	runs, err := sched.LogStore{Dir: dir}.List()
	if err != nil {
		return AutoState{}, fmt.Errorf("%w: %v", sched.ErrTask, err)
	}
	st := AutoState{Runs: runs, Mode: string(sched.Notify), Frequency: string(sched.Daily), Time: "09:00"}
	if _, err := a.sched.runner(); err == nil {
		st.CanEnable = true
	}
	if !info.Exists {
		if !st.CanEnable {
			st.Problem = "runner-missing"
		}
		return st, nil
	}
	st.On, st.Enabled, st.Foreign = true, info.Enabled, info.Foreign
	st.Mode, st.Frequency, st.Weekday, st.Time = string(info.Settings.Mode), string(info.Settings.Frequency), info.Settings.Weekday, info.Settings.Clock()
	if info.Enabled && !info.Foreign {
		st.NextRun = info.Settings.NextRun(a.clock()).Format(time.RFC3339)
	}
	// A path with characters the console may have mangled is not judged.
	if p := info.Exe; p != "" && !strings.ContainsAny(p, "?�") && !a.sched.fileExists(p) {
		st.Problem = "task-program-missing"
	}
	return st, nil
}

func (s AutoSettings) settings() (sched.Settings, error) {
	h, m, err := sched.ParseClock(s.Time)
	if err != nil {
		return sched.Settings{}, err
	}
	out := sched.Settings{Mode: sched.Mode(s.Mode), Frequency: sched.Frequency(s.Frequency), Weekday: s.Weekday, Hour: h, Minute: m}
	if out.Frequency == sched.Daily {
		out.Weekday = 0
	}
	return out, out.Validate()
}

// AutoEnable creates (or replaces) the per-user scheduled task. lang is the language the interface
// shows now ("es" or "en"); the unattended run words its notification in it. Only the allowed
// choices reach the task, and the program it starts is found, never taken from the interface.
func (a *App) AutoEnable(s AutoSettings, lang string) (AutoState, error) {
	set, err := s.settings()
	if err != nil {
		return AutoState{}, err
	}
	if lang != "es" && lang != "en" {
		return AutoState{}, fmt.Errorf("%w: language %q", sched.ErrInvalid, lang)
	}
	a.sched.op.Lock()
	defer a.sched.op.Unlock()
	runner, err := a.sched.runner()
	if err != nil {
		return AutoState{}, err
	}
	dir, err := a.sched.dataDir()
	if err != nil {
		return AutoState{}, err
	}
	if err := sched.SaveLanguage(dir, lang, false); err != nil {
		return AutoState{}, fmt.Errorf("%w: %v", sched.ErrTask, err)
	}
	sys := a.sched.system()
	desktop, _ := a.sched.desktopExe()
	if err := sys.RegisterProtocol(desktop); err != nil {
		return AutoState{}, err
	}
	if err := sys.Create(sched.Task{Exe: runner, Settings: set}); err != nil {
		_ = sys.UnregisterProtocol()
		return AutoState{}, err
	}
	return a.AutoState()
}

// AutoDisable removes the scheduled task. The log of the runs stays.
func (a *App) AutoDisable() (AutoState, error) {
	a.sched.op.Lock()
	defer a.sched.op.Unlock()
	sys := a.sched.system()
	if err := sys.Delete(); err != nil {
		return AutoState{}, err
	}
	_ = sys.UnregisterProtocol()
	return a.AutoState()
}

// AutoRunNow checks for updates right now with the same rules as a scheduled run, and returns
// what it found. It is a dry check: nothing is updated, nothing is logged, no notification is shown.
func (a *App) AutoRunNow() (sched.Report, error) {
	dir, err := a.sched.dataDir()
	if err != nil {
		return sched.Report{}, err
	}
	ups := a.sched.upgrades
	if ups == nil {
		ups = system.WingetUpgrades
	}
	elevated := a.elevated
	if elevated == nil {
		elevated = system.IsElevated
	}
	r := sched.Runner{
		Env:  sched.Env{Cat: a.cat, Upgrades: ups, Rules: a.ignoreRules, Now: a.clock, Elevated: elevated},
		Logs: sched.LogStore{Dir: dir},
		Lock: sched.Lock{Path: dir + string(os.PathSeparator) + "run.lock"},
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return r.Run(ctx, sched.Options{Mode: sched.Notify})
}

// AutoSetLanguage keeps the language of the scheduled notification in step with the interface. It
// changes nothing until the schedule has been turned on once.
func (a *App) AutoSetLanguage(lang string) error {
	if lang != "es" && lang != "en" {
		return fmt.Errorf("%w: language %q", sched.ErrInvalid, lang)
	}
	dir, err := a.sched.dataDir()
	if err != nil {
		return err
	}
	return sched.SaveLanguage(dir, lang, true)
}

// LaunchTarget is the page a link or a notification asked WinForge to open: "updates", or "" for
// the normal start. Only the one known link is understood.
func (a *App) LaunchTarget() string {
	args := a.sched.args
	if args == nil && len(os.Args) > 1 {
		args = os.Args[1:]
	}
	return launchTargetFrom(args)
}

func launchTargetFrom(args []string) string {
	for _, arg := range args {
		if strings.EqualFold(strings.TrimRight(arg, "/"), sched.LaunchUpdates) {
			return "updates"
		}
	}
	return ""
}

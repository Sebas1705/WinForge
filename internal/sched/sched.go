// Package sched runs WinForge's scheduled automatic updates: it builds and reads the per-user
// Windows Scheduled Task, runs the unattended check (and, when the person chose it, the update),
// keeps a log of the last runs and raises a toast.
//
// Everything that touches Windows (the task, the toast, the URL protocol) sits behind System, so the
// rules here run in tests against a fake and on any operating system.
package sched

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TaskName is the scheduled task's name, in the root folder of the person's own tasks.
const TaskName = "WinForge updates"

// Mode says what a scheduled run does.
type Mode string

const (
	// Notify only checks and tells the person (the default).
	Notify Mode = "notify"
	// Apply also updates the catalog apps that may be updated without asking.
	Apply Mode = "apply"
)

// Frequency is how often the task starts.
type Frequency string

const (
	Daily  Frequency = "daily"
	Weekly Frequency = "weekly"
)

// Settings is everything the person chooses. It is validated before it reaches the task.
type Settings struct {
	Mode      Mode      `json:"mode"`
	Frequency Frequency `json:"frequency"`
	Weekday   int       `json:"weekday"` // 0 = Sunday ... 6 = Saturday; only used when weekly
	Hour      int       `json:"hour"`
	Minute    int       `json:"minute"`
}

// Errors the interface recognises by their code (the text before the first space). The words after
// it are for the log; the interface words every message itself, in the person's language.
var (
	ErrInvalid       = errors.New("auto:invalid")
	ErrRunnerMissing = errors.New("auto:runner-missing")
	ErrUnsupported   = errors.New("auto:unsupported")
	ErrTask          = errors.New("auto:task-failed")
	ErrBusy          = errors.New("auto:busy")
)

// Validate rejects anything that is not one of the allowed choices. The task is built only from a
// validated Settings, so nothing the interface sends can reach the command line as text.
func (s Settings) Validate() error {
	if s.Mode != Notify && s.Mode != Apply {
		return fmt.Errorf("%w: mode %q", ErrInvalid, s.Mode)
	}
	if s.Frequency != Daily && s.Frequency != Weekly {
		return fmt.Errorf("%w: frequency %q", ErrInvalid, s.Frequency)
	}
	if s.Frequency == Weekly && (s.Weekday < 0 || s.Weekday > 6) {
		return fmt.Errorf("%w: weekday %d", ErrInvalid, s.Weekday)
	}
	if s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59 {
		return fmt.Errorf("%w: time %02d:%02d", ErrInvalid, s.Hour, s.Minute)
	}
	return nil
}

// ParseClock reads "HH:MM" (24 hours).
func ParseClock(v string) (hour, minute int, err error) {
	h, m, ok := strings.Cut(v, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, 0, fmt.Errorf("%w: time %q", ErrInvalid, v)
	}
	hour, e1 := strconv.Atoi(h)
	minute, e2 := strconv.Atoi(m)
	if e1 != nil || e2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("%w: time %q", ErrInvalid, v)
	}
	return hour, minute, nil
}

// Clock is "HH:MM".
func (s Settings) Clock() string { return fmt.Sprintf("%02d:%02d", s.Hour, s.Minute) }

// NextRun is the next moment the schedule fires at or after now, in now's time zone.
// "Run as soon as possible after a missed start" can make the real run earlier; the person
// is shown this as the next scheduled time.
func (s Settings) NextRun(now time.Time) time.Time {
	at := time.Date(now.Year(), now.Month(), now.Day(), s.Hour, s.Minute, 0, 0, now.Location())
	if s.Frequency == Weekly {
		at = at.AddDate(0, 0, (s.Weekday-int(at.Weekday())+7)%7)
	}
	for at.Before(now) {
		if s.Frequency == Weekly {
			at = at.AddDate(0, 0, 7)
		} else {
			at = at.AddDate(0, 0, 1)
		}
	}
	return at
}

// Args is the command line the task passes to the runner. Every word is a constant or one of the
// validated mode names, so it needs no quoting.
func Args(mode Mode) []string {
	return []string{"updates", "--scheduled", "--mode", string(mode)}
}

// ModeFromArgs reads the mode back out of a task's arguments. ok is false when the arguments are
// not the ones WinForge writes (the person edited the task elsewhere).
func ModeFromArgs(args string) (Mode, bool) {
	f := strings.Fields(args)
	for _, m := range []Mode{Notify, Apply} {
		want := Args(m)
		if len(f) == len(want) {
			same := true
			for i := range f {
				same = same && f[i] == want[i]
			}
			if same {
				return m, true
			}
		}
	}
	return Notify, false
}

// Task is what gets registered.
type Task struct {
	Exe      string // the program the task starts
	Settings Settings
}

// TaskInfo is what the machine says about the task.
type TaskInfo struct {
	Exists   bool
	Enabled  bool
	Settings Settings
	Exe      string
	Args     string
	// Foreign is true when the task exists but does not look like what WinForge writes (a different
	// schedule type or arguments), so the page shows it as it is and does not pretend to own it.
	Foreign bool
}

// Toast is one notification. Launch is a URL the click opens; it is always a constant of this package.
type Toast struct {
	Title  string
	Body   string
	Launch string
}

// LaunchUpdates is what clicking the toast opens: WinForge, on the Updates page.
const LaunchUpdates = "winforge://updates"

// System is everything that touches Windows. Tests use a fake; the real one is in system_windows.go.
type System interface {
	// Query describes the task; Exists is false (and err nil) when there is none.
	Query() (TaskInfo, error)
	// Create registers the task for the signed-in user, replacing a task of the same name.
	Create(Task) error
	// Delete removes the task. A task that is not there is not an error.
	Delete() error
	// Toast shows a notification.
	Toast(Toast) error
	// RegisterProtocol lets winforge:// links start exe; UnregisterProtocol undoes it.
	RegisterProtocol(exe string) error
	UnregisterProtocol() error
}

// RunnerPath finds the program the task should start. The installer lays down WinForge.exe only
// (winforge-cli.exe is not shipped), and WinForge.exe answers `updates --scheduled` itself, headless:
// it is a Windows-subsystem program, so the task never flashes a console window. winforge-cli.exe
// next to the desktop app is the fallback for a build whose desktop program has another name.
// Anything else is refused: a development build in a temporary folder must not become a task that
// breaks the day the folder is cleaned.
func RunnerPath(desktopExe string, exists func(string) bool) (string, error) {
	if desktopExe == "" || strings.ContainsAny(desktopExe, "\"\r\n\x00") {
		return "", ErrRunnerMissing
	}
	sep := strings.LastIndexAny(desktopExe, `\/`)
	dir, base := desktopExe[:sep+1], desktopExe[sep+1:]
	if strings.EqualFold(base, "winforge.exe") && exists(desktopExe) {
		return desktopExe, nil
	}
	if cli := dir + "winforge-cli.exe"; exists(cli) {
		return cli, nil
	}
	return "", ErrRunnerMissing
}

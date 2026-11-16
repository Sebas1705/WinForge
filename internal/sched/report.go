package sched

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// KeepRuns is how many runs the log keeps.
const KeepRuns = 30

// What happened to one app. The interface words each of them.
const (
	ResultFound      = "found"       // an update exists (a check, or a run that only tells)
	ResultUpdated    = "updated"     // updated by this run
	ResultFailed     = "failed"      // the update was tried and did not work; Reason says why
	ResultIgnored    = "ignored"     // the person chose not to update it (skip, pause or never)
	ResultNeedsAdmin = "needs-admin" // its update needs administrator rights, so it waits for the person
	ResultSkipped    = "skipped"     // not tried (the run ended first, or an app it depends on failed)
)

// Run-level problems, as codes the interface words. Empty means the run itself worked.
const (
	ErrCodeNoWinget  = "no-winget"        // winget is not available
	ErrCodeCheck     = "check-failed"     // winget could not list the updates
	ErrCodeSettings  = "settings-damaged" // the ignore list is unreadable, so nothing was touched
	ErrCodeTimeout   = "timeout"          // the run hit its time limit
	ErrCodeNoCatalog = "no-catalog"       // the embedded catalog could not be read
)

// Item is one app in a run, with the versions before and after.
type Item struct {
	ID     string `json:"id"` // catalog id
	Name   string `json:"name"`
	From   string `json:"from"`             // the installed version when the run looked
	To     string `json:"to"`               // the version available (and, for updated, the one asked for)
	Result string `json:"result"`           // one of the Result constants
	Reason string `json:"reason,omitempty"` // for failed: install.Explain's reason; for ignored: the mode
	Reboot bool   `json:"reboot,omitempty"` // updated, but Windows has to restart to finish it
}

// Report is the log of one run.
type Report struct {
	ID        string    `json:"id"` // file name without extension: the start time
	Started   time.Time `json:"started"`
	Finished  time.Time `json:"finished"`
	Mode      Mode      `json:"mode"`
	Scheduled bool      `json:"scheduled"`
	Error     string    `json:"error,omitempty"` // one of the ErrCode constants
	Items     []Item    `json:"items"`
	// Notified is true when this run raised a toast; Fingerprint says what it was about, so the same
	// news is not announced again on every run.
	Notified    bool   `json:"notified,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Count is how many items have the result.
func (r Report) Count(result string) int {
	n := 0
	for _, it := range r.Items {
		if it.Result == result {
			n++
		}
	}
	return n
}

// Restart reports whether an updated app asks for a restart. Nothing here ever restarts the PC.
func (r Report) Restart() bool {
	for _, it := range r.Items {
		if it.Result == ResultUpdated && it.Reboot {
			return true
		}
	}
	return false
}

// Status summarises a run for the list: "error", "failed", "partial", "attention" or "ok".
func (r Report) Status() string {
	updated, failed := r.Count(ResultUpdated), r.Count(ResultFailed)
	switch {
	case r.Error != "":
		return "error"
	case failed > 0 && updated > 0:
		return "partial"
	case failed > 0:
		return "failed"
	case r.Count(ResultNeedsAdmin) > 0 || (r.Mode == Notify && r.Count(ResultFound) > 0):
		return "attention"
	}
	return "ok"
}

// fingerprint says what is worth a toast: the apps found, waiting for the person, updated or failed,
// each with the version it is about. Ignored and skipped apps are not news.
func (r Report) fingerprint() string {
	var parts []string
	for _, it := range r.Items {
		switch it.Result {
		case ResultFound, ResultNeedsAdmin, ResultUpdated, ResultFailed:
			parts = append(parts, it.Result+":"+it.ID+"@"+it.To)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// LogStore keeps the last KeepRuns reports, one JSON file each, under Dir.
type LogStore struct{ Dir string }

// DefaultDir is %AppData%\WinForge\auto-updates.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "WinForge", "auto-updates"), nil
}

func (s LogStore) runs() string { return filepath.Join(s.Dir, "runs") }

// Save writes the report (named after its start time) and drops the oldest ones beyond KeepRuns.
func (s LogStore) Save(r Report) error {
	if r.ID == "" {
		r.ID = r.Started.UTC().Format("20060102T150405Z")
	}
	if err := os.MkdirAll(s.runs(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.runs(), r.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return s.prune()
}

func (s LogStore) names() ([]string, error) {
	entries, err := os.ReadDir(s.runs())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // the names are start times, so this is oldest first
	return names, nil
}

func (s LogStore) prune() error {
	names, err := s.names()
	if err != nil {
		return err
	}
	for len(names) > KeepRuns {
		if err := os.Remove(filepath.Join(s.runs(), names[0])); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		names = names[1:]
	}
	return nil
}

// List returns the kept runs, newest first. A file that cannot be read is skipped: a damaged log
// entry must not hide the rest.
func (s LogStore) List() ([]Report, error) {
	names, err := s.names()
	if err != nil {
		return nil, err
	}
	out := make([]Report, 0, len(names))
	for i := len(names) - 1; i >= 0; i-- {
		b, err := os.ReadFile(filepath.Join(s.runs(), names[i]))
		if err != nil {
			continue
		}
		var r Report
		if json.Unmarshal(b, &r) != nil {
			continue
		}
		if r.ID == "" {
			r.ID = strings.TrimSuffix(names[i], ".json")
		}
		if r.Items == nil {
			r.Items = []Item{}
		}
		out = append(out, r)
	}
	return out, nil
}

// lastNotified is the fingerprint of the newest run that raised a toast, "" when there is none.
func (s LogStore) lastNotified() string {
	runs, _ := s.List()
	for _, r := range runs {
		if r.Notified {
			return r.Fingerprint
		}
	}
	return ""
}

package history

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/Sebas1705/WinForge/internal/install"
)

// Recorder turns the events of a run into history entries.
type Recorder struct {
	Store Store
	Now   func() time.Time // nil means time.Now
	// OnError hears about a store that could not be written. Recording never fails a run.
	OnError func(error)
}

// SourceOf says which part of the app asked for a plan. title is what the app calls the run
// ("updates" for the Updates page); an app plan with a single app is "single", anything larger
// is a profile or a selection.
func SourceOf(plan install.Plan, title string) Source {
	if title == "updates" {
		return FromUpdate
	}
	apps := 0
	for _, s := range plan.Steps {
		if s.Kind == install.StepApp || s.Kind == install.StepKit || s.Kind == install.StepUpgrade || s.Kind == install.StepRollback {
			apps++
		}
	}
	if apps == 1 {
		return FromSingle
	}
	return FromProfile
}

// Recording is one run being written down.
type Recording struct {
	rec    *Recorder
	source Source
	before map[string]string
	added  []Entry
	// OnEntry is called with each entry once it is stored.
	OnEntry func(Entry)
}

// Begin starts recording a run. before maps catalog ids to the version they had before the run
// (absent or "" when unknown or not installed); the caller reads it before anything runs. A nil
// Recorder or Store records nothing, so every method of a nil *Recording is safe.
func (r *Recorder) Begin(plan install.Plan, title string, before map[string]string) *Recording {
	if r == nil || r.Store == nil {
		return nil
	}
	return &Recording{rec: r, source: SourceOf(plan, title), before: before}
}

func actionOf(k install.StepKind) (Action, bool) {
	switch k {
	case install.StepApp, install.StepKit: // an offline kit install is an install
		return Install, true
	case install.StepUpgrade:
		return Update, true
	case install.StepRollback:
		return Rollback, true
	}
	return "", false // recipes and uninstalls are not app history
}

func (r *Recorder) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func newID(at time.Time) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return at.UTC().Format("20060102T150405.000") + "-" + hex.EncodeToString(b[:])
}

// Observe takes one event of the run. Only the end of an app step is written down: a step that
// finished (ok or needing a restart) or failed. Skipped steps did nothing and are not recorded.
func (c *Recording) Observe(e install.Event) {
	if c == nil {
		return
	}
	action, ok := actionOf(e.Step.Kind)
	if !ok || (e.Status != install.StatusOK && e.Status != install.StatusFailed) {
		return
	}
	at := c.rec.now()
	entry := Entry{
		ID: newID(at), App: e.Step.ID, Name: e.Step.Name, Action: action,
		From: c.before[e.Step.ID], To: e.Step.Version, At: at, Source: c.source, Result: OK,
	}
	if action == Install {
		entry.From = "" // an install starts from nothing, whatever an earlier scan said
	}
	switch {
	case e.Status == install.StatusFailed:
		entry.Result, entry.Reason, entry.To = Failed, e.Reason, ""
	case e.Reason == install.ReasonReboot:
		entry.Result = Restart
	}
	if err := c.rec.Store.Add(entry); err != nil {
		if c.rec.OnError != nil {
			c.rec.OnError(err)
		}
		return
	}
	c.added = append(c.added, entry)
	if c.OnEntry != nil {
		c.OnEntry(entry)
	}
}

// NeedsDetect says whether Finish has anything to fill in, so the caller only scans the PC when
// something was installed or updated.
func (c *Recording) NeedsDetect() bool {
	if c == nil {
		return false
	}
	for _, e := range c.added {
		if e.Result != Failed {
			return true
		}
	}
	return false
}

// Finish fills in the version after of every successful entry of the run from detected (catalog
// id to installed version), the detection the Updates and Catalog pages use. An app detected
// without a version keeps what the step said.
func (c *Recording) Finish(detected map[string]string) {
	if c == nil {
		return
	}
	to := map[string]string{}
	for _, e := range c.added {
		if e.Result == Failed {
			continue
		}
		if v := detected[e.App]; v != "" && v != e.To {
			to[e.ID] = v
		}
	}
	if err := c.rec.Store.SetTo(to); err != nil && c.rec.OnError != nil {
		c.rec.OnError(err)
	}
}

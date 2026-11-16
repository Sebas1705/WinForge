// Package tasks lists the third-party scheduled tasks that start with Windows (at log on or at
// startup) and only enables or disables them. It never deletes, runs or edits a task or its
// action. Before the first change of a task the original state is remembered so it can be
// restored. Everything under \Microsoft\, anything Microsoft's or Windows' own, and WinForge's
// own task is hidden: when in doubt a task is hidden, not shown.
//
// The logic here is pure; reading and writing the machine goes through System, which a Windows
// file implements and tests replace with an in-memory fake.
package tasks

import (
	"errors"
	"sort"
	"strings"

	"github.com/Sebas1705/WinForge/internal/origstate"
	"github.com/Sebas1705/WinForge/internal/services"
	"github.com/Sebas1705/WinForge/internal/startup"
)

// OwnTaskName is the scheduled task WinForge's own updater registers; it is never listed.
const OwnTaskName = "WinForge updates"

// Task states as the Task Scheduler numbers them.
const (
	stateUnknown  = 0
	stateDisabled = 1
	stateQueued   = 2
	stateReady    = 3
	stateRunning  = 4
)

// State is what the page shows for a task.
type State string

const (
	Ready    State = "ready"
	Disabled State = "disabled"
	Running  State = "running"
	Unknown  State = "unknown"
)

// StateOf maps the Task Scheduler's state number.
func StateOf(n int) State {
	switch n {
	case stateDisabled:
		return Disabled
	case stateReady, stateQueued:
		return Ready
	case stateRunning:
		return Running
	}
	return Unknown
}

// neverRun is the result code Windows reports for a task that has not run yet (0x41303).
const neverRun int32 = 267011

// Raw is one task as found: its folder (for example `\` or `\Vendor\`), its name and its XML
// definition.
type Raw struct {
	Folder string
	Name   string
	State  int
	XML    string
}

// System is everything the package needs from the machine.
type System interface {
	// List returns the tasks with their definitions. A machine may leave out \Microsoft\ tasks.
	List() ([]Raw, error)
	// LastResults returns the last run result of the tasks, by id (missing ones are left out).
	LastResults(ids []string) map[string]int32
	// SetEnabled enables or disables one task. Nothing else about it changes.
	SetEnabled(id string, enabled bool) error
	// CurrentUser lists the names the signed-in user goes by in task definitions: DOMAIN\user,
	// user and the SID.
	CurrentUser() []string
	// Getenv returns an environment variable, "" when unset.
	Getenv(name string) string
}

// Item is one task the page shows.
type Item struct {
	ID         string   `json:"id"`     // folder + name, e.g. \Vendor\Updater
	Name       string   `json:"name"`   // the task name
	Folder     string   `json:"folder"` // e.g. \ or \Vendor\
	Triggers   []string `json:"triggers"`
	Action     string   `json:"action"` // the program the task runs
	State      State    `json:"state"`
	Enabled    bool     `json:"enabled"`
	LastResult *int32   `json:"lastResult"` // null when it has never run or is unknown
	NeedsAdmin bool     `json:"needsAdmin"` // false only for the signed-in user's own limited tasks
	Caution    bool     `json:"caution"`    // looks like a security product
	Changed    bool     `json:"changed"`    // WinForge changed it and remembers the original
	Original   string   `json:"original"`   // "enabled" or "disabled" when Changed, else ""
}

// Errors the interface recognises by their text: stable codes, not sentences.
var (
	ErrNeedsAdmin   = errors.New("task:needs-admin")
	ErrNotFound     = errors.New("task:not-found")
	ErrProtected    = errors.New("task:protected")
	ErrNeedsConfirm = errors.New("task:needs-confirm") // a security product: ask first
	ErrNoOriginal   = errors.New("task:no-original")
	ErrStore        = errors.New("task:store-failed")
	ErrNotApplied   = errors.New("task:not-applied")
)

// Why a task is hidden. Reasons are for tests and diagnostics, never shown as prose.
const (
	ReasonShown      = ""
	ReasonMicrosoft  = "microsoft"  // in \Microsoft\, or authored by Microsoft/Windows
	ReasonOwn        = "own"        // WinForge's own task
	ReasonNoTrigger  = "no-trigger" // does not start at log on or at startup
	ReasonNoAction   = "no-action"  // nothing the page can name
	ReasonWindows    = "windows"    // runs a Windows program from %WINDIR%
	ReasonUnreadable = "unreadable" // the definition cannot be read
)

// Verdict is whether a task may be shown.
type Verdict struct {
	Show       bool
	Caution    bool
	Reason     string
	Triggers   []string
	Action     string
	NeedsAdmin bool
}

// MakeID is the id of a task: its folder and name.
func MakeID(folder, name string) string {
	f := strings.ReplaceAll(folder, "/", `\`)
	if !strings.HasPrefix(f, `\`) {
		f = `\` + f
	}
	if !strings.HasSuffix(f, `\`) {
		f += `\`
	}
	return f + name
}

// SplitID is the inverse of MakeID.
func SplitID(id string) (folder, name string) {
	i := strings.LastIndexByte(id, '\\')
	if i < 0 {
		return `\`, id
	}
	return id[:i+1], id[i+1:]
}

func inMicrosoftFolder(folder string) bool {
	f := strings.ToLower(strings.TrimRight(strings.ReplaceAll(folder, "/", `\`), `\`))
	return f == `\microsoft` || strings.HasPrefix(f, `\microsoft\`) || f == "microsoft" || strings.HasPrefix(f, `microsoft\`)
}

// launchers run something else named in their arguments: a task using one is the vendor's even
// though the launcher itself lives in %WINDIR%.
func isLauncher(command string, lookup func(string) string) bool {
	return startup.ParseCommand(command, lookup, nil).Launcher
}

func underDir(path, dir string) bool {
	norm := func(s string) string { return strings.TrimRight(strings.ToLower(strings.ReplaceAll(s, "/", `\`)), `\`) }
	d, p := norm(dir), norm(path)
	return d != "" && (p == d || strings.HasPrefix(p, d+`\`))
}

// perUser reports whether the task is the signed-in user's own, limited task: one the user can
// switch without administrator rights.
func perUser(p Parsed, me []string) bool {
	if p.UserID == "" || strings.EqualFold(p.RunLevel, "HighestAvailable") {
		return false
	}
	short := func(s string) string {
		if i := strings.LastIndexByte(s, '\\'); i >= 0 {
			return s[i+1:]
		}
		return s
	}
	for _, id := range me {
		if id == "" {
			continue
		}
		if strings.EqualFold(p.UserID, id) || (strings.Contains(id, `\`) && strings.EqualFold(short(p.UserID), short(id)) && !strings.HasPrefix(strings.ToUpper(p.UserID), "S-1-")) {
			return true
		}
	}
	return false
}

// Classify decides whether a task is shown and what to show about it.
func Classify(r Raw, p Parsed, parseErr error, winDir string, lookup func(string) string, me []string) Verdict {
	hide := func(reason string) Verdict { return Verdict{Reason: reason} }
	if inMicrosoftFolder(r.Folder) {
		return hide(ReasonMicrosoft)
	}
	if strings.EqualFold(strings.TrimSpace(r.Name), OwnTaskName) {
		return hide(ReasonOwn)
	}
	if parseErr != nil {
		return hide(ReasonUnreadable)
	}
	if len(p.Triggers) == 0 {
		return hide(ReasonNoTrigger)
	}
	author := strings.ToLower(p.Author)
	if strings.Contains(author, "microsoft") || strings.Contains(author, "windows") || strings.HasPrefix(author, "$(") {
		return hide(ReasonMicrosoft)
	}
	if p.Command == "" {
		return hide(ReasonNoAction)
	}
	cmd := startup.ExpandPercent(p.Command, lookup)
	action := strings.TrimSpace(strings.Trim(strings.TrimSpace(cmd), `"`))
	if !isLauncher(cmd, lookup) {
		if parsed := startup.ParseCommand(cmd, lookup, nil); parsed.Path != "" {
			action = parsed.Path
			if parsed.Checkable && underDir(parsed.Path, winDir) {
				return hide(ReasonWindows)
			}
		}
	}
	if action == "" {
		return hide(ReasonNoAction)
	}
	return Verdict{
		Show: true, Triggers: p.Triggers, Action: action, NeedsAdmin: !perUser(p, me),
		Caution: services.LooksLikeSecurity(r.Name + " " + r.Folder + " " + p.Author + " " + action),
	}
}

// Manager lists and changes tasks.
type Manager struct {
	Sys      System
	Elevated func() bool // nil means not elevated
	Store    *origstate.Store
}

func (m *Manager) elevated() bool { return m.Elevated != nil && m.Elevated() }

type found struct {
	raw Raw
	id  string
	v   Verdict
}

func (m *Manager) scan() ([]found, error) {
	raws, err := m.Sys.List()
	if err != nil {
		return nil, err
	}
	winDir := m.Sys.Getenv("SystemRoot")
	if winDir == "" {
		winDir = m.Sys.Getenv("windir")
	}
	me := m.Sys.CurrentUser()
	out := make([]found, 0, len(raws))
	for _, r := range raws {
		if strings.TrimSpace(r.Name) == "" {
			continue
		}
		p, perr := ParseXML(r.XML)
		out = append(out, found{raw: r, id: MakeID(r.Folder, r.Name), v: Classify(r, p, perr, winDir, m.Sys.Getenv, me)})
	}
	return out, nil
}

// List returns the third-party tasks that start with Windows, ordered by name.
func (m *Manager) List() ([]Item, error) {
	all, err := m.scan()
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, f := range all {
		if f.v.Show {
			ids = append(ids, f.id)
		}
	}
	var results map[string]int32
	if len(ids) > 0 {
		results = m.Sys.LastResults(ids)
	}
	originals := map[string]string{}
	if m.Store != nil {
		if o, err := m.Store.All(); err == nil {
			originals = o
		}
	}
	items := make([]Item, 0, len(ids))
	for _, f := range all {
		if !f.v.Show {
			continue
		}
		st := StateOf(f.raw.State)
		it := Item{
			ID: f.id, Name: f.raw.Name, Folder: MakeID(f.raw.Folder, ""), Triggers: f.v.Triggers, Action: f.v.Action,
			State: st, Enabled: st != Disabled, NeedsAdmin: f.v.NeedsAdmin, Caution: f.v.Caution,
		}
		if code, ok := results[f.id]; ok && code != neverRun {
			c := code
			it.LastResult = &c
		}
		if o, ok := originals[strings.ToLower(f.id)]; ok && (o == "enabled" || o == "disabled") && (o == "enabled") != it.Enabled {
			it.Changed, it.Original = true, o
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
		if a != b {
			return a < b
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

// SetEnabled enables or disables one task. The tasks are read and classified again first: a task
// that is gone is ErrNotFound and one that is hidden is ErrProtected, so a guessed id can never be
// changed. A task that is not the user's own limited one needs administrator rights
// (ErrNeedsAdmin) and nothing is touched without them. Disabling a security product's task is
// refused unless confirmed (ErrNeedsConfirm). The original state is remembered before the first
// change; asking for the state a task already has changes nothing.
func (m *Manager) SetEnabled(id string, enabled, confirmed bool) error {
	return m.change(id, enabled, confirmed)
}

// Restore puts a task back to the state it had before WinForge first changed it.
func (m *Manager) Restore(id string, confirmed bool) error {
	if m.Store == nil {
		return ErrNoOriginal
	}
	o, ok, err := m.Store.Get(id)
	if err != nil {
		return ErrStore
	}
	if !ok || (o != "enabled" && o != "disabled") {
		return ErrNoOriginal
	}
	return m.change(id, o == "enabled", confirmed)
}

func word(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func (m *Manager) change(id string, enabled, confirmed bool) error {
	all, err := m.scan()
	if err != nil {
		return err
	}
	var cur *found
	for i := range all {
		if strings.EqualFold(all[i].id, id) {
			cur = &all[i]
			break
		}
	}
	if cur == nil {
		return ErrNotFound
	}
	if !cur.v.Show {
		return ErrProtected
	}
	if cur.v.NeedsAdmin && !m.elevated() {
		return ErrNeedsAdmin
	}
	was := StateOf(cur.raw.State) != Disabled
	if was == enabled {
		return nil
	}
	if !enabled && cur.v.Caution && !confirmed {
		return ErrNeedsConfirm
	}
	if m.Store == nil {
		return ErrStore
	}
	added, err := m.Store.Remember(cur.id, word(was))
	if err != nil {
		return ErrStore
	}
	if err := m.Sys.SetEnabled(cur.id, enabled); err != nil {
		if added {
			_ = m.Store.Forget(cur.id)
		}
		return err
	}
	after, err := m.scan()
	if err != nil {
		return err
	}
	for _, f := range after {
		if strings.EqualFold(f.id, cur.id) {
			if (StateOf(f.raw.State) != Disabled) != enabled {
				return ErrNotApplied
			}
			break
		}
	}
	if o, ok, _ := m.Store.Get(cur.id); ok && o == word(enabled) {
		_ = m.Store.Forget(cur.id)
	}
	return nil
}

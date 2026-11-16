// Package services lists the third-party Windows services and changes only their start mode
// (Automatic, Automatic delayed, Manual, Disabled). It never stops, starts, deletes or edits a
// service, and never touches recovery actions or dependencies. Before the first change of a
// service the original start mode is remembered so it can be restored.
//
// The logic here is pure; reading and writing the machine goes through System, which a Windows
// file implements and tests replace with an in-memory fake. Everything Microsoft, driver-type,
// per-user or security-critical is hidden: when in doubt a service is hidden, not shown.
package services

import (
	"errors"
	"sort"
	"strings"

	"github.com/Sebas1705/WinForge/internal/origstate"
)

// Mode is a start mode WinForge can set.
type Mode string

const (
	Auto     Mode = "auto"
	Delayed  Mode = "delayed"
	Manual   Mode = "manual"
	Disabled Mode = "disabled"
)

// Modes is every mode a change may ask for.
var Modes = []Mode{Auto, Delayed, Manual, Disabled}

// Valid reports whether m is one of the four modes.
func (m Mode) Valid() bool {
	for _, k := range Modes {
		if k == m {
			return true
		}
	}
	return false
}

// rank orders modes by how eagerly the service starts: lowering it is what can break dependents.
func (m Mode) rank() int {
	switch m {
	case Auto, Delayed:
		return 2
	case Manual:
		return 1
	}
	return 0
}

// ModeOf maps the registry's Start value (2 automatic, 3 manual, 4 disabled) and the delayed flag
// to a Mode. ok is false for boot and system start (drivers) and for unknown values.
func ModeOf(start uint32, delayed bool) (Mode, bool) {
	switch start {
	case 2:
		if delayed {
			return Delayed, true
		}
		return Auto, true
	case 3:
		return Manual, true
	case 4:
		return Disabled, true
	}
	return "", false
}

// State is what the service is doing now.
type State string

const (
	Running State = "running"
	Stopped State = "stopped"
	Other   State = "other" // starting, stopping, paused, or not readable
)

// Raw is one service as found in the machine's configuration.
type Raw struct {
	Name        string
	DisplayName string
	Type        uint32   // SERVICE_* type flags (0x1 kernel driver, 0x10 own process, 0x20 shared...)
	Start       uint32   // 0 boot, 1 system, 2 automatic, 3 manual, 4 disabled
	Delayed     bool     // DelayedAutostart (only meaningful when Start is 2)
	ImagePath   string   // the command line that starts it
	DependsOn   []string // names of the services it needs
}

// System is everything the package needs from the machine.
type System interface {
	// List returns every service and driver entry of the machine.
	List() ([]Raw, error)
	// States returns the current state of the named services (missing ones are left out).
	States(names []string) map[string]State
	// Publishers returns the company of the given files, by path (unknown ones are left out).
	Publishers(paths []string) map[string]string
	// SetStartMode changes only the start mode of one service.
	SetStartMode(name string, m Mode) error
	// Getenv returns an environment variable, "" when unset.
	Getenv(name string) string
}

// Item is one service the page shows.
type Item struct {
	ID          string   `json:"id"`          // the service name
	Name        string   `json:"name"`        // the service name, as Windows knows it
	DisplayName string   `json:"displayName"` // what the person sees
	Publisher   string   `json:"publisher"`   // "" when unknown
	State       State    `json:"state"`
	StartMode   Mode     `json:"startMode"`
	Binary      string   `json:"binary"`     // the program, without arguments
	NeedsAdmin  bool     `json:"needsAdmin"` // always true: every change needs administrator rights
	Caution     bool     `json:"caution"`    // looks like a security product
	Dependents  []string `json:"dependents"` // display names of services that need this one
	Changed     bool     `json:"changed"`    // WinForge changed it and remembers the original
	Original    Mode     `json:"original"`   // the remembered original, "" unless Changed
}

// Errors the interface recognises by their text: stable codes, not sentences.
var (
	ErrNeedsAdmin    = errors.New("svc:needs-admin")
	ErrNotFound      = errors.New("svc:not-found")
	ErrProtected     = errors.New("svc:protected")
	ErrBadMode       = errors.New("svc:bad-mode")
	ErrNeedsConfirm  = errors.New("svc:needs-confirm")  // a security product: ask first
	ErrHasDependents = errors.New("svc:has-dependents") // other services need it: ask first
	ErrNoOriginal    = errors.New("svc:no-original")
	ErrStore         = errors.New("svc:store-failed")
	ErrNotApplied    = errors.New("svc:not-applied")
)

// Manager lists and changes services.
type Manager struct {
	Sys      System
	Elevated func() bool // nil means not elevated
	Store    *origstate.Store
}

func (m *Manager) elevated() bool { return m.Elevated != nil && m.Elevated() }

type found struct {
	raw  Raw
	mode Mode
	exe  string
	v    Verdict
}

// scan reads the machine and classifies every service. The returned slice holds every entry
// (hidden ones too, with their verdict) so a write can tell "gone" from "protected".
func (m *Manager) scan() ([]found, []Raw, error) {
	raws, err := m.Sys.List()
	if err != nil {
		return nil, nil, err
	}
	env := Env{WinDir: m.Sys.Getenv("SystemRoot"), Lookup: m.Sys.Getenv}
	if env.WinDir == "" {
		env.WinDir = m.Sys.Getenv("windir")
	}
	all := make([]found, len(raws))
	var paths []string
	seen := map[string]bool{}
	for i, r := range raws {
		f := found{raw: r, exe: ExecutablePath(r.ImagePath, env)}
		f.mode, _ = ModeOf(r.Start, r.Delayed)
		f.v = Classify(r, env, "")
		all[i] = f
		if f.v.Show && !seen[f.exe] {
			seen[f.exe] = true
			paths = append(paths, f.exe)
		}
	}
	var pubs map[string]string
	if len(paths) > 0 {
		pubs = m.Sys.Publishers(paths)
	}
	for i := range all {
		if !all[i].v.Show {
			continue
		}
		all[i].v = Classify(all[i].raw, env, strings.TrimSpace(pubs[all[i].exe]))
		if all[i].v.Show {
			all[i].v.Publisher = strings.TrimSpace(pubs[all[i].exe])
		}
	}
	return all, raws, nil
}

// List returns the third-party services, ordered by display name.
func (m *Manager) List() ([]Item, error) {
	all, raws, err := m.scan()
	if err != nil {
		return nil, err
	}
	display := map[string]string{}
	for _, r := range raws {
		display[strings.ToLower(r.Name)] = nameOf(r)
	}
	dependents := map[string][]string{}
	for _, r := range raws {
		for _, d := range r.DependsOn {
			k := strings.ToLower(d)
			dependents[k] = append(dependents[k], nameOf(r))
		}
	}
	var names []string
	for _, f := range all {
		if f.v.Show {
			names = append(names, f.raw.Name)
		}
	}
	var states map[string]State
	if len(names) > 0 {
		states = m.Sys.States(names)
	}
	originals := map[string]string{}
	if m.Store != nil {
		// An unreadable store only hides the "Restore original" buttons; the list is still right.
		if o, err := m.Store.All(); err == nil {
			originals = o
		}
	}
	items := make([]Item, 0, len(names))
	for _, f := range all {
		if !f.v.Show {
			continue
		}
		it := Item{
			ID: f.raw.Name, Name: f.raw.Name, DisplayName: nameOf(f.raw), Publisher: f.v.Publisher,
			State: Other, StartMode: f.mode, Binary: f.exe, NeedsAdmin: true, Caution: f.v.Caution,
			Dependents: uniqueSorted(dependents[strings.ToLower(f.raw.Name)]),
		}
		if st, ok := states[f.raw.Name]; ok {
			it.State = st
		}
		if o, ok := originals[strings.ToLower(f.raw.Name)]; ok && Mode(o).Valid() && Mode(o) != f.mode {
			it.Changed, it.Original = true, Mode(o)
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := strings.ToLower(items[i].DisplayName), strings.ToLower(items[j].DisplayName)
		if a != b {
			return a < b
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func nameOf(r Raw) string {
	if d := strings.TrimSpace(r.DisplayName); d != "" && !strings.HasPrefix(d, "@") {
		return d
	}
	return r.Name
}

func uniqueSorted(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		if k := strings.ToLower(s); !seen[k] {
			seen[k] = true
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// SetMode changes the start mode of one service. The services are read and classified again first:
// a service that is gone is ErrNotFound and one that is hidden or protected is ErrProtected, so a
// guessed name can never be changed. Without administrator rights nothing is touched
// (ErrNeedsAdmin). A security product (ErrNeedsConfirm) and a service other services need when
// its start mode is lowered (ErrHasDependents) are refused unless confirmed. The original start
// mode is remembered before the first change; asking for the mode a service already has changes
// nothing.
func (m *Manager) SetMode(id string, mode Mode, confirmed bool) error {
	return m.change(id, mode, confirmed)
}

// Restore puts a service back to the start mode it had before WinForge first changed it.
func (m *Manager) Restore(id string, confirmed bool) error {
	if m.Store == nil {
		return ErrNoOriginal
	}
	o, ok, err := m.Store.Get(id)
	if err != nil {
		return ErrStore
	}
	if !ok || !Mode(o).Valid() {
		return ErrNoOriginal
	}
	return m.change(id, Mode(o), confirmed)
}

func (m *Manager) change(id string, target Mode, confirmed bool) error {
	if !target.Valid() {
		return ErrBadMode
	}
	all, _, err := m.scan()
	if err != nil {
		return err
	}
	var cur *found
	for i := range all {
		if strings.EqualFold(all[i].raw.Name, id) {
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
	if !m.elevated() {
		return ErrNeedsAdmin
	}
	if cur.mode == target {
		return nil
	}
	if !confirmed {
		if cur.v.Caution {
			return ErrNeedsConfirm
		}
		if target.rank() < cur.mode.rank() && m.hasDependents(all, cur.raw.Name) {
			return ErrHasDependents
		}
	}
	if m.Store == nil {
		return ErrStore
	}
	added, err := m.Store.Remember(cur.raw.Name, string(cur.mode))
	if err != nil {
		return ErrStore
	}
	if err := m.Sys.SetStartMode(cur.raw.Name, target); err != nil {
		if added {
			_ = m.Store.Forget(cur.raw.Name)
		}
		return err
	}
	// Read it back: a change that did not stick must not be reported as done.
	after, _, err := m.scan()
	if err != nil {
		return err
	}
	for _, f := range after {
		if strings.EqualFold(f.raw.Name, cur.raw.Name) {
			if f.mode != target {
				return ErrNotApplied
			}
			break
		}
	}
	// Back to what it was: there is nothing left to restore.
	if o, ok, _ := m.Store.Get(cur.raw.Name); ok && Mode(o) == target {
		_ = m.Store.Forget(cur.raw.Name)
	}
	return nil
}

func (m *Manager) hasDependents(all []found, name string) bool {
	for _, f := range all {
		for _, d := range f.raw.DependsOn {
			if strings.EqualFold(d, name) {
				return true
			}
		}
	}
	return false
}

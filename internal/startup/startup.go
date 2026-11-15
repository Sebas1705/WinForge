// Package startup lists what starts with Windows (Run registry values and Startup folders) and
// switches items off and on the way Task Manager does, through the StartupApproved values.
// Nothing is ever deleted: the Run value and the file stay where they are.
//
// The logic here is pure; reading and writing the registry and disk goes through System, which a
// Windows file implements and tests replace with an in-memory fake.
package startup

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// Origin is one place Windows looks for programs to start.
type Origin string

const (
	UserRun      Origin = "hkcu-run"      // HKCU\...\Run
	MachineRun   Origin = "hklm-run"      // HKLM\...\Run
	MachineRun32 Origin = "hklm-run32"    // HKLM\SOFTWARE\WOW6432Node\...\Run (32-bit programs)
	UserFolder   Origin = "user-folder"   // %APPDATA%\...\Startup
	CommonFolder Origin = "common-folder" // %ProgramData%\...\StartUp
)

// Origins in the order the page groups them when names are equal.
var Origins = []Origin{UserRun, MachineRun, MachineRun32, UserFolder, CommonFolder}

func (o Origin) valid() bool {
	for _, k := range Origins {
		if k == o {
			return true
		}
	}
	return false
}

// Machine is true for origins that apply to every user: changing them writes HKLM and needs
// administrator rights.
func (o Origin) Machine() bool { return o == MachineRun || o == MachineRun32 || o == CommonFolder }

// Folder is true for the Startup folders (files and shortcuts) rather than registry values.
func (o Origin) Folder() bool { return o == UserFolder || o == CommonFolder }

// ApprovedKey is the StartupApproved subkey that holds the on/off flags of the origin.
func (o Origin) ApprovedKey() string {
	switch o {
	case MachineRun32:
		return "Run32"
	case UserFolder, CommonFolder:
		return "StartupFolder"
	}
	return "Run"
}

func (o Origin) rank() int {
	for i, k := range Origins {
		if k == o {
			return i
		}
	}
	return len(Origins)
}

// Raw is one entry as found: the registry value name or the file name, and its command line
// (the value's data, or the full path of the file).
type Raw struct {
	Name    string
	Command string
}

// FileInfo is what could be learned about a file. Both fields may be empty.
type FileInfo struct {
	Publisher string // CompanyName of the version information
	Target    string // where a shortcut (.lnk) points
}

// System is everything the package needs from the machine.
type System interface {
	// Entries lists the Run values or startup-folder files of an origin. A missing key or folder
	// is an empty list, not an error.
	Entries(o Origin) ([]Raw, error)
	// Approved returns the StartupApproved values of an origin by lower-case name.
	Approved(o Origin) (map[string][]byte, error)
	// SetApproved writes one StartupApproved value, creating its key when needed.
	SetApproved(o Origin, name string, data []byte) error
	// Exists reports whether a file (not a folder) is there.
	Exists(path string) bool
	// Getenv returns an environment variable, "" when unset.
	Getenv(name string) string
	// Inspect reads the publisher of the given files and where shortcuts point, all at once.
	// Paths it knows nothing about are left out.
	Inspect(paths []string) map[string]FileInfo
}

// Item is one thing that starts with Windows.
type Item struct {
	ID         string `json:"id"`        // "<source>|<name>", stable between lists
	Name       string `json:"name"`      // what the person sees
	Source     string `json:"source"`    // an Origin
	Command    string `json:"command"`   // the command line, or the path of the startup file
	Target     string `json:"target"`    // the program it starts, "" when it cannot be told
	Publisher  string `json:"publisher"` // "" when unknown
	Enabled    bool   `json:"enabled"`
	NeedsAdmin bool   `json:"needsAdmin"` // switching it needs administrator rights
	Missing    bool   `json:"missing"`    // the program it starts is not there
	Caution    bool   `json:"caution"`    // looks like a Windows or security component
}

// Errors the interface recognises by their text.
var (
	ErrNeedsAdmin = errors.New("needs administrator")
	ErrNotFound   = errors.New("that startup item no longer exists")
)

// Manager lists and switches startup items.
type Manager struct {
	Sys      System
	Elevated func() bool      // nil means not elevated
	Now      func() time.Time // nil means time.Now
}

func (m *Manager) elevated() bool { return m.Elevated != nil && m.Elevated() }

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// MakeID builds the id of an item. The name may contain any character, so the id is split at the
// first bar only.
func MakeID(o Origin, name string) string { return string(o) + "|" + name }

// ParseID is the inverse of MakeID. ok is false for text that names no origin.
func ParseID(id string) (o Origin, name string, ok bool) {
	i := strings.IndexByte(id, '|')
	if i < 0 {
		return "", "", false
	}
	o, name = Origin(id[:i]), id[i+1:]
	return o, name, o.valid() && name != ""
}

// displayName drops the extension of shortcuts, which only repeats that it is one.
func displayName(o Origin, name string) string {
	if o.Folder() {
		l := strings.ToLower(name)
		for _, e := range []string{".lnk", ".url"} {
			if strings.HasSuffix(l, e) && len(name) > len(e) {
				return name[:len(name)-len(e)]
			}
		}
	}
	return name
}

// List returns every startup item, ordered by name. An origin that cannot be read is skipped so
// one damaged key does not hide the rest; the error is returned only when nothing could be read.
func (m *Manager) List() ([]Item, error) {
	type found struct {
		origin Origin
		raw    Raw
		flags  map[string][]byte
	}
	var all []found
	var errs []error
	okCount := 0
	for _, o := range Origins {
		raws, err := m.Sys.Entries(o)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		okCount++
		// A flags error leaves everything of that origin looking enabled; the list is still right.
		flags, _ := m.Sys.Approved(o)
		seen := map[string]bool{}
		for _, r := range raws {
			k := strings.ToLower(r.Name)
			if r.Name == "" || seen[k] {
				continue
			}
			seen[k] = true
			all = append(all, found{o, r, flags})
		}
	}
	if okCount == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	lookup := m.Sys.Getenv
	parsed := make([]Parsed, len(all))
	var paths []string
	seenPath := map[string]bool{}
	want := func(p string) {
		if p != "" && !seenPath[p] {
			seenPath[p] = true
			paths = append(paths, p)
		}
	}
	for i, f := range all {
		if f.origin.Folder() {
			parsed[i] = Parsed{Path: f.raw.Command, Checkable: true}
			want(f.raw.Command)
			continue
		}
		parsed[i] = ParseCommand(f.raw.Command, lookup, m.Sys.Exists)
		if !parsed[i].Launcher && parsed[i].Checkable && m.Sys.Exists(parsed[i].Path) {
			want(parsed[i].Path)
		}
	}
	var info map[string]FileInfo
	if len(paths) > 0 {
		info = m.Sys.Inspect(paths)
	}

	items := make([]Item, 0, len(all))
	for i, f := range all {
		p := parsed[i]
		it := Item{
			ID: MakeID(f.origin, f.raw.Name), Name: displayName(f.origin, f.raw.Name), Source: string(f.origin),
			Command: f.raw.Command, NeedsAdmin: f.origin.Machine(),
			Enabled: DecodeApproved(f.flags[strings.ToLower(f.raw.Name)]),
		}
		// A value that is not a path or a program name (a counter, an id) starts nothing we can show.
		if !p.Launcher && (p.Checkable || hasProgramExt(p.Path)) {
			it.Target = p.Path
			fi := info[p.Path]
			it.Publisher = strings.TrimSpace(fi.Publisher)
			if f.origin.Folder() && fi.Target != "" {
				// A shortcut: what it points at is what starts, and what can be missing.
				it.Target = fi.Target
				it.Missing = !m.Sys.Exists(fi.Target)
			} else if p.Checkable {
				it.Missing = !m.Sys.Exists(p.Path)
			}
		}
		it.Caution = Caution(it.Name, it.Publisher, it.Command)
		items = append(items, it)
	}
	sortItems(items)
	return items, nil
}

func sortItems(items []Item) {
	rank := func(s string) int { return Origin(s).rank() }
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if x, y := strings.ToLower(a.Name), strings.ToLower(b.Name); x != y {
			return x < y
		}
		if rank(a.Source) != rank(b.Source) {
			return rank(a.Source) < rank(b.Source)
		}
		return a.ID < b.ID
	})
}

// Toggle switches one item on or off. The entry is read again first, so an item that is gone is
// reported and nothing is written for it. Machine items need administrator rights and are refused
// before anything is touched when the app does not have them. Asking for the state the item
// already has changes nothing.
func (m *Manager) Toggle(id string, enabled bool) error {
	o, name, ok := ParseID(id)
	if !ok {
		return ErrNotFound
	}
	raws, err := m.Sys.Entries(o)
	if err != nil {
		return err
	}
	key := ""
	for _, r := range raws {
		if strings.EqualFold(r.Name, name) {
			key = r.Name
			break
		}
	}
	if key == "" {
		return ErrNotFound
	}
	if o.Machine() && !m.elevated() {
		return ErrNeedsAdmin
	}
	flags, err := m.Sys.Approved(o)
	if err != nil {
		return err
	}
	if DecodeApproved(flags[strings.ToLower(key)]) == enabled {
		// Already as asked. A damaged value (too short to read) is still rewritten whole.
		if len(flags[strings.ToLower(key)]) == 0 || len(flags[strings.ToLower(key)]) >= 4 {
			return nil
		}
	}
	return m.Sys.SetApproved(o, key, EncodeApproved(enabled, m.now()))
}

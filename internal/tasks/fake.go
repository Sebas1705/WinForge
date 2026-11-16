package tasks

import (
	"strings"
	"sync"
)

// Fake is an in-memory machine for tests of this package and of the bindings: no Task Scheduler.
type Fake struct {
	mu         sync.Mutex
	Tasks      []Raw
	Results    map[string]int32 // by id
	Me         []string
	Env        map[string]string
	SetErr     error // returned by SetEnabled when set
	IgnoreSets bool  // SetEnabled succeeds but changes nothing
	Writes     []string
}

// NewFake is a machine with C:\Windows as %SystemRoot%, the user "PC\sebas" and no tasks.
func NewFake() *Fake {
	return &Fake{Results: map[string]int32{}, Me: []string{`PC\sebas`, "sebas", "S-1-5-21-1-2-3-1001"}, Env: map[string]string{
		"SystemRoot": `C:\Windows`, "ProgramFiles": `C:\Program Files`, "LOCALAPPDATA": `C:\Users\sebas\AppData\Local`,
	}}
}

func (f *Fake) List() ([]Raw, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Raw(nil), f.Tasks...), nil
}

func (f *Fake) LastResults(ids []string) map[string]int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int32{}
	for _, id := range ids {
		if r, ok := f.Results[id]; ok {
			out[id] = r
		}
	}
	return out
}

func (f *Fake) CurrentUser() []string { return f.Me }

func (f *Fake) Getenv(n string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Env[n]
}

// SetEnabled records the write and applies it to the fake task.
func (f *Fake) SetEnabled(id string, enabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SetErr != nil {
		return f.SetErr
	}
	f.Writes = append(f.Writes, id+"="+word(enabled))
	if f.IgnoreSets {
		return nil
	}
	for i := range f.Tasks {
		if strings.EqualFold(MakeID(f.Tasks[i].Folder, f.Tasks[i].Name), id) {
			if enabled {
				f.Tasks[i].State = stateReady
			} else {
				f.Tasks[i].State = stateDisabled
			}
		}
	}
	return nil
}

// WriteCount is how many changes were asked for.
func (f *Fake) WriteCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Writes)
}

// Enabled reports whether a fake task is not disabled.
func (f *Fake) Enabled(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.Tasks {
		if strings.EqualFold(MakeID(t.Folder, t.Name), id) {
			return StateOf(t.State) != Disabled
		}
	}
	return false
}

package services

import (
	"strings"
	"sync"
)

// Fake is an in-memory machine for tests of this package and of the bindings: no registry, no
// service control manager.
type Fake struct {
	mu         sync.Mutex
	Services   []Raw
	Running    map[string]State  // by service name
	Companies  map[string]string // publisher by program path
	Env        map[string]string
	SetErr     error // returned by SetStartMode when set
	IgnoreSets bool  // SetStartMode succeeds but changes nothing
	Writes     []string
}

// NewFake is a machine with C:\Windows as %SystemRoot% and no services.
func NewFake() *Fake {
	return &Fake{Running: map[string]State{}, Companies: map[string]string{}, Env: map[string]string{
		"SystemRoot": `C:\Windows`, "ProgramFiles": `C:\Program Files`, "ProgramData": `C:\ProgramData`,
	}}
}

func (f *Fake) List() ([]Raw, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Raw(nil), f.Services...), nil
}

func (f *Fake) States(names []string) map[string]State {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]State{}
	for _, n := range names {
		if s, ok := f.Running[n]; ok {
			out[n] = s
		}
	}
	return out
}

func (f *Fake) Publishers(paths []string) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for _, p := range paths {
		if c, ok := f.Companies[p]; ok {
			out[p] = c
		}
	}
	return out
}

func (f *Fake) Getenv(n string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Env[n]
}

// SetStartMode records the write and applies it to the fake service.
func (f *Fake) SetStartMode(name string, m Mode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SetErr != nil {
		return f.SetErr
	}
	f.Writes = append(f.Writes, name+"="+string(m))
	if f.IgnoreSets {
		return nil
	}
	for i := range f.Services {
		if !strings.EqualFold(f.Services[i].Name, name) {
			continue
		}
		f.Services[i].Delayed = m == Delayed
		switch m {
		case Auto, Delayed:
			f.Services[i].Start = 2
		case Manual:
			f.Services[i].Start = 3
		case Disabled:
			f.Services[i].Start = 4
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

// Mode returns the current start mode of a fake service.
func (f *Fake) Mode(name string) Mode {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.Services {
		if strings.EqualFold(r.Name, name) {
			m, _ := ModeOf(r.Start, r.Delayed)
			return m
		}
	}
	return ""
}

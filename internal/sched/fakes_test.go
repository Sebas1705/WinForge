package sched

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// fakeSys is a Windows with nothing on it: it records what would have been done.
type fakeSys struct {
	mu        sync.Mutex
	info      TaskInfo
	queryErr  error
	created   []Task
	deletes   int
	toasts    []Toast
	protocol  string
	unproto   int
	createErr error
	toastErr  error
	protoErr  error
}

func (f *fakeSys) Query() (TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.info, f.queryErr
}

func (f *fakeSys) Create(t Task) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, t)
	f.info = TaskInfo{Exists: true, Enabled: true, Settings: t.Settings, Exe: t.Exe, Args: strings.Join(Args(t.Settings.Mode), " ")}
	return nil
}

func (f *fakeSys) Delete() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes++
	f.info = TaskInfo{}
	return nil
}

func (f *fakeSys) Toast(t Toast) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.toastErr != nil {
		return f.toastErr
	}
	f.toasts = append(f.toasts, t)
	return nil
}

func (f *fakeSys) RegisterProtocol(exe string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.protoErr != nil {
		return f.protoErr
	}
	f.protocol = exe
	return nil
}

func (f *fakeSys) UnregisterProtocol() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unproto++
	f.protocol = ""
	return nil
}

// fakeExec records the winget commands a run would start.
type fakeExec struct {
	mu    sync.Mutex
	calls []string
	exit  map[string]int    // substring of the command line -> exit code
	out   map[string]string // substring -> one line printed
}

func (f *fakeExec) Run(_ context.Context, name string, args []string, onLine func(string)) (int, error) {
	line := name + " " + strings.Join(args, " ")
	f.mu.Lock()
	f.calls = append(f.calls, line)
	f.mu.Unlock()
	for k, o := range f.out {
		if strings.Contains(line, k) {
			onLine(o)
		}
	}
	for k, code := range f.exit {
		if strings.Contains(line, k) {
			return code, nil
		}
	}
	return 0, nil
}

func testCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		Apps: map[string]*catalog.App{
			"git":      {ID: "git", Name: "Git", Winget: "Git.Git"},
			"vlc":      {ID: "vlc", Name: "VLC", Winget: "VideoLAN.VLC"},
			"7zip":     {ID: "7zip", Name: "7-Zip", Winget: "7zip.7zip", Admin: true},
			"firefox":  {ID: "firefox", Name: "Firefox", Winget: "Mozilla.Firefox"},
			"notepad":  {ID: "notepad", Name: "Notepad++", Winget: "Notepad++.Notepad++"},
			"winforge": {ID: "winforge", Name: "WinForge", Winget: "Sebas1705.WinForge"},
		},
		Recipes: map[string]*catalog.Recipe{"tweak": {ID: "tweak", Description: "a recipe"}},
	}
}

var day0 = time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

func ups() []system.Upgrade {
	return []system.Upgrade{
		{ID: "Git.Git", Name: "Git", Current: "1.0", Available: "2.0"},
		{ID: "VideoLAN.VLC", Name: "VLC", Current: "3.0", Available: "3.1"},
		{ID: "7zip.7zip", Name: "7-Zip", Current: "23", Available: "24"},
		{ID: "Mozilla.Firefox", Name: "Firefox", Current: "130", Available: "131"},
		{ID: "Sebas1705.WinForge", Name: "WinForge", Current: "0.1", Available: "0.2"},
		{ID: "Some.Unrelated", Name: "Unrelated", Current: "1", Available: "2"}, // not in the catalog
	}
}

// env is a PC with the updates above, no ignore rules, not elevated, and a winget that works.
func env(x *fakeExec) Env {
	return Env{
		Cat:      testCatalog(),
		Upgrades: func(context.Context) ([]system.Upgrade, error) { return ups(), nil },
		Rules:    func() (map[string]updates.Rule, error) { return map[string]updates.Rule{}, nil },
		Exec:     x,
		Now:      func() time.Time { return day0 },
		Elevated: func() bool { return false },
	}
}

func item(t *testing.T, r Report, id string) Item {
	t.Helper()
	for _, it := range r.Items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("no item %q in %+v", id, r.Items)
	return Item{}
}

func wantErr(t *testing.T, err error, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want %v", err, target)
	}
}

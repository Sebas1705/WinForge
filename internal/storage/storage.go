// Package storage finds and removes what wastes disk space: temporary files,
// caches, the Recycle Bin, old Windows leftovers, and the large and duplicate
// files a person chooses to look at.
//
// It is built around one rule: it deletes only inside a short, fixed list of
// folders that Windows and apps rebuild by themselves (see Specs), never
// follows a link out of them, and never touches anything the person has not
// been shown. Files the person's own folders hold (documents, pictures,
// downloads) are only ever listed or moved to the Recycle Bin on request, never
// deleted by a category.
package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Risk says how much a person should think before cleaning a category.
type Risk string

const (
	// Safe categories are rebuilt by Windows or the app with no visible effect.
	Safe Risk = "safe"
	// Review categories have a cost the person should know: a slow first build,
	// a download, or not being able to go back.
	Review Risk = "review"
)

// Env holds the folders categories are made of. It is injected so tests can
// run on a temporary tree; DefaultEnv reads the real ones.
type Env struct {
	Home         string // the person's profile folder
	LocalAppData string
	AppData      string
	ProgramData  string
	WinDir       string
	SystemDrive  string // "C:"
	Temp         string
}

// DefaultEnv reads the folders from the environment.
func DefaultEnv() Env {
	home, _ := os.UserHomeDir()
	drive := os.Getenv("SystemDrive")
	if drive == "" {
		drive = "C:"
	}
	return Env{
		Home:         home,
		LocalAppData: os.Getenv("LOCALAPPDATA"),
		AppData:      os.Getenv("APPDATA"),
		ProgramData:  os.Getenv("ProgramData"),
		WinDir:       os.Getenv("SystemRoot"),
		SystemDrive:  drive,
		Temp:         os.Getenv("TEMP"),
	}
}

// Spec describes one cleanable category.
type Spec struct {
	ID         string
	Risk       Risk
	NeedsAdmin bool
	// Dirs are folders whose contents are removed (never the folders
	// themselves). A "*" segment stands for any one folder name, for example a
	// browser profile.
	Dirs func(Env) []string
	// Files, when set, limits removal to file names that match one of these
	// patterns (path.Match), for folders that hold other things too.
	Files []string
	// MinAge keeps recent files, which something may still be using.
	MinAge time.Duration
	// Special categories are not just folders of files.
	Special string
}

func under(base string, parts ...string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, parts...)...)
}

func list(paths ...string) []string {
	var out []string
	for _, p := range paths {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func browser(base string, parts ...string) []string {
	// Only the cache folders of each profile: never cookies, history, passwords or sessions.
	if base == "" {
		return nil
	}
	var out []string
	for _, c := range []string{"Cache", "Code Cache", "GPUCache"} {
		out = append(out, under(base, append(append([]string{}, parts...), "*", c)...))
	}
	return out
}

// Specs is the whole list of categories, in the order they are shown. Adding
// one is adding an entry here; nothing is discovered by scanning the disk.
var Specs = []Spec{
	{ID: "temp-user", Risk: Safe, MinAge: 24 * time.Hour,
		Dirs: func(e Env) []string { return list(e.Temp) }},
	{ID: "temp-windows", Risk: Safe, NeedsAdmin: true, MinAge: 24 * time.Hour,
		Dirs: func(e Env) []string { return list(under(e.WinDir, "Temp")) }},
	{ID: "recycle-bin", Risk: Safe, Special: "recycle-bin"},
	{ID: "thumbnails", Risk: Safe, Files: []string{"thumbcache_*.db", "iconcache_*.db"},
		Dirs: func(e Env) []string { return list(under(e.LocalAppData, "Microsoft", "Windows", "Explorer")) }},
	{ID: "crash-dumps", Risk: Safe,
		Dirs: func(e Env) []string {
			return list(
				under(e.LocalAppData, "CrashDumps"),
				under(e.LocalAppData, "Microsoft", "Windows", "WER", "ReportArchive"),
				under(e.LocalAppData, "Microsoft", "Windows", "WER", "ReportQueue"),
				under(e.ProgramData, "Microsoft", "Windows", "WER", "ReportArchive"),
				under(e.ProgramData, "Microsoft", "Windows", "WER", "ReportQueue"))
		}},
	{ID: "delivery-optimization", Risk: Safe, NeedsAdmin: true,
		Dirs: func(e Env) []string {
			return list(under(e.WinDir, "ServiceProfiles", "NetworkService", "AppData", "Local", "Microsoft", "Windows", "DeliveryOptimization", "Cache"))
		}},
	{ID: "windows-update", Risk: Safe, NeedsAdmin: true,
		Dirs: func(e Env) []string { return list(under(e.WinDir, "SoftwareDistribution", "Download")) }},
	{ID: "windows-old", Risk: Review, NeedsAdmin: true, Special: "windows-old"},
	{ID: "cache-chrome", Risk: Safe,
		Dirs: func(e Env) []string { return browser(e.LocalAppData, "Google", "Chrome", "User Data") }},
	{ID: "cache-edge", Risk: Safe,
		Dirs: func(e Env) []string { return browser(e.LocalAppData, "Microsoft", "Edge", "User Data") }},
	{ID: "cache-firefox", Risk: Safe,
		Dirs: func(e Env) []string {
			return list(under(e.LocalAppData, "Mozilla", "Firefox", "Profiles", "*", "cache2"))
		}},
	{ID: "cache-brave", Risk: Safe,
		Dirs: func(e Env) []string { return browser(e.LocalAppData, "BraveSoftware", "Brave-Browser", "User Data") }},
	{ID: "shader-cache", Risk: Review,
		Dirs: func(e Env) []string {
			return list(
				under(e.LocalAppData, "D3DSCache"),
				under(e.LocalAppData, "NVIDIA", "DXCache"),
				under(e.LocalAppData, "NVIDIA", "GLCache"),
				under(e.LocalAppData, "AMD", "DxCache"),
				under(e.LocalAppData, "AMD", "GLCache"))
		}},
	{ID: "dev-npm", Risk: Safe,
		Dirs: func(e Env) []string { return list(under(e.LocalAppData, "npm-cache"), under(e.AppData, "npm-cache")) }},
	{ID: "dev-pip", Risk: Safe,
		Dirs: func(e Env) []string { return list(under(e.LocalAppData, "pip", "Cache")) }},
	{ID: "dev-nuget", Risk: Safe,
		Dirs: func(e Env) []string {
			return list(under(e.LocalAppData, "NuGet", "v3-cache"), under(e.LocalAppData, "NuGet", "plugins-cache"))
		}},
	{ID: "dev-go", Risk: Safe,
		Dirs: func(e Env) []string { return list(under(e.LocalAppData, "go-build")) }},
	{ID: "dev-gradle", Risk: Review,
		Dirs: func(e Env) []string { return list(under(e.Home, ".gradle", "caches")) }},
	// Downloaded again on the next build; only the download folders, never a setting, a key or a project.
	{ID: "dev-yarn", Risk: Safe,
		Dirs: func(e Env) []string { return list(under(e.LocalAppData, "Yarn", "Cache")) }},
	{ID: "dev-pnpm", Risk: Review,
		Dirs: func(e Env) []string { return list(under(e.LocalAppData, "pnpm", "store")) }},
	{ID: "dev-cargo", Risk: Review,
		Dirs: func(e Env) []string {
			return list(under(e.Home, ".cargo", "registry", "cache"), under(e.Home, ".cargo", "registry", "src"))
		}},
	{ID: "dev-gradle-wrapper", Risk: Review,
		Dirs: func(e Env) []string { return list(under(e.Home, ".gradle", "wrapper", "dists")) }},
	{ID: "dev-android", Risk: Safe,
		Dirs: func(e Env) []string { return list(under(e.Home, ".android", "cache")) }},
	// Docker's own commands do the removing; containers and volumes are never touched.
	{ID: "docker-build-cache", Risk: Review, Special: "docker-build-cache"},
	{ID: "docker-dangling", Risk: Review, Special: "docker-dangling"},
}

// SpecFor returns the category with that id.
func SpecFor(id string) (Spec, bool) {
	for _, s := range Specs {
		if s.ID == id {
			return s, true
		}
	}
	return Spec{}, false
}

// Category is what a scan found for one category.
type Category struct {
	ID         string `json:"id"`
	Risk       Risk   `json:"risk"`
	NeedsAdmin bool   `json:"needsAdmin"`
	Available  bool   `json:"available"` // the folder exists on this PC
	Bytes      int64  `json:"bytes"`
	Files      int    `json:"files"`
	Skipped    int    `json:"skipped"` // could not be read
}

// Outcome is what cleaning one category did.
type Outcome struct {
	ID         string `json:"id"`
	FreedBytes int64  `json:"freedBytes"`
	Files      int    `json:"files"`
	Failed     int    `json:"failed"` // in use or not allowed: left as they were
	Error      string `json:"error,omitempty"`
}

// Result is what a clean did, in total and per category.
type Result struct {
	FreedBytes int64     `json:"freedBytes"`
	Files      int       `json:"files"`
	Failed     int       `json:"failed"`
	Outcomes   []Outcome `json:"outcomes"`
}

// Progress is reported while something runs.
type Progress struct {
	Task    string `json:"task"` // scan, clean, big, folders, dups
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	Current string `json:"current,omitempty"`
}

// ProgressFunc receives progress, at most a few times a second.
type ProgressFunc func(Progress)

// Runner runs a command, for the categories that need Windows' own tools.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// throttle wraps a ProgressFunc so it fires at most every 150 ms, plus once at the end.
// It is safe for several goroutines to report at once.
type throttle struct {
	mu   sync.Mutex
	fn   ProgressFunc
	last time.Time
	p    Progress
}

func newThrottle(task string, fn ProgressFunc) *throttle {
	return &throttle{fn: fn, p: Progress{Task: task}}
}

func (t *throttle) add(files int, bytes int64, current string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.p.Files += files
	t.p.Bytes += bytes
	if current != "" {
		t.p.Current = current
	}
	if t.fn != nil && time.Since(t.last) > 150*time.Millisecond {
		t.last = time.Now()
		t.fn(t.p)
	}
}

func (t *throttle) done() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fn != nil {
		t.fn(t.p)
	}
}

// workers is how many folders are measured at once. Walking many small files is
// latency-bound, so a few in parallel is much faster on an SSD, while more would
// only fight over the disk.
const workers = 4

// parallel runs fn for every index in 0..n-1 with at most `workers` at once and
// returns the first error. Results are written by index, so order is kept.
func parallel(n int, fn func(i int) error) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			errs[i] = fn(i)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// within reports whether path is strictly inside dir.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

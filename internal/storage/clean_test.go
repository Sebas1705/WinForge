package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/storage"
)

// fakeEnv is a PC laid out in a temporary folder.
func fakeEnv(t *testing.T) storage.Env {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "Users", "me")
	e := storage.Env{
		Home:         home,
		LocalAppData: filepath.Join(home, "AppData", "Local"),
		AppData:      filepath.Join(home, "AppData", "Roaming"),
		ProgramData:  filepath.Join(root, "ProgramData"),
		WinDir:       filepath.Join(root, "Windows"),
		SystemDrive:  root,
		Temp:         filepath.Join(home, "AppData", "Local", "Temp"),
	}
	for _, d := range []string{e.LocalAppData, e.AppData, e.ProgramData, e.WinDir, e.Temp,
		filepath.Join(home, "Documents"), filepath.Join(home, "Downloads"), filepath.Join(home, "Desktop")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

var now = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// put writes a file of the given size and age, creating folders.
func put(t *testing.T, path string, size int, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	mt := now.Add(-age)
	if err := os.Chtimes(path, mt, mt); err != nil {
		t.Fatal(err)
	}
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func opts(e storage.Env) storage.Options {
	return storage.Options{Env: e, Now: now, Admin: true, RecycleRoots: []string{}}
}

func category(t *testing.T, cs []storage.Category, id string) storage.Category {
	t.Helper()
	for _, c := range cs {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("no category %q", id)
	return storage.Category{}
}

func outcome(t *testing.T, r storage.Result, id string) storage.Outcome {
	t.Helper()
	for _, o := range r.Outcomes {
		if o.ID == id {
			return o
		}
	}
	t.Fatalf("no outcome %q in %+v", id, r)
	return storage.Outcome{}
}

func TestScanMeasuresExactlyWhatCleanWillRemove(t *testing.T) {
	e := fakeEnv(t)
	day := 24 * time.Hour
	put(t, filepath.Join(e.Temp, "old.tmp"), 1000, 3*day)
	put(t, filepath.Join(e.Temp, "sub", "older.tmp"), 500, 10*day)
	put(t, filepath.Join(e.Temp, "fresh.tmp"), 7000, time.Hour) // may still be in use: kept
	ex := filepath.Join(e.LocalAppData, "Microsoft", "Windows", "Explorer")
	put(t, filepath.Join(ex, "thumbcache_256.db"), 300, day)
	put(t, filepath.Join(ex, "iconcache_32.db"), 200, day)
	put(t, filepath.Join(ex, "other-setting.dat"), 9999, day) // not ours
	put(t, filepath.Join(e.LocalAppData, "go-build", "ab", "cd"), 4000, day)

	cs, err := storage.Scan(context.Background(), opts(e))
	if err != nil {
		t.Fatal(err)
	}
	if c := category(t, cs, "temp-user"); c.Bytes != 1500 || c.Files != 2 || !c.Available || c.Risk != storage.Safe {
		t.Fatalf("temp-user: %+v", c)
	}
	if c := category(t, cs, "thumbnails"); c.Bytes != 500 || c.Files != 2 {
		t.Fatalf("thumbnails must count only its own files: %+v", c)
	}
	if c := category(t, cs, "dev-go"); c.Bytes != 4000 {
		t.Fatalf("dev-go: %+v", c)
	}
	if c := category(t, cs, "cache-chrome"); c.Available || c.Bytes != 0 {
		t.Fatalf("a browser that is not installed is unavailable: %+v", c)
	}
	if c := category(t, cs, "windows-old"); c.Available {
		t.Fatalf("no Windows.old here: %+v", c)
	}

	res := storage.Clean(context.Background(), []string{"temp-user", "thumbnails", "dev-go"}, opts(e))
	for _, id := range []string{"temp-user", "thumbnails", "dev-go"} {
		want := category(t, cs, id)
		got := outcome(t, res, id)
		if got.FreedBytes != want.Bytes || got.Files != want.Files || got.Failed != 0 || got.Error != "" {
			t.Errorf("%s: scan said %+v, clean did %+v", id, want, got)
		}
	}
	if res.FreedBytes != 6000 || res.Files != 5 || res.Failed != 0 {
		t.Fatalf("totals: %+v", res)
	}
	if !exists(filepath.Join(e.Temp, "fresh.tmp")) {
		t.Error("a recent file must be kept")
	}
	if !exists(filepath.Join(ex, "other-setting.dat")) {
		t.Error("a file that is not a thumbnail cache was deleted")
	}
	if exists(filepath.Join(e.Temp, "sub")) {
		t.Error("an emptied folder should go")
	}
	if !exists(e.Temp) || !exists(filepath.Join(e.LocalAppData, "go-build")) {
		t.Error("the category folder itself stays")
	}
	again, _ := storage.Scan(context.Background(), opts(e))
	if c := category(t, again, "temp-user"); c.Bytes != 0 {
		t.Fatalf("scan after clean: %+v", c)
	}
}

func TestBrowserCleaningTouchesOnlyCachesOfEveryProfile(t *testing.T) {
	e := fakeEnv(t)
	base := filepath.Join(e.LocalAppData, "Google", "Chrome", "User Data")
	for _, prof := range []string{"Default", "Profile 1", "Profile [2]"} {
		put(t, filepath.Join(base, prof, "Cache", "Cache_Data", "f_0001"), 100, 0)
		put(t, filepath.Join(base, prof, "Code Cache", "js", "x"), 50, 0)
		put(t, filepath.Join(base, prof, "GPUCache", "data_0"), 25, 0)
		put(t, filepath.Join(base, prof, "Cookies"), 777, 0)
		put(t, filepath.Join(base, prof, "Login Data"), 888, 0)
		put(t, filepath.Join(base, prof, "History"), 999, 0)
	}
	put(t, filepath.Join(base, "Local State"), 10, 0)
	put(t, filepath.Join(e.LocalAppData, "Mozilla", "Firefox", "Profiles", "abc.default", "cache2", "entries", "e1"), 40, 0)
	put(t, filepath.Join(e.LocalAppData, "Mozilla", "Firefox", "Profiles", "abc.default", "cookies.sqlite"), 40, 0)

	res := storage.Clean(context.Background(), []string{"cache-chrome", "cache-firefox"}, opts(e))
	if o := outcome(t, res, "cache-chrome"); o.FreedBytes != 3*175 || o.Failed != 0 {
		t.Fatalf("chrome: %+v", o)
	}
	if o := outcome(t, res, "cache-firefox"); o.FreedBytes != 40 {
		t.Fatalf("firefox: %+v", o)
	}
	for _, prof := range []string{"Default", "Profile 1", "Profile [2]"} {
		for _, keep := range []string{"Cookies", "Login Data", "History"} {
			if !exists(filepath.Join(base, prof, keep)) {
				t.Errorf("%s/%s was deleted: caches only, never the person's data", prof, keep)
			}
		}
	}
	if !exists(filepath.Join(base, "Local State")) || !exists(filepath.Join(e.LocalAppData, "Mozilla", "Firefox", "Profiles", "abc.default", "cookies.sqlite")) {
		t.Error("browser state was deleted")
	}
}

func TestATempVariablePointingAtPersonalFoldersDeletesNothing(t *testing.T) {
	for name, pick := range map[string]func(storage.Env) string{
		"documents": func(e storage.Env) string { return filepath.Join(e.Home, "Documents") },
		"profile":   func(e storage.Env) string { return e.Home },
		"downloads": func(e storage.Env) string { return filepath.Join(e.Home, "Downloads") },
		"drive":     func(e storage.Env) string { return e.SystemDrive },
		"windows":   func(e storage.Env) string { return e.WinDir },
		"appdata":   func(e storage.Env) string { return e.LocalAppData },
		"parent":    func(e storage.Env) string { return filepath.Dir(e.Home) },
	} {
		e := fakeEnv(t)
		precious := filepath.Join(e.Home, "Documents", "thesis.docx")
		put(t, precious, 100, 365*24*time.Hour)
		put(t, filepath.Join(e.Home, "Downloads", "setup.exe"), 100, 365*24*time.Hour)
		put(t, filepath.Join(e.WinDir, "system.dll"), 100, 365*24*time.Hour)
		e.Temp = pick(e)
		before := snapshot(t, filepath.Dir(filepath.Dir(e.Home)))

		cs, err := storage.Scan(context.Background(), opts(e))
		if err != nil {
			t.Fatal(err)
		}
		if c := category(t, cs, "temp-user"); c.Bytes != 0 || c.Files != 0 || c.Available {
			t.Errorf("%s: scanned a folder that is not a temp folder: %+v", name, c)
		}
		res := storage.Clean(context.Background(), []string{"temp-user"}, opts(e))
		if res.FreedBytes != 0 || res.Files != 0 {
			t.Errorf("%s: cleaned %+v", name, res)
		}
		if after := snapshot(t, filepath.Dir(filepath.Dir(e.Home))); after != before {
			t.Errorf("%s: the tree changed:\nbefore %s\nafter  %s", name, before, after)
		}
	}
}

// snapshot lists every path under dir, to compare before and after.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil {
			b.WriteString(p + "\n")
		}
		return nil
	})
	return b.String()
}

func TestATempFolderNamedTempIsAcceptedWherever(t *testing.T) {
	e := fakeEnv(t)
	e.Temp = filepath.Join(filepath.Dir(e.Home), "..", "Scratch", "Temp")
	e.Temp = filepath.Clean(e.Temp)
	put(t, filepath.Join(e.Temp, "a.tmp"), 123, 5*24*time.Hour)
	res := storage.Clean(context.Background(), []string{"temp-user"}, opts(e))
	if o := outcome(t, res, "temp-user"); o.FreedBytes != 123 {
		t.Fatalf("a moved TEMP called Temp is a temp folder: %+v", o)
	}
}

func TestEmptyEnvironmentIsHarmless(t *testing.T) {
	cs, err := storage.Scan(context.Background(), storage.Options{Now: now, RecycleRoots: []string{}})
	if err != nil || len(cs) != len(storage.Specs) {
		t.Fatalf("%v %d", err, len(cs))
	}
	for _, c := range cs {
		if c.Bytes != 0 || c.Files != 0 {
			t.Errorf("%s found things with no folders configured: %+v", c.ID, c)
		}
	}
	res := storage.Clean(context.Background(), []string{"temp-user", "dev-go", "cache-chrome"}, storage.Options{Admin: true, Now: now})
	if res.FreedBytes != 0 || res.Failed != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestLinksAreNeverFollowedOutOfACategory(t *testing.T) {
	e := fakeEnv(t)
	outside := filepath.Join(filepath.Dir(e.Home), "outside")
	put(t, filepath.Join(outside, "keep-me.txt"), 100, 400*24*time.Hour)
	put(t, filepath.Join(e.Temp, "real.tmp"), 10, 5*24*time.Hour)
	link := filepath.Join(e.Temp, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("cannot create symbolic links here: %v", err)
	}
	fileLink := filepath.Join(e.Temp, "filelink.tmp")
	_ = os.Symlink(filepath.Join(outside, "keep-me.txt"), fileLink)

	cs, _ := storage.Scan(context.Background(), opts(e))
	if c := category(t, cs, "temp-user"); c.Bytes != 10 || c.Files != 1 {
		t.Fatalf("a link must not be followed when measuring: %+v", c)
	}
	res := storage.Clean(context.Background(), []string{"temp-user"}, opts(e))
	if outcome(t, res, "temp-user").FreedBytes != 10 {
		t.Fatalf("%+v", res)
	}
	if !exists(filepath.Join(outside, "keep-me.txt")) {
		t.Fatal("a file outside the category was deleted through a link")
	}
}

func TestReadOnlyFilesAreRemovedAndLockedOnesAreCounted(t *testing.T) {
	e := fakeEnv(t)
	ro := filepath.Join(e.Temp, "readonly.tmp")
	put(t, ro, 64, 3*24*time.Hour)
	if err := os.Chmod(ro, 0o444); err != nil {
		t.Fatal(err)
	}
	res := storage.Clean(context.Background(), []string{"temp-user"}, opts(e))
	if o := outcome(t, res, "temp-user"); o.FreedBytes != 64 || o.Failed != 0 || exists(ro) {
		t.Fatalf("a read-only file is still just a temp file: %+v", o)
	}
}

func TestCategoriesNeedingAdministratorRightsAreNotAttemptedWithout(t *testing.T) {
	e := fakeEnv(t)
	f := filepath.Join(e.WinDir, "SoftwareDistribution", "Download", "update.cab")
	put(t, f, 500, 0)
	o := opts(e)
	o.Admin = false
	res := storage.Clean(context.Background(), []string{"windows-update"}, o)
	if got := outcome(t, res, "windows-update"); got.Error != "needs-admin" || got.FreedBytes != 0 || !exists(f) {
		t.Fatalf("%+v", got)
	}
	o.Admin = true
	res = storage.Clean(context.Background(), []string{"windows-update"}, o)
	if got := outcome(t, res, "windows-update"); got.FreedBytes != 500 || exists(f) {
		t.Fatalf("%+v", got)
	}
}

func TestUnknownAndRepeatedCategories(t *testing.T) {
	e := fakeEnv(t)
	put(t, filepath.Join(e.Temp, "a.tmp"), 10, 3*24*time.Hour)
	res := storage.Clean(context.Background(), []string{"../../Windows", "temp-user", "temp-user", ""}, opts(e))
	if len(res.Outcomes) != 3 {
		t.Fatalf("each id once: %+v", res.Outcomes)
	}
	if o := outcome(t, res, "../../Windows"); o.Error != "unknown category" {
		t.Fatalf("%+v", o)
	}
	if res.FreedBytes != 10 {
		t.Fatalf("a repeated id must not count twice: %+v", res)
	}
	if empty := storage.Clean(context.Background(), nil, opts(e)); empty.Outcomes == nil || len(empty.Outcomes) != 0 {
		t.Fatalf("no ids, no outcomes, but never null: %+v", empty)
	}
}

func TestProgressAndCancellation(t *testing.T) {
	e := fakeEnv(t)
	for i := 0; i < 50; i++ {
		put(t, filepath.Join(e.Temp, "d", "f"+string(rune('a'+i%26))+string(rune('a'+i/26))), 10, 3*24*time.Hour)
	}
	var last storage.Progress
	o := opts(e)
	o.Progress = func(p storage.Progress) { last = p }
	if _, err := storage.Scan(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if last.Task != "scan" || last.Files != 50 || last.Bytes != 500 {
		t.Fatalf("the last progress report must carry the totals: %+v", last)
	}
	storage.Clean(context.Background(), []string{"temp-user"}, o)
	if last.Task != "clean" || last.Files != 50 {
		t.Fatalf("%+v", last)
	}

	for i := 0; i < 20; i++ {
		put(t, filepath.Join(e.Temp, "again", "g"+string(rune('a'+i))), 10, 3*24*time.Hour)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := storage.Scan(ctx, opts(e)); err == nil {
		t.Fatal("a cancelled scan must say so")
	}
	res := storage.Clean(ctx, []string{"temp-user", "dev-go"}, opts(e))
	if res.FreedBytes != 0 {
		t.Fatalf("a cancelled clean frees nothing: %+v", res)
	}
	if !exists(filepath.Join(e.Temp, "again", "ga")) {
		t.Fatal("files were deleted after cancelling")
	}
}

func TestEverySpecStaysInsideKnownFolders(t *testing.T) {
	e := fakeEnv(t)
	seen := map[string]bool{}
	bases := []string{e.Home, e.LocalAppData, e.AppData, e.ProgramData, e.WinDir}
	for _, s := range storage.Specs {
		if seen[s.ID] || s.ID == "" {
			t.Errorf("duplicate or empty id %q", s.ID)
		}
		seen[s.ID] = true
		if s.Risk != storage.Safe && s.Risk != storage.Review {
			t.Errorf("%s: unknown risk %q", s.ID, s.Risk)
		}
		if s.Special != "" && s.Dirs != nil {
			t.Errorf("%s: a special category is not a list of folders", s.ID)
		}
		if s.Dirs == nil {
			continue
		}
		for _, p := range s.Dirs(e) {
			ok := s.ID == "temp-user" // TEMP is checked at run time instead
			for _, b := range bases {
				ok = ok || strings.HasPrefix(p, b+string(filepath.Separator))
			}
			if !ok {
				t.Errorf("%s: %s is outside the known folders", s.ID, p)
			}
			for _, personal := range []string{"Documents", "Desktop", "Downloads", "Pictures", "Videos", "Music"} {
				if strings.Contains(p, string(filepath.Separator)+personal) {
					t.Errorf("%s: %s reaches into %s", s.ID, p, personal)
				}
			}
		}
		if s.Dirs(storage.Env{}) != nil && len(s.Dirs(storage.Env{})) > 0 {
			t.Errorf("%s: with no folders configured it still names some: %v", s.ID, s.Dirs(storage.Env{}))
		}
	}
	for _, want := range []string{"temp-user", "recycle-bin", "windows-update", "windows-old", "cache-chrome", "dev-gradle"} {
		if !seen[want] {
			t.Errorf("missing category %s", want)
		}
	}
	// The interface names these ids; keep them stable.
	if len(storage.Specs) != 18 {
		t.Errorf("%d categories: the frontend lists 18 (update both together)", len(storage.Specs))
	}
}

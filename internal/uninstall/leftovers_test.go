package uninstall_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/uninstall"
)

func find(t *testing.T, p *pc, app uninstall.App, others ...uninstall.App) []uninstall.Candidate {
	t.Helper()
	got, _, err := p.finder().Find(context.Background(), app, others)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func paths(cs []uninstall.Candidate) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.Path)
	}
	return out
}

func TestFindListsEachFixedPlaceWithItsReason(t *testing.T) {
	p := newPC(t)
	e := p.env
	p.file(filepath.Join(e.AppData, "Git", "config.json"), 100)
	p.file(filepath.Join(e.LocalAppData, "Git", "cache.bin"), 2000)
	p.file(filepath.Join(e.LocalAppData, "Programs", "Git", "git.exe"), 5000)
	p.file(filepath.Join(e.ProgramData, "Git", "x"), 1)
	p.file(filepath.Join(e.ProgramFiles, "Git", "bin", "git.exe"), 40000)
	p.file(filepath.Join(e.ProgramFilesX86, "Git", "y"), 1)
	p.file(filepath.Join(p.startMenu(), "Git", "Git Bash.lnk"), 1)
	p.file(filepath.Join(p.startMenuAll(), "Git", "Git GUI.lnk"), 1)
	p.file(filepath.Join(p.startMenu(), "Git.lnk"), 1000)
	p.file(filepath.Join(p.desktop(), "Git.lnk"), 1100)
	p.file(filepath.Join(e.Public, "Desktop", "git.LNK"), 1200)
	p.file(filepath.Join(e.OneDrive, "Desktop", "Git.lnk"), 1300)
	newer := epoch.Add(48 * time.Hour)
	p.touched(filepath.Join(e.ProgramFiles, "Git", "bin", "git.exe"), newer)

	got := find(t, p, git)
	byPath := map[string]uninstall.Candidate{}
	for _, c := range got {
		byPath[c.Path] = c
	}
	want := []struct {
		path  string
		kind  uninstall.Kind
		place uninstall.PlaceID
		size  int64
	}{
		{filepath.Join(e.AppData, "Git"), uninstall.KindFolder, uninstall.PlaceAppData, 100},
		{filepath.Join(e.LocalAppData, "Git"), uninstall.KindFolder, uninstall.PlaceLocalAppData, 2000},
		{filepath.Join(e.LocalAppData, "Programs", "Git"), uninstall.KindFolder, uninstall.PlaceLocalPrograms, 5000},
		{filepath.Join(e.ProgramData, "Git"), uninstall.KindFolder, uninstall.PlaceProgramData, 1},
		{filepath.Join(e.ProgramFiles, "Git"), uninstall.KindFolder, uninstall.PlaceProgramFiles, 40000},
		{filepath.Join(e.ProgramFilesX86, "Git"), uninstall.KindFolder, uninstall.PlaceProgramFilesX86, 1},
		{filepath.Join(p.startMenu(), "Git"), uninstall.KindFolder, uninstall.PlaceStartMenu, 1},
		{filepath.Join(p.startMenuAll(), "Git"), uninstall.KindFolder, uninstall.PlaceStartMenuAll, 1},
		{filepath.Join(p.startMenu(), "Git.lnk"), uninstall.KindShortcut, uninstall.PlaceStartMenu, 1000},
		{filepath.Join(p.desktop(), "Git.lnk"), uninstall.KindShortcut, uninstall.PlaceDesktop, 1100},
		{filepath.Join(e.Public, "Desktop", "git.LNK"), uninstall.KindShortcut, uninstall.PlaceDesktopAll, 1200},
		{filepath.Join(e.OneDrive, "Desktop", "Git.lnk"), uninstall.KindShortcut, uninstall.PlaceDesktopOneDrive, 1300},
	}
	if len(got) != len(want) {
		t.Fatalf("found %d, want %d: %v", len(got), len(want), paths(got))
	}
	for _, w := range want {
		c, ok := byPath[w.path]
		if !ok {
			t.Errorf("missing %s", w.path)
			continue
		}
		if c.Kind != w.kind || c.Place != w.place || c.SizeBytes != w.size {
			t.Errorf("%s: kind %s place %s size %d, want %s %s %d", w.path, c.Kind, c.Place, c.SizeBytes, w.kind, w.place, w.size)
		}
		if c.Why.Rule != uninstall.RuleName || c.Why.Value != "Git" {
			t.Errorf("%s: why %+v", w.path, c.Why)
		}
		if c.Modified.IsZero() {
			t.Errorf("%s: no modified time", w.path)
		}
	}
	if m := byPath[filepath.Join(e.ProgramFiles, "Git")].Modified; !m.Equal(newer) {
		t.Fatalf("a folder's modified time is the newest file in it: %v", m)
	}
	if byPath[filepath.Join(e.ProgramFiles, "Git")].Files != 1 {
		t.Fatalf("file count: %+v", byPath[filepath.Join(e.ProgramFiles, "Git")])
	}
}

func TestFindReportsWhereItLookedAndNothingElse(t *testing.T) {
	p := newPC(t)
	_, searched, err := p.finder().Find(context.Background(), git, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range searched {
		ids = append(ids, string(s.Place))
	}
	want := []string{"appdata", "localappdata", "localprograms", "programdata", "programfiles", "programfilesx86", "startmenu", "startmenuall", "desktop", "desktoponedrive", "desktopall"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("places searched %v", ids)
	}
	// A place that does not exist on this PC is simply not searched.
	delete(p.nodes, key(p.env.ProgramFilesX86))
	_, searched, _ = p.finder().Find(context.Background(), git, nil)
	for _, s := range searched {
		if s.Place == uninstall.PlaceProgramFilesX86 {
			t.Fatal("a missing folder cannot have been searched")
		}
	}
}

func TestFindNeverListsNearMissesOrOtherPeoplesFiles(t *testing.T) {
	p := newPC(t)
	e := p.env
	// Names that merely contain the app's name.
	for _, n := range []string{"digital", "GitHub", "Git LFS", "GitKraken", "Gitea", "Git2", "legit", "Git Gui", "git-lfs", "Digital Editions"} {
		p.file(filepath.Join(e.AppData, n, "f"), 1)
		p.file(filepath.Join(e.ProgramFiles, n, "f"), 1)
		p.file(filepath.Join(p.startMenu(), n, "f.lnk"), 1)
		p.file(filepath.Join(p.desktop(), n+".lnk"), 1)
	}
	// Windows's own folders, and the vendor folder Microsoft keeps its parts in.
	for _, n := range []string{"Windows", "Microsoft", "Common Files", "WindowsApps", "Windows NT", "Windows Defender"} {
		p.file(filepath.Join(e.ProgramFiles, n, "f"), 1)
		p.file(filepath.Join(e.LocalAppData, n, "f"), 1)
	}
	// The person's own files.
	p.file(filepath.Join(p.desktop(), "Git.txt"), 10)
	p.file(filepath.Join(p.desktop(), "Git"), 10) // a file with no extension
	p.file(filepath.Join(p.desktop(), "Git.lnk.txt"), 10)
	p.file(filepath.Join(p.desktop(), "Git.url"), 10)
	p.file(filepath.Join(p.desktop(), "notes about git.lnk"), 10)
	p.file(filepath.Join(p.desktop(), "Git Stuff", "Git.lnk"), 10) // a folder of the person's, whatever is inside
	p.file(filepath.Join(e.Home, "Documents", "Git", "notes.txt"), 10)
	p.file(filepath.Join(e.Home, "Documents", "Git.lnk"), 10)
	p.file(filepath.Join(e.Home, "Downloads", "Git", "setup.exe"), 10)
	p.file(filepath.Join(e.Home, "Git", "x"), 10)
	p.file(filepath.Join(e.Home, "Pictures", "Git", "x"), 10)
	p.file(filepath.Join(e.WinDir, "Git", "x"), 10)
	p.file(filepath.Join(e.AppData, "Git.lnk"), 10) // a file, not a folder, in a data place
	p.file(filepath.Join(e.ProgramFiles, "Git.lnk"), 10)
	// Deeper than the fixed places read.
	p.file(filepath.Join(e.AppData, "Vendor", "Git", "x"), 10)
	p.file(filepath.Join(e.LocalAppData, "Programs", "Vendor", "Git", "x"), 10)
	p.file(filepath.Join(e.ProgramFiles, "Common Files", "Git", "x"), 10)
	p.file(filepath.Join(e.LocalAppData, "Temp", "Git", "x"), 10)

	if got := find(t, p, git); len(got) != 0 {
		t.Fatalf("nothing here is Git's: %v", paths(got))
	}
}

func TestFindNestedPublisherFolders(t *testing.T) {
	p := newPC(t)
	e := p.env
	p.file(filepath.Join(e.LocalAppData, "Mozilla", "Firefox", "profile.ini"), 10)
	p.file(filepath.Join(e.LocalAppData, "Mozilla", "Thunderbird", "x"), 10)
	p.file(filepath.Join(e.AppData, "Other", "Firefox", "x"), 10)
	p.file(filepath.Join(e.AppData, "Microsoft", "Firefox", "x"), 10)
	p.file(filepath.Join(e.ProgramFilesX86, "Mozilla Maintenance Service", "x"), 10)
	got := find(t, p, firefox)
	if len(got) != 1 || got[0].Path != filepath.Join(e.LocalAppData, "Mozilla", "Firefox") {
		t.Fatalf("got %v", paths(got))
	}
	if got[0].Why.Rule != uninstall.RuleNested || got[0].Why.Value != `Mozilla\Firefox` || got[0].Place != uninstall.PlaceLocalAppData {
		t.Fatalf("why %+v place %s", got[0].Why, got[0].Place)
	}
	// The publisher's own folder is never a candidate: other apps of the vendor live in it.
	for _, c := range got {
		if filepath.Base(c.Path) == "Mozilla" {
			t.Fatal("the vendor folder must not be offered")
		}
	}
}

func TestFindNeverTouchesMicrosoftsFolders(t *testing.T) {
	p := newPC(t)
	e := p.env
	app := uninstall.App{ID: "edge", Name: "Edge", Publisher: "Microsoft Corporation", Winget: "Microsoft.Edge"}
	p.file(filepath.Join(e.ProgramFilesX86, "Microsoft", "Edge", "x"), 10)
	p.file(filepath.Join(e.LocalAppData, "Microsoft", "Edge", "x"), 10)
	if got := find(t, p, app); len(got) != 0 {
		t.Fatalf("%v", paths(got))
	}
}

func TestFindSkipsWhatAnotherInstalledAppOwns(t *testing.T) {
	p := newPC(t)
	e := p.env
	a := uninstall.App{ID: "foo", Name: "Foo", Publisher: "Acme", Winget: "Acme.Foo"}
	b := uninstall.App{ID: "foo-pro", Name: "Foo Pro", Publisher: "Acme", Winget: "Acme.Foo"} // same product, still installed
	p.file(filepath.Join(e.ProgramFiles, "Foo", "x"), 1)
	p.file(filepath.Join(e.AppData, "Foo", "x"), 1)
	if got := find(t, p, a, b); len(got) != 0 {
		t.Fatalf("a folder another installed app also answers to is not offered: %v", paths(got))
	}
	if got := find(t, p, a); len(got) != 2 {
		t.Fatalf("with no other app installed both are: %v", paths(got))
	}
}

func TestFindSkipsNamesWindowsStillListsAsInstalled(t *testing.T) {
	p := newPC(t)
	p.file(filepath.Join(p.env.ProgramFiles, "Git", "x"), 1)
	p.installed = []uninstall.RegApp{{Name: "git"}}
	if got := find(t, p, git); len(got) != 0 {
		t.Fatalf("a program that is still in Installed apps is not leftover: %v", paths(got))
	}
}

func TestFindNeverEntersOrListsLinks(t *testing.T) {
	p := newPC(t)
	e := p.env
	p.link(filepath.Join(e.ProgramFiles, "Git"), true) // a junction named like the app
	p.link(filepath.Join(e.AppData, "Git"), true)
	p.link(filepath.Join(p.desktop(), "Git.lnk"), false) // not a plain file
	p.link(filepath.Join(e.LocalAppData, "Mozilla"), true)
	p.file(filepath.Join(e.LocalAppData, "Mozilla", "Firefox", "x"), 1) // would be reached through the link
	p.dir(filepath.Join(e.AppData, "Mozilla"))
	p.link(filepath.Join(e.AppData, "Mozilla", "Firefox"), true)
	if got := find(t, p, git); len(got) != 0 {
		t.Fatalf("%v", paths(got))
	}
	if got := find(t, p, firefox); len(got) != 0 {
		t.Fatalf("a link is never followed or listed: %v", paths(got))
	}
}

func TestFindDoesNotChangeAnything(t *testing.T) {
	p := newPC(t)
	p.file(filepath.Join(p.env.ProgramFiles, "Git", "x"), 1)
	p.file(filepath.Join(p.desktop(), "Git.lnk"), 1)
	_ = find(t, p, git)
	if len(p.recycled) != 0 || !p.has(filepath.Join(p.env.ProgramFiles, "Git")) {
		t.Fatalf("a search must only read: recycled %v", p.recycled)
	}
}

func TestFindWhenCancelled(t *testing.T) {
	p := newPC(t)
	p.file(filepath.Join(p.env.AppData, "Git", "x"), 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := p.finder().Find(ctx, git, nil); err == nil {
		t.Fatal("a cancelled search is an error, not an empty answer")
	}
}

func TestPlacesSkipUnknownFolders(t *testing.T) {
	if got := (uninstall.Env{}).Places(); len(got) != 0 {
		t.Fatalf("an empty environment has no places: %v", got)
	}
	got := uninstall.Env{AppData: "relative/path", ProgramFiles: filepath.Join(t.TempDir(), "PF")}.Places()
	if len(got) != 1 || got[0].ID != uninstall.PlaceProgramFiles {
		t.Fatalf("relative folders are ignored: %v", got)
	}
}

// ---- removal ----

func listed(t *testing.T, p *pc, app uninstall.App, others ...uninstall.App) []uninstall.Candidate {
	t.Helper()
	cs := find(t, p, app, others...)
	if len(cs) == 0 {
		t.Fatal("nothing listed")
	}
	return cs
}

func reasonOf(res uninstall.Result, path string) string {
	for _, f := range res.Failed {
		if f.Path == path {
			return f.Reason
		}
	}
	return ""
}

func TestRemoveMovesListedFoldersAndShortcuts(t *testing.T) {
	p := newPC(t)
	e := p.env
	p.file(filepath.Join(e.AppData, "Git", "a"), 100)
	p.file(filepath.Join(e.ProgramFiles, "Git", "b"), 300)
	p.file(filepath.Join(p.desktop(), "Git.lnk"), 7)
	p.file(filepath.Join(p.desktop(), "keep.txt"), 7)
	cs := listed(t, p, git)
	res := p.finder().Remove(context.Background(), git, nil, cs)
	if res.Moved != 3 || res.FreedBytes != 407 || len(res.Failed) != 0 {
		t.Fatalf("%+v", res)
	}
	if p.has(filepath.Join(e.AppData, "Git")) || p.has(filepath.Join(p.desktop(), "Git.lnk")) || !p.has(filepath.Join(p.desktop(), "keep.txt")) {
		t.Fatal("only what was listed is gone")
	}
}

func TestRemoveReportsEachFailureAndLeavesTheFolder(t *testing.T) {
	p := newPC(t)
	e := p.env
	p.file(filepath.Join(e.AppData, "Git", "a"), 100)
	p.file(filepath.Join(e.ProgramFiles, "Git", "b"), 300)
	cs := listed(t, p, git)
	p.stuck[key(filepath.Join(e.ProgramFiles, "Git"))] = true // in use
	res := p.finder().Remove(context.Background(), git, nil, cs)
	if res.Moved != 1 || len(res.Failed) != 1 || res.Failed[0].Reason != uninstall.ReasonNotMoved || res.Failed[0].Path != filepath.Join(e.ProgramFiles, "Git") {
		t.Fatalf("%+v", res)
	}
	if !p.has(filepath.Join(e.ProgramFiles, "Git", "b")) {
		t.Fatal("a folder the Recycle Bin could not take stays whole")
	}
}

func TestRemoveRefusesWhatIsNotAPlainFolderInAFixedPlace(t *testing.T) {
	p := newPC(t)
	e := p.env
	folder := func(path string) uninstall.Candidate {
		return uninstall.Candidate{Path: path, Kind: uninstall.KindFolder}
	}
	for _, d := range []string{"Windows", "Program Files", "Users", "x"} {
		p.dir(filepath.Join(p.root, d))
	}
	p.file(filepath.Join(e.AppData, "Git", "a"), 1)
	p.file(filepath.Join(p.root, "Git", "a"), 1)   // directly under the drive root: not a fixed place
	p.file(filepath.Join(e.WinDir, "Git", "a"), 1) // inside Windows
	p.file(filepath.Join(e.Home, "Documents", "Git", "a"), 1)
	p.file(filepath.Join(e.Home, "Desktop", "Git", "a"), 1)
	p.file(filepath.Join(e.AppData, "Vendor", "Deep", "Git", "a"), 1) // too deep
	p.link(filepath.Join(e.LocalAppData, "Git"), true)
	p.dir(filepath.Join(e.LocalAppData, "Mozilla"))
	p.link(filepath.Join(e.LocalAppData, "Mozilla", "Firefox"), true)
	p.link(filepath.Join(e.ProgramData, "Mozilla"), true)
	p.dir(filepath.Join(e.ProgramData, "Mozilla", "Firefox")) // reached only through a link

	bad := map[string]string{
		e.AppData:                             uninstall.ReasonProtected, // a place itself
		e.LocalAppData:                        uninstall.ReasonProtected,
		e.ProgramFiles:                        uninstall.ReasonProtected,
		e.ProgramFilesX86:                     uninstall.ReasonProtected,
		e.ProgramData:                         uninstall.ReasonProtected,
		e.WinDir:                              uninstall.ReasonProtected,
		e.Home:                                uninstall.ReasonProtected,
		p.root:                                uninstall.ReasonProtected, // a drive root
		filepath.Join(e.Home, "Documents"):    uninstall.ReasonProtected,
		p.desktop():                           uninstall.ReasonProtected,
		p.startMenu():                         uninstall.ReasonProtected,
		filepath.Join(e.AppData, "Microsoft"): uninstall.ReasonProtected,
		filepath.Join(e.LocalAppData, "Programs"):           uninstall.ReasonProtected,
		filepath.Join(p.root, "Git"):                        uninstall.ReasonPlace,
		filepath.Join(e.WinDir, "Git"):                      uninstall.ReasonProtected,
		filepath.Join(e.Home, "Documents", "Git"):           uninstall.ReasonProtected,
		filepath.Join(e.Home, "Desktop", "Git"):             uninstall.ReasonPlace,
		filepath.Join(e.AppData, "Vendor", "Deep", "Git"):   uninstall.ReasonPlace,
		filepath.Join(e.LocalAppData, "Git"):                uninstall.ReasonLink,
		filepath.Join(e.LocalAppData, "Mozilla", "Firefox"): uninstall.ReasonLink,
		filepath.Join(e.ProgramData, "Mozilla", "Firefox"):  uninstall.ReasonLink, // through a linked publisher folder
		filepath.Join(e.AppData, "Gone"):                    uninstall.ReasonMissing,
		"Git":                                               uninstall.ReasonPlace, // relative
		e.AppData + string(filepath.Separator) + ".." + string(filepath.Separator) + "Roaming" + string(filepath.Separator) + "Git": uninstall.ReasonPlace, // not clean
		"": uninstall.ReasonPlace,
	}
	var items []uninstall.Candidate
	for path := range bad {
		items = append(items, folder(path))
	}
	res := p.finder().Remove(context.Background(), git, nil, items)
	if res.Moved != 0 || len(p.recycled) != 0 {
		t.Fatalf("nothing may be moved: %+v, asked %v", res, p.recycled)
	}
	for path, want := range bad {
		if got := reasonOf(res, path); got != want {
			t.Errorf("%q: reason %q, want %q", path, got, want)
		}
	}
	if !p.has(filepath.Join(e.AppData, "Git")) {
		t.Fatal("untouched")
	}
}

func TestRemoveRefusesAKindThatDoesNotMatchTheDisk(t *testing.T) {
	p := newPC(t)
	e := p.env
	p.file(filepath.Join(e.AppData, "Git", "a"), 1)
	p.file(filepath.Join(p.desktop(), "Git.lnk"), 1)
	p.file(filepath.Join(p.desktop(), "Git.txt"), 1)
	p.file(filepath.Join(e.ProgramFiles, "Git.lnk"), 1)
	items := []uninstall.Candidate{
		{Path: filepath.Join(e.AppData, "Git"), Kind: uninstall.KindShortcut},          // a folder claiming to be a shortcut
		{Path: filepath.Join(p.desktop(), "Git.lnk"), Kind: uninstall.KindFolder},      // a shortcut claiming to be a folder
		{Path: filepath.Join(p.desktop(), "Git.txt"), Kind: uninstall.KindShortcut},    // a file that is not a .lnk
		{Path: filepath.Join(e.ProgramFiles, "Git.lnk"), Kind: uninstall.KindShortcut}, // a shortcut where shortcuts are not read
		{Path: filepath.Join(e.AppData, "Git"), Kind: "other"},
	}
	res := p.finder().Remove(context.Background(), git, nil, items)
	if res.Moved != 0 || len(p.recycled) != 0 || len(res.Failed) != len(items) {
		t.Fatalf("%+v %v", res, p.recycled)
	}
}

func TestRemoveProvesTheNameAgain(t *testing.T) {
	p := newPC(t)
	e := p.env
	p.file(filepath.Join(e.AppData, "GitHub", "a"), 1)
	p.file(filepath.Join(e.AppData, "Git", "a"), 1)
	p.file(filepath.Join(e.AppData, "Windows", "a"), 1)
	p.file(filepath.Join(e.ProgramFiles, "Common Files", "a"), 1)
	items := []uninstall.Candidate{
		{Path: filepath.Join(e.AppData, "GitHub"), Kind: uninstall.KindFolder, Why: uninstall.Why{Rule: uninstall.RuleName, Value: "Git"}}, // a made-up reason
		{Path: filepath.Join(e.AppData, "Windows"), Kind: uninstall.KindFolder},
		{Path: filepath.Join(e.ProgramFiles, "Common Files"), Kind: uninstall.KindFolder},
	}
	res := p.finder().Remove(context.Background(), git, nil, items)
	if res.Moved != 0 || len(p.recycled) != 0 {
		t.Fatalf("a path that is not named like the app is refused whatever the request says: %+v", res)
	}
	for _, it := range items {
		if reasonOf(res, it.Path) != uninstall.ReasonChanged {
			t.Errorf("%s: %q", it.Path, reasonOf(res, it.Path))
		}
	}
}

func TestRemoveRefusesWhenAnotherInstalledAppNowOwnsTheName(t *testing.T) {
	p := newPC(t)
	e := p.env
	a := uninstall.App{ID: "foo", Name: "Foo", Publisher: "Acme", Winget: "Acme.Foo"}
	b := uninstall.App{ID: "foo-pro", Name: "Foo Pro", Publisher: "Acme", Winget: "Acme.Foo"}
	p.file(filepath.Join(e.AppData, "Foo", "x"), 1)
	cs := listed(t, p, a)
	res := p.finder().Remove(context.Background(), a, []uninstall.App{b}, cs)
	if res.Moved != 0 || reasonOf(res, cs[0].Path) != uninstall.ReasonChanged {
		t.Fatalf("%+v", res)
	}
	// ...or when the registry now lists it.
	p.installed = []uninstall.RegApp{{Name: "Foo"}}
	res = p.finder().Remove(context.Background(), a, nil, cs)
	if res.Moved != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestRemoveNestedAndShortcutNames(t *testing.T) {
	p := newPC(t)
	e := p.env
	p.file(filepath.Join(e.LocalAppData, "Mozilla", "Firefox", "x"), 5)
	p.file(filepath.Join(p.desktop(), "Mozilla Firefox.lnk"), 5)
	cs := listed(t, p, firefox)
	if len(cs) != 2 {
		t.Fatalf("%v", paths(cs))
	}
	res := p.finder().Remove(context.Background(), firefox, nil, cs)
	if res.Moved != 2 {
		t.Fatalf("%+v", res)
	}
	if !p.has(filepath.Join(e.LocalAppData, "Mozilla")) {
		t.Fatal("the vendor folder stays")
	}
}

func TestRemoveAlreadyGoneAndCancelled(t *testing.T) {
	p := newPC(t)
	p.file(filepath.Join(p.env.AppData, "Git", "a"), 1)
	cs := listed(t, p, git)
	delete(p.nodes, key(cs[0].Path))
	res := p.finder().Remove(context.Background(), git, nil, cs)
	if res.Moved != 0 || reasonOf(res, cs[0].Path) != uninstall.ReasonMissing {
		t.Fatalf("%+v", res)
	}
	p.file(filepath.Join(p.env.AppData, "Git", "a"), 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res = p.finder().Remove(ctx, git, nil, cs)
	if res.Moved != 0 || len(p.recycled) != 0 || len(res.Failed) != 1 {
		t.Fatalf("%+v", res)
	}
}

func TestRemoveWithNothingDoesNothing(t *testing.T) {
	p := newPC(t)
	res := p.finder().Remove(context.Background(), git, nil, nil)
	if res.Moved != 0 || res.Failed == nil || len(p.recycled) != 0 {
		t.Fatalf("%+v", res)
	}
}

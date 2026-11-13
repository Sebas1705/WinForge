package storage_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/Sebas1705/WinForge/internal/storage"
)

func write(t *testing.T, path string, content []byte, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func names(fs []storage.BigFile) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return out
}

func TestBigFilesAreTheLargestFirstAndBounded(t *testing.T) {
	root := t.TempDir()
	for name, size := range map[string]int{"a.bin": 100, "b.bin": 5000, "c.bin": 300, "d.bin": 70000, "e.bin": 20, "zero.bin": 0} {
		write(t, filepath.Join(root, "x", name), make([]byte, size), now)
	}
	write(t, filepath.Join(root, "$RECYCLE.BIN", "big.bin"), make([]byte, 999999), now)
	write(t, filepath.Join(root, "System Volume Information", "huge.bin"), make([]byte, 999999), now)

	got, err := storage.BigFiles(context.Background(), root, 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names(got), ",") != "d.bin,b.bin,c.bin" || got[0].SizeBytes != 70000 || got[0].Path != filepath.Join(root, "x", "d.bin") || got[0].Modified.IsZero() {
		t.Fatalf("%+v", got)
	}
	all, _ := storage.BigFiles(context.Background(), root, 500, nil)
	if len(all) != 5 {
		t.Fatalf("empty files and system folders are not listed: %v", names(all))
	}
	for _, f := range all {
		if f.Name == "zero.bin" || strings.Contains(f.Path, "RECYCLE") {
			t.Errorf("unexpected %s", f.Path)
		}
	}
	for _, bad := range []int{0, -4, 100000} {
		if r, err := storage.BigFiles(context.Background(), root, bad, nil); err != nil || len(r) == 0 || len(r) > 500 {
			t.Errorf("limit %d must be clamped, got %d %v", bad, len(r), err)
		}
	}
}

func TestBigFilesEvictsSmallerOnesWhileWalking(t *testing.T) {
	root := t.TempDir()
	for i := 1; i <= 60; i++ {
		write(t, filepath.Join(root, "f", string(rune('a'+i%26))+string(rune('A'+i/26))), make([]byte, i*10), now)
	}
	got, _ := storage.BigFiles(context.Background(), root, 5, nil)
	var sizes []int64
	for _, f := range got {
		sizes = append(sizes, f.SizeBytes)
	}
	want := []int64{600, 590, 580, 570, 560}
	for i := range want {
		if sizes[i] != want[i] {
			t.Fatalf("%v", sizes)
		}
	}
}

func TestBigFilesAndFoldersRefuseBadRoots(t *testing.T) {
	file := filepath.Join(t.TempDir(), "f.txt")
	write(t, file, []byte("x"), now)
	for name, root := range map[string]string{"relative": "some/dir", "missing": filepath.Join(t.TempDir(), "nope"), "a file": file, "empty": ""} {
		if _, err := storage.BigFiles(context.Background(), root, 10, nil); err == nil {
			t.Errorf("%s: BigFiles must refuse", name)
		}
		if _, err := storage.FolderSizes(context.Background(), root, nil); err == nil {
			t.Errorf("%s: FolderSizes must refuse", name)
		}
		if _, err := storage.FindDuplicates(context.Background(), []string{root}, 1, nil); err == nil {
			t.Errorf("%s: FindDuplicates must refuse", name)
		}
	}
}

func TestFolderSizesSumEachFolderAndSkipLinks(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "big", "a"), make([]byte, 1000), now)
	write(t, filepath.Join(root, "big", "deep", "b"), make([]byte, 2000), now)
	write(t, filepath.Join(root, "small", "c"), make([]byte, 10), now)
	write(t, filepath.Join(root, "loose.txt"), make([]byte, 9999), now) // a file, not a folder
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	write(t, filepath.Join(outside, "huge"), make([]byte, 50000), now)
	_ = os.Symlink(outside, filepath.Join(root, "link"))

	got, err := storage.FolderSizes(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, f := range got {
		order = append(order, f.Name)
	}
	if strings.Join(order, ",") != "big,small,empty" || got[0].SizeBytes != 3000 || got[0].Files != 2 || got[0].Path != filepath.Join(root, "big") {
		t.Fatalf("%+v", got)
	}
	none, err := storage.FolderSizes(context.Background(), outside, nil)
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("no folders gives an empty list, never null: %v %v", none, err)
	}
}

func TestBigFilesAndFoldersCanBeCancelled(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "d", "a"), make([]byte, 10), now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := storage.BigFiles(ctx, root, 5, nil); err == nil {
		t.Error("BigFiles")
	}
	if _, err := storage.FolderSizes(ctx, root, nil); err == nil {
		t.Error("FolderSizes")
	}
	if _, err := storage.FindDuplicates(ctx, []string{root}, 1, nil); err == nil {
		t.Error("FindDuplicates")
	}
}

func groupPaths(g storage.DupGroup, root string) []string {
	var out []string
	for _, f := range g.Files {
		r, _ := filepath.Rel(root, f.Path)
		out = append(out, filepath.ToSlash(r))
	}
	return out
}

func TestDuplicatesAreFilesWithTheSameContent(t *testing.T) {
	root := t.TempDir()
	same := bytes.Repeat([]byte("A"), 5000)
	old := now.Add(-48 * time.Hour)
	write(t, filepath.Join(root, "orig.bin"), same, old)
	write(t, filepath.Join(root, "copy", "orig (1).bin"), same, now)
	write(t, filepath.Join(root, "copy", "newest.bin"), same, now.Add(time.Hour))
	write(t, filepath.Join(root, "other.bin"), bytes.Repeat([]byte("B"), 5000), now)             // same size, different
	write(t, filepath.Join(root, "tail1.bin"), append(bytes.Repeat([]byte("C"), 70000), 1), now) // same first 64 KB,
	write(t, filepath.Join(root, "tail2.bin"), append(bytes.Repeat([]byte("C"), 70000), 2), now) // different end
	write(t, filepath.Join(root, "tiny1.txt"), []byte("hi"), now)
	write(t, filepath.Join(root, "tiny2.txt"), []byte("hi"), now)
	write(t, filepath.Join(root, "node_modules", "pkg", "dup.bin"), same, now) // dependencies repeat on purpose
	write(t, filepath.Join(root, ".git", "objects", "dup.bin"), same, now)

	gs, err := storage.FindDuplicates(context.Background(), []string{root}, 1000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 1 {
		t.Fatalf("exactly one group: %+v", gs)
	}
	g := gs[0]
	if strings.Join(groupPaths(g, root), ",") != "orig.bin,copy/orig (1).bin,copy/newest.bin" || g.SizeBytes != 5000 || g.Waste() != 10000 || len(g.ID) != 12 {
		t.Fatalf("oldest first, so the copy to keep is first: %+v %v", g, groupPaths(g, root))
	}
	small, _ := storage.FindDuplicates(context.Background(), []string{root}, 1, nil)
	if len(small) != 2 {
		t.Fatalf("with a low minimum the tiny pair counts too: %d", len(small))
	}
	if gs[0].ID == small[1].ID {
		t.Fatal("group ids must differ")
	}
}

func TestDuplicatesOrderedByWasteAndNestedRootsAreWalkedOnce(t *testing.T) {
	root := t.TempDir()
	big := bytes.Repeat([]byte("X"), 100000)
	small := bytes.Repeat([]byte("y"), 2000)
	for i := 0; i < 2; i++ {
		write(t, filepath.Join(root, "s", string(rune('a'+i))), small, now)
	}
	for i := 0; i < 3; i++ {
		write(t, filepath.Join(root, "b", string(rune('a'+i))), big, now.Add(time.Duration(i)*time.Minute))
	}
	gs, _ := storage.FindDuplicates(context.Background(), []string{root, filepath.Join(root, "b"), root}, 1000, nil)
	if len(gs) != 2 || gs[0].SizeBytes != 100000 || len(gs[0].Files) != 3 {
		t.Fatalf("biggest waste first, and overlapping roots must not double the copies: %+v", gs)
	}
}

func TestHardLinksAreNotDuplicates(t *testing.T) {
	root := t.TempDir()
	data := bytes.Repeat([]byte("H"), 4000)
	write(t, filepath.Join(root, "a.bin"), data, now)
	if err := os.Link(filepath.Join(root, "a.bin"), filepath.Join(root, "b.bin")); err != nil {
		t.Skipf("no hard links here: %v", err)
	}
	gs, _ := storage.FindDuplicates(context.Background(), []string{root}, 1000, nil)
	if len(gs) != 0 {
		t.Fatalf("removing one hard link frees nothing, so it is not a duplicate: %+v", gs)
	}
	write(t, filepath.Join(root, "real-copy.bin"), data, now)
	gs, _ = storage.FindDuplicates(context.Background(), []string{root}, 1000, nil)
	if len(gs) != 1 || len(gs[0].Files) != 2 {
		t.Fatalf("a real copy next to a hard-linked pair is one duplicate of it: %+v", gs)
	}
}

func TestDuplicatesNeverEmptyAndProgressReports(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "only.bin"), []byte("one"), now)
	var last storage.Progress
	gs, err := storage.FindDuplicates(context.Background(), []string{root}, 1, func(p storage.Progress) { last = p })
	if err != nil || gs == nil || len(gs) != 0 {
		t.Fatalf("no duplicates is an empty list, never null: %v %v", gs, err)
	}
	if last.Task != "dups" || last.Files != 1 {
		t.Fatalf("%+v", last)
	}
}

// fakeRunner records commands and runs an optional effect for each.
type fakeRunner struct {
	mu     sync.Mutex
	calls  [][]string
	effect func(name string, args []string)
	err    error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{name}, args...))
	f.mu.Unlock()
	if f.effect != nil {
		f.effect(name, args)
	}
	return nil, f.err
}

// decode reads the script back out of the -EncodedCommand argument.
func decode(t *testing.T, args []string) string {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(args[len(args)-1])
	if err != nil {
		t.Fatal(err)
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u))
}

func TestRecycleCountsOnlyFilesThatAreActuallyGone(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, "f"+string(rune('0'+i))+".bin")
		write(t, p, make([]byte, 100*(i+1)), now)
		paths = append(paths, p)
	}
	// Windows "recycles" the first three; the other two are in use.
	run := &fakeRunner{effect: func(string, []string) {
		for _, p := range paths[:3] {
			_ = os.Remove(p)
		}
	}}
	moved, freed, failed := storage.Recycle(context.Background(), run, paths)
	if moved != 3 || freed != 100+200+300 || len(failed) != 2 || failed[0] != paths[3] {
		t.Fatalf("moved %d freed %d failed %v", moved, freed, failed)
	}
	if !exists(paths[3]) || !exists(paths[4]) {
		t.Fatal("files that could not be moved must stay")
	}
}

func TestRecycleRefusesNonFilesAndBatchesLongLists(t *testing.T) {
	dir := t.TempDir()
	if m, f, fl := storage.Recycle(context.Background(), nil, []string{filepath.Join(dir, "x")}); m != 0 || f != 0 || len(fl) != 1 {
		t.Fatalf("no runner, nothing moves: %d %d %v", m, f, fl)
	}
	sub := filepath.Join(dir, "folder")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	run := &fakeRunner{}
	m, _, failed := storage.Recycle(context.Background(), run, []string{sub, filepath.Join(dir, "missing")})
	if m != 0 || len(failed) != 2 || len(run.calls) != 0 {
		t.Fatalf("folders and missing files are refused without running anything: %d %v %d", m, failed, len(run.calls))
	}
	var paths []string
	for i := 0; i < 95; i++ {
		p := filepath.Join(dir, "many", "n"+string(rune('a'+i%26))+string(rune('a'+i/26)))
		write(t, p, []byte("x"), now)
		paths = append(paths, p)
	}
	run = &fakeRunner{effect: func(name string, args []string) {
		for _, p := range paths {
			_ = os.Remove(p)
		}
	}}
	m, _, failed = storage.Recycle(context.Background(), run, paths)
	if m != 95 || len(failed) != 0 || len(run.calls) != 3 {
		t.Fatalf("95 paths go in 3 batches of at most 40: moved %d, %d calls", m, len(run.calls))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	write(t, filepath.Join(dir, "late"), []byte("x"), now)
	if m, _, fl := storage.Recycle(ctx, &fakeRunner{}, []string{filepath.Join(dir, "late")}); m != 0 || len(fl) != 1 {
		t.Fatalf("cancelled: %d %v", m, fl)
	}
}

func TestRecyclePathsTravelEncodedSoNoFileNameCanInjectCommands(t *testing.T) {
	dir := t.TempDir()
	evil := filepath.Join(dir, "a'; Remove-Item -Recurse C:\\ #$(calc).bin")
	if strings.ContainsAny(evil, `<>:"|?*`) && os.PathSeparator == '\\' {
		evil = filepath.Join(dir, "a'; Remove-Item -Recurse #$(calc).bin")
	}
	write(t, evil, []byte("x"), now)
	run := &fakeRunner{}
	storage.Recycle(context.Background(), run, []string{evil})
	if len(run.calls) != 1 {
		t.Fatalf("%v", run.calls)
	}
	call := run.calls[0]
	script := decode(t, call)
	for _, leak := range []string{"Remove-Item", "calc", "'; "} {
		if strings.Contains(script, leak) {
			t.Fatalf("a file name leaked into the command text (%q): %s", leak, script)
		}
	}
	if !strings.Contains(script, "SendToRecycleBin") || strings.Contains(script, "DeletePermanently") {
		t.Fatalf("must send to the Recycle Bin, never delete for good: %s", script)
	}
	if call[0] != "powershell.exe" || !contains(call, "-NoProfile") || !contains(call, "-EncodedCommand") {
		t.Fatalf("%v", call)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestRecycleBinCategoryIsMeasuredBeforeAndAfter(t *testing.T) {
	e := fakeEnv(t)
	bin := filepath.Join(t.TempDir(), "$Recycle.Bin")
	put(t, filepath.Join(bin, "S-1-5-21-1", "$R123.iso"), 5000, 0)
	put(t, filepath.Join(bin, "S-1-5-21-1", "$I123.iso"), 100, 0)
	o := opts(e)
	o.RecycleRoots = []string{bin, filepath.Join(t.TempDir(), "no-bin")}
	cs, _ := storage.Scan(context.Background(), o)
	if c := category(t, cs, "recycle-bin"); c.Bytes != 5100 || c.Files != 2 || !c.Available {
		t.Fatalf("%+v", c)
	}
	run := &fakeRunner{effect: func(name string, args []string) {
		if name == "powershell.exe" {
			_ = os.RemoveAll(filepath.Join(bin, "S-1-5-21-1"))
		}
	}}
	o.Runner = run
	res := storage.Clean(context.Background(), []string{"recycle-bin"}, o)
	got := outcome(t, res, "recycle-bin")
	if got.FreedBytes != 5100 || got.Files != 2 || got.Failed != 0 || got.Error != "" {
		t.Fatalf("what is freed is measured, not assumed: %+v", got)
	}
	if len(run.calls) != 1 || run.calls[0][0] != "powershell.exe" || !strings.Contains(strings.Join(run.calls[0], " "), "Clear-RecycleBin") {
		t.Fatalf("%v", run.calls)
	}

	// If Windows fails, the error is reported and nothing is claimed as freed.
	put(t, filepath.Join(bin, "S-1", "$Rabc"), 700, 0)
	o.Runner = &fakeRunner{err: os.ErrPermission}
	res = storage.Clean(context.Background(), []string{"recycle-bin"}, o)
	if got := outcome(t, res, "recycle-bin"); got.Error == "" || got.FreedBytes != 0 || got.Failed != 1 {
		t.Fatalf("%+v", got)
	}
	o.Runner = nil
	if got := outcome(t, storage.Clean(context.Background(), []string{"recycle-bin"}, o), "recycle-bin"); got.Error == "" {
		t.Fatalf("no runner is an error, not a silent success: %+v", got)
	}
}

func TestWindowsOldIsTakenOverThenRemovedByTheExactSteps(t *testing.T) {
	e := fakeEnv(t)
	old := filepath.Join(e.SystemDrive, "Windows.old")
	put(t, filepath.Join(old, "Windows", "System32", "big.dll"), 9000, 0)
	o := opts(e)
	cs, _ := storage.Scan(context.Background(), o)
	if c := category(t, cs, "windows-old"); !c.Available || c.Bytes != 9000 || c.Risk != storage.Review || !c.NeedsAdmin {
		t.Fatalf("%+v", c)
	}
	run := &fakeRunner{effect: func(name string, args []string) {
		if name == "cmd.exe" {
			_ = os.RemoveAll(old)
		}
	}}
	o.Runner = run
	o.Admin = false
	if got := outcome(t, storage.Clean(context.Background(), []string{"windows-old"}, o), "windows-old"); got.Error != "needs-admin" || len(run.calls) != 0 || !exists(old) {
		t.Fatalf("not without administrator rights: %+v", got)
	}
	o.Admin = true
	got := outcome(t, storage.Clean(context.Background(), []string{"windows-old"}, o), "windows-old")
	if got.FreedBytes != 9000 || got.Error != "" || exists(old) {
		t.Fatalf("%+v", got)
	}
	var tools []string
	for _, c := range run.calls {
		tools = append(tools, c[0])
		if !contains(c, old) {
			t.Errorf("every step works on Windows.old and nothing else: %v", c)
		}
	}
	if strings.Join(tools, ",") != "takeown.exe,icacls.exe,cmd.exe" {
		t.Fatalf("%v", tools)
	}
	// When taking ownership fails, nothing is removed.
	put(t, filepath.Join(old, "x"), 10, 0)
	o.Runner = &fakeRunner{err: os.ErrPermission}
	if got := outcome(t, storage.Clean(context.Background(), []string{"windows-old"}, o), "windows-old"); got.Error == "" || !exists(filepath.Join(old, "x")) {
		t.Fatalf("%+v", got)
	}
}

func TestDefaultFoldersAreUsable(t *testing.T) {
	d := storage.DefaultFolders()
	for name, p := range map[string]string{"home": d.Home, "downloads": d.Downloads, "documents": d.Documents, "pictures": d.Pictures, "videos": d.Videos} {
		if p == "" || !filepath.IsAbs(p) {
			t.Errorf("%s: %q", name, p)
		}
	}
	for _, v := range storage.Volumes() {
		if v.Letter == "" || v.TotalBytes <= 0 || v.FreeBytes < 0 || v.FreeBytes > v.TotalBytes {
			t.Errorf("bad volume %+v", v)
		}
	}
	if e := storage.DefaultEnv(); e.SystemDrive == "" {
		t.Error("SystemDrive falls back to C:")
	}
}

package settings_test

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/settings"
)

func makeBackup(t *testing.T, o settings.Options) ([]byte, *settings.Manifest) {
	t.Helper()
	var buf bytes.Buffer
	m, err := settings.Create(&buf, o)
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), m
}

func TestFullBackupCarriesAppsSettingsAndFolders(t *testing.T) {
	src := roots(t)
	write(t, filepath.Join(src["home"], ".gitconfig"), "[user]\n")
	docs := filepath.Join(t.TempDir(), "Mis Documentos")
	write(t, filepath.Join(docs, "a.txt"), "A")
	write(t, filepath.Join(docs, "sub/b.txt"), "B")

	z, m := makeBackup(t, settings.Options{
		IDs: []string{"git"}, Roots: src, Profile: []byte(`{"winforge":1}`), Apps: 12,
		Folders: []string{docs}, Host: "PC-VIEJO", App: "v0.6.0",
	})
	if !m.HasProfile || m.Apps != 12 || m.Host != "PC-VIEJO" || len(m.Folders) != 1 || m.Folders[0].Files != 2 || m.Folders[0].Name != "Mis Documentos" {
		t.Fatalf("%+v", m)
	}
	got, err := settings.ReadManifest(bytes.NewReader(z), int64(len(z)))
	if err != nil || got.Format != settings.FormatVersion || len(got.Sums) != 4 {
		t.Fatalf("%v %+v", err, got)
	}
	prof, err := settings.Profile(bytes.NewReader(z), int64(len(z)))
	if err != nil || string(prof) != `{"winforge":1}` {
		t.Fatalf("%q %v", prof, err)
	}
	in, err := settings.Verify(bytes.NewReader(z), int64(len(z)))
	if err != nil || !in.OK() || !in.Checked || in.Files != 4 {
		t.Fatalf("%v %+v", err, in)
	}

	// Folders go where the person says, under their own name; existing files stay.
	dest := t.TempDir()
	write(t, filepath.Join(dest, "Mis Documentos/a.txt"), "MINE")
	res, err := settings.RestoreFolders(bytes.NewReader(z), int64(len(z)), dest, []string{"Mis Documentos"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 1 || res.Kept != 1 || res.Same != 0 {
		t.Fatalf("%+v", res)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "Mis Documentos/a.txt")); string(b) != "MINE" {
		t.Fatalf("an existing file was overwritten: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "Mis Documentos/sub/b.txt")); string(b) != "B" {
		t.Fatalf("b.txt: %q", b)
	}
	res, _ = settings.RestoreFolders(bytes.NewReader(z), int64(len(z)), dest, []string{"Mis Documentos"})
	if res.Written != 0 || res.Same != 1 || res.Kept != 1 {
		t.Fatalf("a second restore must change nothing: %+v", res)
	}
	if _, err := settings.RestoreFolders(bytes.NewReader(z), int64(len(z)), filepath.Join(dest, "missing"), nil); err == nil {
		t.Fatal("restoring into a folder that does not exist must say so")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dest, "*", "*", "*.winforge-part"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

// rewrite builds a copy of an archive with one entry replaced or added.
func rewrite(t *testing.T, z []byte, name string, content string) []byte {
	t.Helper()
	zr, _ := zip.NewReader(bytes.NewReader(z), int64(len(z)))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	seen := false
	for _, f := range zr.File {
		w, _ := zw.Create(f.Name)
		if f.Name == name {
			_, _ = w.Write([]byte(content))
			seen = true
			continue
		}
		rc, _ := f.Open()
		_, _ = io.Copy(w, rc)
		rc.Close()
	}
	if !seen {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(content))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func TestTamperedEntriesAreDetectedAndNeverWritten(t *testing.T) {
	src := roots(t)
	write(t, filepath.Join(src["home"], ".gitconfig"), "good")
	z, _ := makeBackup(t, settings.Options{IDs: []string{"git"}, Roots: src})

	edited := rewrite(t, z, "git/home/.gitconfig", "evil")
	in, err := settings.Verify(bytes.NewReader(edited), int64(len(edited)))
	if err != nil || in.OK() || len(in.Damaged) != 1 || in.Damaged[0] != "git/home/.gitconfig" {
		t.Fatalf("%v %+v", err, in)
	}
	dst := roots(t)
	res, err := settings.Restore(bytes.NewReader(edited), int64(len(edited)), []string{"git"}, dst, nil)
	if err != nil || res.Restored != 0 || len(res.Skipped) != 1 {
		t.Fatalf("an altered entry must be skipped: %v %+v", err, res)
	}
	if _, err := os.Stat(filepath.Join(dst["home"], ".gitconfig")); err == nil {
		t.Fatal("an altered file was written")
	}

	added := rewrite(t, z, "git/home/.gitignore_global", "sneaked in")
	in, _ = settings.Verify(bytes.NewReader(added), int64(len(added)))
	if len(in.Unlisted) != 1 {
		t.Fatalf("an entry added after the backup must be flagged: %+v", in)
	}
	res, _ = settings.Restore(bytes.NewReader(added), int64(len(added)), []string{"git"}, dst, nil)
	if res.Restored != 1 || len(res.Skipped) != 1 {
		t.Fatalf("only the original, checksummed file may be written: %+v", res)
	}
}

func TestDiffShowsWhatARestoreWouldDo(t *testing.T) {
	src, dst := roots(t), roots(t)
	write(t, filepath.Join(src["home"], ".gitconfig"), "new")
	write(t, filepath.Join(src["home"], ".gitignore_global"), "same")
	write(t, filepath.Join(src["home"], ".config/git/config"), "brand new")
	z, _ := makeBackup(t, settings.Options{IDs: []string{"git"}, Roots: src})
	write(t, filepath.Join(dst["home"], ".gitconfig"), "old")
	write(t, filepath.Join(dst["home"], ".gitignore_global"), "same")

	changes, err := settings.Diff(bytes.NewReader(z), int64(len(z)), []string{"git"}, dst)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]string{}
	for _, c := range changes {
		state[c.File] = c.State
	}
	if state["home/.gitconfig"] != "changed" || state["home/.gitignore_global"] != "same" || state["home/.config/git/config"] != "new" {
		t.Fatalf("%v", state)
	}
	if b, _ := os.ReadFile(filepath.Join(dst["home"], ".gitconfig")); string(b) != "old" {
		t.Fatal("a diff must not write anything")
	}
}

func TestVersionedFoldersAreExpandedAndMatched(t *testing.T) {
	src, dst := roots(t), roots(t)
	write(t, filepath.Join(src["appdata"], "JetBrains/IntelliJIdea2025.1/options/ide.general.xml"), "<a/>")
	write(t, filepath.Join(src["appdata"], "JetBrains/PyCharm2025.1/keymaps/mine.xml"), "<k/>")
	write(t, filepath.Join(src["appdata"], "JetBrains/IntelliJIdea2025.1/eval/evaluation.key"), "license-ish")
	write(t, filepath.Join(src["appdata"], "Microsoft/PowerToys/FancyZones/settings.json"), "{}")
	write(t, filepath.Join(src["appdata"], "Microsoft/PowerToys/FancyZones/logs.txt"), "no")
	// PowerToys is under localappdata, JetBrains under appdata.
	write(t, filepath.Join(src["localappdata"], "Microsoft/PowerToys/FancyZones/settings.json"), "{}")
	write(t, filepath.Join(src["localappdata"], "Microsoft/PowerToys/FancyZones/logs.txt"), "no")

	z, m := makeBackup(t, settings.Options{IDs: []string{"jetbrains", "powertoys"}, Roots: src})
	files := map[string]int{}
	for _, s := range m.Sets {
		files[s.ID] = s.Files
	}
	if files["jetbrains"] != 2 || files["powertoys"] != 1 {
		t.Fatalf("only matching files count (no eval keys, no logs): %v", files)
	}
	res, err := settings.Restore(bytes.NewReader(z), int64(len(z)), []string{"jetbrains", "powertoys"}, dst, nil)
	if err != nil || res.Restored != 3 {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := os.Stat(filepath.Join(dst["appdata"], "JetBrains/IntelliJIdea2025.1/eval/evaluation.key")); err == nil {
		t.Fatal("a file outside the listed folders was restored")
	}
}

func TestPatternsDoNotLetAnEntryEscapeTheirSegment(t *testing.T) {
	dst := roots(t)
	z := evil(t,
		"jetbrains/appdata/JetBrains/X/options/../../../../escape.txt",
		"jetbrains/appdata/JetBrains/options/a.xml",
		"jetbrains/appdata/JetBrains/X/Y/options/a.xml",
		"jetbrains/appdata/JetBrains/X/options/ok.xml",
	)
	res, err := settings.Restore(bytes.NewReader(z), int64(len(z)), []string{"jetbrains"}, dst, nil)
	if err != nil || res.Restored != 1 || len(res.Skipped) != 3 {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestFolderRestoreRefusesEscapingNames(t *testing.T) {
	src := roots(t)
	docs := t.TempDir()
	write(t, filepath.Join(docs, "ok.txt"), "ok")
	z, m := makeBackup(t, settings.Options{Roots: src, Folders: []string{docs}})
	bad := rewrite(t, z, "folders/1/../../escape.txt", "x")
	dest := t.TempDir()
	res, err := settings.RestoreFolders(bytes.NewReader(bad), int64(len(bad)), dest, []string{m.Folders[0].Name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escape.txt")); err == nil {
		t.Fatal("escaped the destination")
	}
	if res.Written != 1 {
		t.Fatalf("%+v", res)
	}
}

func TestEmptyBackupAndBadFolder(t *testing.T) {
	if _, err := settings.Create(&bytes.Buffer{}, settings.Options{Roots: roots(t)}); err == nil {
		t.Error("nothing to back up must be an error")
	}
	if _, err := settings.Create(&bytes.Buffer{}, settings.Options{Roots: roots(t), Folders: []string{filepath.Join(t.TempDir(), "nope")}}); err == nil {
		t.Error("a folder that does not exist must be an error")
	}
}

func TestSetListNeverReachesForSecrets(t *testing.T) {
	deny := regexp.MustCompile(`(?i)(token|passw|credential|cookie|secret|login|hosts\.yml|id_(rsa|ed25519|ecdsa|dsa)|\.pem|\.pfx|known_hosts|authorized_keys|\.kdbx|\.key\b|kube|\.npmrc|\.netrc|stream)`)
	for _, s := range settings.Sets {
		for _, p := range s.Paths {
			if deny.MatchString(p.Rel) {
				t.Errorf("%s: %q looks like it holds a secret", s.ID, p.Rel)
			}
			if strings.Contains(p.Rel, "..") || strings.Contains(p.Rel, `\`) || strings.HasPrefix(p.Rel, "/") {
				t.Errorf("%s: bad path %q", s.ID, p.Rel)
			}
		}
	}
	if len(settings.Sets) < 25 {
		t.Errorf("the list shrank to %d sets", len(settings.Sets))
	}
}

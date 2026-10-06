package settings_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/settings"
)

type fakeRun struct{ calls []string }

func (f *fakeRun) Run(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if len(args) > 0 && args[0] == "--list-extensions" {
		return []byte("ms-python.python\r\nrust-lang.rust-analyzer\nnot an id; calc.exe\n"), nil
	}
	return nil, nil
}

func roots(t *testing.T) settings.Roots {
	d := t.TempDir()
	r := settings.Roots{}
	for _, k := range []string{"appdata", "localappdata", "home", "documents"} {
		r[k] = filepath.Join(d, k)
		_ = os.MkdirAll(r[k], 0o755)
	}
	return r
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBackupAndRestoreToAnotherPC(t *testing.T) {
	src, dst := roots(t), roots(t)
	write(t, filepath.Join(src["appdata"], "Code/User/settings.json"), `{"editor.fontSize": 15}`)
	write(t, filepath.Join(src["appdata"], "Code/User/snippets/go.json"), `{}`)
	write(t, filepath.Join(src["home"], ".gitconfig"), "[user]\n name = Sebas\n")
	write(t, filepath.Join(src["home"], ".ssh/id_ed25519"), "PRIVATE KEY")
	write(t, filepath.Join(src["home"], ".ssh/config"), "Host *\n")

	found := settings.Detect(src, &fakeRun{})
	ids := map[string]bool{}
	for _, f := range found {
		ids[f.ID] = true
	}
	if !ids["vscode"] || !ids["git"] || !ids["ssh"] || ids["terminal"] {
		t.Fatalf("detected: %+v", found)
	}

	var buf bytes.Buffer
	if _, err := settings.Backup(&buf, []string{"vscode", "git", "ssh"}, src, &fakeRun{}); err != nil {
		t.Fatal(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	for _, f := range zr.File {
		if strings.Contains(f.Name, "id_ed25519") {
			t.Fatal("a private key must never be in a backup")
		}
	}

	m, err := settings.ReadManifest(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil || len(m.Sets) != 3 {
		t.Fatalf("%v %+v", err, m)
	}

	run := &fakeRun{}
	res, err := settings.Restore(bytes.NewReader(buf.Bytes()), int64(buf.Len()), []string{"vscode", "git"}, dst, run)
	if err != nil {
		t.Fatal(err)
	}
	if res.Restored != 3 || res.Extensions != 2 {
		t.Fatalf("%+v", res)
	}
	b, _ := os.ReadFile(filepath.Join(dst["appdata"], "Code/User/settings.json"))
	if string(b) != `{"editor.fontSize": 15}` {
		t.Fatalf("settings not restored: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dst["home"], ".ssh/config")); err == nil {
		t.Fatal("ssh was not selected, so it must not be restored")
	}
	for _, c := range run.calls {
		if strings.Contains(c, "calc") || strings.Contains(c, ";") {
			t.Fatalf("only publisher.name ids may reach a command: %q", c)
		}
	}
	if len(run.calls) != 2 || !strings.HasPrefix(run.calls[0], "code --install-extension ms-python.python --force") {
		t.Fatalf("calls: %v", run.calls)
	}
}

func TestRestoreKeepsWhatItReplaces(t *testing.T) {
	src, dst := roots(t), roots(t)
	write(t, filepath.Join(src["home"], ".gitconfig"), "new")
	var buf bytes.Buffer
	if _, err := settings.Backup(&buf, []string{"git"}, src, nil); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dst["home"], ".gitconfig")
	write(t, target, "mine")
	res, err := settings.Restore(bytes.NewReader(buf.Bytes()), int64(buf.Len()), []string{"git"}, dst, nil)
	if err != nil || res.BackedUp != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	old, _ := os.ReadFile(target + settings.BackupSuffix)
	cur, _ := os.ReadFile(target)
	if string(old) != "mine" || string(cur) != "new" {
		t.Fatalf("old=%q cur=%q", old, cur)
	}
	// Restoring the same thing again changes nothing.
	res, _ = settings.Restore(bytes.NewReader(buf.Bytes()), int64(buf.Len()), []string{"git"}, dst, nil)
	if res.Restored != 0 || res.Unchanged != 1 {
		t.Fatalf("%+v", res)
	}
}

func evil(t *testing.T, names ...string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, _ := zw.Create(n)
		_, _ = w.Write([]byte("pwned"))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func TestRestoreCannotWriteOutsideTheListedFiles(t *testing.T) {
	dst := roots(t)
	z := evil(t,
		"git/home/../../escape.txt",
		"git/home/.gitconfig/../../../x",
		"git/home/.ssh/authorized_keys",
		"git/home/Documents/evil.ps1",
		"git/home/C:/Windows/evil",
		"git/home/.bashrc",
		"git/nowhere/.gitconfig",
		"git/appdata/Code/User/settings.json",
		"vscode/appdata/Code/User/snippets/../../../../evil",
		"vscode/appdata/Code/User/snippets/ok.json",
	)
	res, err := settings.Restore(bytes.NewReader(z), int64(len(z)), []string{"git", "vscode"}, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Restored != 1 || len(res.Skipped) != 9 {
		t.Fatalf("only the one listed snippet may be written: %+v", res)
	}
	var wrote []string
	_ = filepath.WalkDir(filepath.Dir(dst["home"]), func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			wrote = append(wrote, filepath.ToSlash(strings.TrimPrefix(p, filepath.Dir(dst["home"]))))
		}
		return nil
	})
	if len(wrote) != 1 || !strings.HasSuffix(wrote[0], "snippets/ok.json") {
		t.Fatalf("files written: %v", wrote)
	}
}

func TestNotABackup(t *testing.T) {
	for name, b := range map[string][]byte{"text": []byte("hello"), "zip without manifest": evil(t, "git/home/.gitconfig")} {
		if _, err := settings.ReadManifest(bytes.NewReader(b), int64(len(b))); err == nil {
			t.Errorf("%s should be refused", name)
		}
	}
	if _, err := settings.Backup(&bytes.Buffer{}, []string{"nope"}, roots(t), nil); err == nil {
		t.Error("unknown set")
	}
	if _, err := settings.Backup(&bytes.Buffer{}, []string{"git"}, roots(t), nil); err == nil {
		t.Error("an empty backup is an error, not an empty file")
	}
}

func TestEverySetPathStaysRelativeAndForwardSlashed(t *testing.T) {
	for _, s := range settings.Sets {
		for _, p := range s.Paths {
			if strings.Contains(p.Rel, `\`) || strings.HasPrefix(p.Rel, "/") || strings.Contains(p.Rel, "..") {
				t.Errorf("%s: bad path %q", s.ID, p.Rel)
			}
			if strings.Contains(strings.ToLower(p.Rel), "id_") || strings.Contains(strings.ToLower(p.Rel), "token") {
				t.Errorf("%s: %q looks like a secret", s.ID, p.Rel)
			}
		}
	}
}

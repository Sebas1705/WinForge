package sysapps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/compare"
	"github.com/Sebas1705/WinForge/internal/profile"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	must(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func argsPath(p string) string {
	b, _ := json.Marshal(map[string]string{"path": p})
	return string(b)
}

const otherProfile = `{"winforge":1,"profile":{"id":"x","name":"Other PC","apps":[{"id":"alpha","version":"1.0"},{"id":"gamma","version":"2.0"},{"id":"delta"},{"id":"ghost"}],"recipes":["nope"],"extends":["not-a-profile"]}}`

func TestCompareFile(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0", "gamma", "3.0", "beta", "5")
	p := writeFile(t, r.root, "other.json", otherProfile)

	m := r.ok("winforge_compare_profile", argsPath(p))
	if r.snapshots() != 1 {
		t.Fatalf("snapshots %d", r.snapshots())
	}
	if str(m, "source") != "file" || str(m, "file") != "other.json" || str(m, "profileName") != "Other PC" {
		t.Fatalf("header %v", m)
	}
	if got := ids(list(m, "both"), "id"); len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("both %v", got)
	}
	d := list(m, "differ")
	if len(d) != 1 || str(obj(d[0]), "id") != "gamma" || str(obj(d[0]), "thisPc") != "3.0" || str(obj(d[0]), "profile") != "2.0" {
		t.Fatalf("differ %v", d)
	}
	if got := ids(list(m, "onlyProfile"), "id"); len(got) != 1 || got[0] != "delta" {
		t.Fatalf("onlyProfile %v", got)
	}
	if got := ids(list(m, "onlyThisPc"), "id"); len(got) != 1 || got[0] != "beta" {
		t.Fatalf("onlyThisPc %v", got)
	}
	if u := list(m, "unknown"); len(u) != 1 || str(obj(u[0]), "id") != "ghost" || str(obj(u[0]), "in") != "profile" {
		t.Fatalf("unknown %v", u)
	}
	if got := list(m, "unknownRecipes"); len(got) != 1 || got[0] != "nope" {
		t.Fatalf("unknownRecipes %v", got)
	}
	if got := list(m, "unknownProfiles"); len(got) != 1 {
		t.Fatalf("unknownProfiles %v", got)
	}
	if strings.Contains(strings.ToLower(mustText(m)), strings.ToLower(r.root)) {
		t.Fatal("the answer leaks the folder of the file")
	}
	if len(r.exec.calls) != 0 || r.plans.count() != 0 {
		t.Fatal("compare ran something")
	}
}

func mustText(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func TestCompareCodeAndLimits(t *testing.T) {
	r := newRig(t)
	for i := 1; i <= 120; i++ {
		r.install(idN(i), "1")
	}
	var apps []catalog.ProfileApp
	for i := 100; i <= 250; i++ {
		apps = append(apps, catalog.ProfileApp{ID: idN(i)})
	}
	code, err := profile.EncodeCode(catalog.Profile{ID: "big", Name: "Big", Kind: "custom", Apps: apps})
	must(t, err)
	b, _ := json.Marshal(map[string]any{"code": code, "limit": 5})
	m := r.ok("winforge_compare_profile", string(b))
	if str(m, "source") != "code" || !boolean(m, "truncated") || len(list(m, "onlyThisPc")) != 5 || len(list(m, "onlyProfile")) != 5 {
		t.Fatalf("limits %v", m["counts"])
	}
	c := obj(m["counts"])
	if num(c, "onlyThisPc") != 99 || num(c, "onlyProfile") != 130 || num(c, "both") != 21 {
		t.Fatalf("counts %v", c)
	}

	for _, bad := range []string{"hello", "WF1.!!!", "WF1." + strings.Repeat("A", 40)} {
		b, _ := json.Marshal(map[string]string{"code": bad})
		r.fails("winforge_compare_profile", string(b), "could not be compared")
	}
}

func idN(i int) string { return "bulk-" + pad3(i) }

func pad3(i int) string {
	s := "000" + itoa(i)
	return s[len(s)-3:]
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var d []byte
	for ; i > 0; i /= 10 {
		d = append([]byte{byte('0' + i%10)}, d...)
	}
	return string(d)
}

func TestCompareRefusesHostileFiles(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0")
	dir := filepath.Join(r.root, "folder.json")
	must(t, os.Mkdir(dir, 0o755))
	big := writeFile(t, r.root, "big.json", `{"winforge":1,"profile":{"id":"x","name":"X","apps":[]}}`+strings.Repeat(" ", compare.MaxFile))

	cases := []struct{ name, path, want string }{
		{"empty", "", "exactly one"},
		{"relative", "other.json", "absolute"},
		{"relative dotdot", `..\..\other.json`, "absolute"},
		{"unc", `\\server\share\other.json`, "UNC"},
		{"unc slashes", `//server/share/other.json`, "UNC"},
		{"device namespace", `\\.\PhysicalDrive0`, "UNC"},
		{"extended path", `\\?\C:\other.json`, "UNC"},
		{"not json extension", writeFile(t, r.root, "other.txt", otherProfile), ".json"},
		{"no extension", writeFile(t, r.root, "other", otherProfile), ".json"},
		{"device name", filepath.Join(r.root, "NUL.json"), "device"},
		{"device name com", filepath.Join(r.root, "com1.json"), "device"},
		{"stream", filepath.Join(r.root, "other.json:evil"), "stream"},
		{"control char", filepath.Join(r.root, "o\x00ther.json"), "control"},
		{"directory", dir, "regular file"},
		{"missing", filepath.Join(r.root, "missing.json"), "unreadable"},
		{"too big", big, "too-big"},
		{"too long", filepath.Join(r.root, strings.Repeat("a", 1100)+".json"), "longer"},
		{"garbage", writeFile(t, r.root, "garbage.json", "\x00\x01\x02 not json"), "not-json"},
		{"trailing document", writeFile(t, r.root, "two.json", otherProfile+otherProfile), "not-json"},
		{"not a profile", writeFile(t, r.root, "np.json", `{"hello":"world"}`), "not-profile"},
		{"newer format", writeFile(t, r.root, "new.json", `{"winforge":99,"profile":{"id":"x","name":"X"}}`), "newer-format"},
		{"wrong types", writeFile(t, r.root, "types.json", `{"winforge":1,"profile":{"id":"x","name":"X","apps":"alpha"}}`), "not-json"},
		{"array root", writeFile(t, r.root, "arr.json", `[1,2,3]`), "not-json"},
		{"empty file", writeFile(t, r.root, "empty.json", ``), "not-json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := r.snapshots()
			res := r.call("winforge_compare_profile", argsPath(c.path))
			if !res.IsError {
				t.Fatalf("expected an error, got %s", res.Text)
			}
			if !strings.Contains(res.Text, c.want) {
				t.Fatalf("error %q does not mention %q", res.Text, c.want)
			}
			if strings.Contains(res.Text, r.root) {
				t.Fatalf("error leaks the path: %s", res.Text)
			}
			if r.snapshots() != before {
				t.Fatal("a refused file must not cause a scan of the PC")
			}
		})
	}
}

func TestCompareRefusesLinks(t *testing.T) {
	r := newRig(t)
	target := writeFile(t, r.root, "real.json", otherProfile)
	link := filepath.Join(r.root, "link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symbolic link here: %v", err)
	}
	r.fails("winforge_compare_profile", argsPath(link), "regular file")
}

func TestCompareHostileContentIsBounded(t *testing.T) {
	r := newRig(t)
	// a file listing thousands of unknown ids, a huge name and control characters must stay small and clean
	var sb strings.Builder
	sb.WriteString(`{"winforge":1,"profile":{"id":"x","name":"` + strings.Repeat("N", 5000) + "\\u0007\\u202ebad" + `","apps":[`)
	for i := 0; i < 3000; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"id":"unknown-` + itoa(i) + `-` + strings.Repeat("z", 300) + `"}`)
	}
	sb.WriteString(`]}}`)
	p := writeFile(t, r.root, "hostile.json", sb.String())
	res := r.call("winforge_compare_profile", argsPath(p))
	if res.IsError {
		t.Fatalf("a large but valid file should compare: %s", res.Text)
	}
	if len(res.Text) > 64*1024 {
		t.Fatalf("answer is %d bytes", len(res.Text))
	}
	m := decodeText(t, res)
	if len(list(m, "unknown")) != defaultLimit || !boolean(m, "truncated") || num(obj(m["counts"]), "unknown") != 3000 {
		t.Fatalf("unknown ids must be capped: %d", len(list(m, "unknown")))
	}
	for _, u := range list(m, "unknown") {
		if len([]rune(str(obj(u), "id"))) > 81 {
			t.Fatal("unknown id not bounded")
		}
	}
	if len([]rune(str(m, "profileName"))) > 200 || strings.ContainsRune(str(m, "profileName"), '\u0007') || strings.ContainsRune(str(m, "profileName"), '\u202e') {
		t.Fatal("profile name not cleaned")
	}

	// more apps than the file limit
	var many strings.Builder
	many.WriteString(`{"winforge":1,"profile":{"id":"x","name":"X","apps":[`)
	for i := 0; i < compare.MaxApps+1; i++ {
		if i > 0 {
			many.WriteString(",")
		}
		many.WriteString(`{"id":"a"}`)
	}
	many.WriteString(`]}}`)
	r.fails("winforge_compare_profile", argsPath(writeFile(t, r.root, "many.json", many.String())), "too-many")
}

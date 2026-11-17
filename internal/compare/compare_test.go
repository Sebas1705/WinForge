package compare

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/profile"
)

func testCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		Apps: map[string]*catalog.App{
			"git":  {ID: "git", Name: "Git"},
			"vlc":  {ID: "vlc", Name: "VLC"},
			"7zip": {ID: "7zip", Name: "7-Zip"},
			"node": {ID: "node", Name: "Node", Requires: []string{"git"}},
		},
		Profiles: map[string]*catalog.Profile{"base": {ID: "base", Name: "Base", Apps: []catalog.ProfileApp{{ID: "vlc"}}}},
		Recipes:  map[string]*catalog.Recipe{"tweak": {ID: "tweak"}},
	}
}

func side(pairs ...string) Side {
	s := Side{Versions: map[string]string{}}
	for i := 0; i+1 < len(pairs); i += 2 {
		s.Versions[pairs[i]] = pairs[i+1]
	}
	return s
}

func TestCompareTable(t *testing.T) {
	cases := []struct {
		name       string
		a, b       Side
		onlyA, onB []string
		both       []string
		differ     []Differ
	}{
		{"both empty", side(), side(), nil, nil, nil, nil},
		{"empty a", side(), side("git", ""), nil, []string{"git"}, nil, nil},
		{"empty b", side("git", "1"), side(), []string{"git"}, nil, nil, nil},
		{"same versions", side("git", "1"), side("git", "1"), nil, nil, []string{"git"}, nil},
		{"different versions", side("git", "1"), side("git", "2"), nil, nil, nil, []Differ{{"git", "1", "2"}}},
		{"version missing on one side", side("git", "1"), side("git", ""), nil, nil, []string{"git"}, nil},
		{"version missing on both", side("git", ""), side("git", ""), nil, nil, []string{"git"}, nil},
		{"mixed", side("git", "1", "vlc", "3", "7zip", ""), side("git", "1", "vlc", "4", "node", ""), []string{"7zip"}, []string{"node"}, []string{"git"}, []Differ{{"vlc", "3", "4"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Compare(c.a, c.b)
			check := func(got, want []string, what string) {
				if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
					t.Errorf("%s: got %v want %v", what, got, want)
				}
				if got == nil {
					t.Errorf("%s is nil: it must be an empty list", what)
				}
			}
			check(r.OnlyA, c.onlyA, "onlyA")
			check(r.OnlyB, c.onB, "onlyB")
			check(r.Both, c.both, "both")
			if len(r.Differ) != len(c.differ) || (len(c.differ) > 0 && !reflect.DeepEqual(r.Differ, c.differ)) || r.Differ == nil {
				t.Errorf("differ: %+v want %+v", r.Differ, c.differ)
			}
		})
	}
}

func TestCompareSortsByID(t *testing.T) {
	r := Compare(side("z", "", "a", "", "m", ""), side())
	if !reflect.DeepEqual(r.OnlyA, []string{"a", "m", "z"}) {
		t.Fatalf("%v", r.OnlyA)
	}
}

func TestUnknownIDsSaySideAndMerge(t *testing.T) {
	a := Side{Versions: map[string]string{}, Unknown: []string{"ghost", "shared"}}
	b := Side{Versions: map[string]string{}, Unknown: []string{"shared", "other"}}
	r := Compare(a, b)
	want := []UnknownID{{"ghost", "a"}, {"other", "b"}, {"shared", "both"}}
	if !reflect.DeepEqual(r.Unknown, want) {
		t.Fatalf("%v", r.Unknown)
	}
}

func TestPCSideLowercasesAndTrims(t *testing.T) {
	s := PC(map[string]string{"Git": " 2.1 ", "vlc": ""})
	if s.Versions["git"] != "2.1" || !s.Versioned {
		t.Fatalf("%+v", s)
	}
	if PC(map[string]string{"vlc": ""}).Versioned {
		t.Fatal("no version anywhere means not versioned")
	}
}

func TestParseExportedFile(t *testing.T) {
	cat := testCatalog()
	exp, _ := profile.Export(catalog.Profile{ID: "mine", Name: "My PC", Apps: []catalog.ProfileApp{{ID: "git", Version: "2.1"}, {ID: "vlc"}}, Recipes: []string{"tweak"}})
	s, err := ParseFile(exp, cat)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "My PC" || s.Versions["git"] != "2.1" || s.Versions["vlc"] != "" || !s.Versioned {
		t.Fatalf("%+v", s)
	}
	if s.Profile == nil || len(s.Profile.Apps) != 2 || len(s.Profile.Recipes) != 1 {
		t.Fatalf("%+v", s.Profile)
	}
}

func TestParseFileToleratesWhatImportWouldNot(t *testing.T) {
	cat := testCatalog()
	doc := `{"winforge":1,"future":{"x":[1,2]},"profile":{"id":"A B","name":"Mine","apps":[{"id":"GIT","version":"1","extra":true},{"id":" git "},{"id":"vlc"}]}}`
	s, err := ParseFile([]byte(doc), cat)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Versions) != 2 || s.Versions["git"] != "1" {
		t.Fatalf("case and duplicates: %+v", s.Versions)
	}
	if len(s.Profile.Apps) != 2 {
		t.Fatalf("%+v", s.Profile.Apps)
	}
}

func TestParseFileDuplicateTakesTheFirstVersionThatExists(t *testing.T) {
	s, err := ParseFile([]byte(`{"winforge":1,"profile":{"apps":[{"id":"git"},{"id":"git","version":"3"},{"id":"git","version":"4"}]}}`), testCatalog())
	if err != nil || s.Versions["git"] != "3" || s.Profile.Apps[0].Version != "3" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestParseFileReportsUnknownThings(t *testing.T) {
	doc := `{"winforge":1,"profile":{"name":"x","extends":["base","nope"],"apps":[{"id":"ghost"},{"id":"GHOST"},{"id":"git"}],"recipes":["tweak","evil"]}}`
	s, err := ParseFile([]byte(doc), testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Unknown, []string{"ghost"}) || !reflect.DeepEqual(s.UnknownRecipes, []string{"evil"}) || !reflect.DeepEqual(s.UnknownProfiles, []string{"nope"}) {
		t.Fatalf("%+v", s)
	}
	if _, ok := s.Versions["vlc"]; !ok {
		t.Fatal("a catalog profile it extends contributes its apps")
	}
	if !reflect.DeepEqual(s.Profile.Extends, []string{"base"}) || !reflect.DeepEqual(s.Profile.Recipes, []string{"tweak"}) {
		t.Fatalf("%+v", s.Profile)
	}
}

func TestParseFileDoesNotPullInRequiredApps(t *testing.T) {
	s, _ := ParseFile([]byte(`{"winforge":1,"profile":{"apps":[{"id":"node"}]}}`), testCatalog())
	if _, ok := s.Versions["git"]; ok {
		t.Fatal("what the file lists is what is compared, not what its apps need")
	}
}

func TestParseFileDropsBadVersionsAndText(t *testing.T) {
	doc := "{\"winforge\":1,\"profile\":{\"name\":\"a\\u0000b\\nc\",\"apps\":[{\"id\":\"git\",\"version\":\"1; rm -rf /\"},{\"id\":\"vlc\",\"version\":\"3.0.1-beta+x\"}]}}"
	s, err := ParseFile([]byte(doc), testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if s.Versions["git"] != "" || s.Versions["vlc"] != "3.0.1-beta+x" || s.Name != "abc" {
		t.Fatalf("%+v", s)
	}
}

func code(t *testing.T, doc string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(doc))
	zw.Close()
	return profile.CodePrefix + base64.RawURLEncoding.EncodeToString(buf.Bytes())
}

func errCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "other:" + fmt.Sprint(err)
}

func TestParseCode(t *testing.T) {
	cat := testCatalog()
	c, err := profile.EncodeCode(catalog.Profile{ID: "mine", Name: "Mine", Apps: []catalog.ProfileApp{{ID: "git", Version: "9"}}})
	if err != nil {
		t.Fatal(err)
	}
	wrapped := c[:20] + "\n  " + c[20:] + "\r\n"
	s, err := ParseCode(wrapped, cat)
	if err != nil || s.Versions["git"] != "9" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestParseCodeRejects(t *testing.T) {
	cat := testCatalog()
	bomb := code(t, `{"winforge":1,"profile":{"name":"`+strings.Repeat("a", MaxFile)+`"}}`)
	cases := map[string]string{
		"":                                    "not-code",
		"hello":                               "not-code",
		"WF1.***":                             "cut-off",
		"WF1.AAAA":                            "cut-off",
		code(t, "not json"):                   "not-json",
		code(t, `{"winforge":1}`):             "not-profile",
		bomb:                                  "too-big",
		"WF1." + strings.Repeat("A", MaxFile): "too-big",
	}
	for in, want := range cases {
		if _, err := ParseCode(in, cat); errCode(err) != want {
			t.Errorf("%.30q: got %v want %s", in, err, want)
		}
	}
}

func TestHostileFiles(t *testing.T) {
	cat := testCatalog()
	deep := strings.Repeat("[", 20000) + strings.Repeat("]", 20000)
	many := `{"winforge":1,"profile":{"apps":[` + strings.Repeat(`{"id":"git"},`, MaxApps) + `{"id":"vlc"}]}}`
	cases := []struct{ name, in, want string }{
		{"empty", ``, "not-json"},
		{"whitespace", "  \n", "not-json"},
		{"just null", `null`, "not-profile"},
		{"array", `[]`, "not-json"},
		{"string", `"x"`, "not-json"},
		{"number", `12`, "not-json"},
		{"no version", `{"profile":{}}`, "not-profile"},
		{"zero version", `{"winforge":0,"profile":{}}`, "not-profile"},
		{"no profile", `{"winforge":1}`, "not-profile"},
		{"profile null", `{"winforge":1,"profile":null}`, "not-profile"},
		{"newer", `{"winforge":2,"profile":{}}`, "newer-format"},
		{"version string", `{"winforge":"1","profile":{}}`, "not-json"},
		{"fraction", `{"winforge":1.5,"profile":{}}`, "not-json"},
		{"apps wrong type", `{"winforge":1,"profile":{"apps":"git"}}`, "not-json"},
		{"app is string", `{"winforge":1,"profile":{"apps":["git"]}}`, "not-json"},
		{"id is number", `{"winforge":1,"profile":{"apps":[{"id":5}]}}`, "not-json"},
		{"version is object", `{"winforge":1,"profile":{"apps":[{"id":"git","version":{}}]}}`, "not-json"},
		{"trailing garbage", `{"winforge":1,"profile":{}} garbage`, "not-json"},
		{"two documents", `{"winforge":1,"profile":{}}{"winforge":1,"profile":{}}`, "not-json"},
		{"cut off", `{"winforge":1,"profile":{"apps":[{"id":"gi`, "not-json"},
		{"too deep", deep, "not-json"},
		{"deep in a field", `{"winforge":1,"profile":{"name":"a"},"x":` + deep + `}`, "not-json"},
		{"too many apps", many, "too-many"},
		{"too big", `{"winforge":1,"profile":{"description":"` + strings.Repeat("a", MaxFile) + `"}}`, "too-big"},
		{"binary", "\x00\x01\x02\xff\xfe", "not-json"},
		{"utf16 bom", "\xff\xfe{\x00", "not-json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseFile([]byte(c.in), cat)
			if errCode(err) != c.want {
				t.Fatalf("got %v want %s", err, c.want)
			}
		})
	}
}

func TestAByteOrderMarkIsFine(t *testing.T) {
	s, err := ParseFile([]byte("\xef\xbb\xbf{\"winforge\":1,\"profile\":{\"apps\":[{\"id\":\"git\"}]}}"), testCatalog())
	if err != nil || len(s.Versions) != 1 {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestHostileIDsAreBoundedAndNeverRun(t *testing.T) {
	long := strings.Repeat("x", 5000)
	doc := fmt.Sprintf(`{"winforge":1,"profile":{"name":"$(calc)","apps":[{"id":"%s"},{"id":"a\u0000b"},{"id":"<script>alert(1)</script>"},{"id":"..\\..\\evil"},{"id":"\ud800"}]}}`, long)
	s, err := ParseFile([]byte(doc), testCatalog())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range s.Unknown {
		if len([]rune(id)) > maxID+1 || strings.ContainsAny(id, "\x00\n") {
			t.Fatalf("%q", id)
		}
	}
	if len(s.Versions) != 0 || len(s.Profile.Apps) != 0 {
		t.Fatalf("nothing unknown may become an app: %+v", s)
	}
}

// A fuzz-like pass: random cuts, flips and splices of a real file must never panic, and whatever is
// accepted must only name catalog apps.
func TestRandomDamageNeverPanics(t *testing.T) {
	cat := testCatalog()
	good, _ := profile.Export(catalog.Profile{ID: "mine", Name: "Mine", Apps: []catalog.ProfileApp{{ID: "git", Version: "2"}, {ID: "vlc"}, {ID: "ghost"}}, Recipes: []string{"tweak"}})
	rng := rand.New(rand.NewSource(18))
	for i := 0; i < 3000; i++ {
		b := append([]byte(nil), good...)
		switch rng.Intn(4) {
		case 0:
			b = b[:rng.Intn(len(b)+1)]
		case 1:
			for n := rng.Intn(6) + 1; n > 0; n-- {
				b[rng.Intn(len(b))] = byte(rng.Intn(256))
			}
		case 2:
			at := rng.Intn(len(b))
			b = append(b[:at], append([]byte(`"{[`), b[at:]...)...)
		case 3:
			at := rng.Intn(len(b))
			b = append(b[:at], b[at+rng.Intn(len(b)-at):]...)
		}
		s, err := ParseFile(b, cat)
		if err != nil {
			if errCode(err)[:5] == "other" {
				t.Fatalf("an error without a code: %v", err)
			}
			continue
		}
		for id := range s.Versions {
			if cat.Apps[id] == nil {
				t.Fatalf("%q is not a catalog app", id)
			}
		}
		_ = Compare(PC(map[string]string{"git": "1"}), s)
	}
}

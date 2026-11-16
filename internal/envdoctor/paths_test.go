package envdoctor

import (
	"reflect"
	"strings"
	"testing"
)

func TestSplitPathRoundTrips(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"a", []string{"a"}},
		{"a;b", []string{"a", "b"}},
		{"a;b;", []string{"a", "b", ""}},
		{";a", []string{"", "a"}},
		{"a;;b", []string{"a", "", "b"}},
		{";", []string{"", ""}},
		{`"C:\Program Files;x";D:\b`, []string{`"C:\Program Files;x"`, `D:\b`}},
		{`C:\a;"C:\b c";%USERPROFILE%\bin`, []string{`C:\a`, `"C:\b c"`, `%USERPROFILE%\bin`}},
		{`"unbalanced;a;b`, []string{`"unbalanced;a;b`}},
	}
	for _, c := range cases {
		got := splitPath(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitPath(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := strings.Join(splitPath(c.in), ";"); got != c.in {
			t.Errorf("join(split(%q)) = %q", c.in, got)
		}
	}
}

func TestNormDir(t *testing.T) {
	cases := map[string]string{
		`C:\Tools`:           `C:\Tools`,
		`C:\Tools\`:          `C:\Tools`,
		`C:\Tools\\`:         `C:\Tools`,
		`C:/Tools/bin/`:      `C:\Tools\bin`,
		`"C:\Program Files"`: `C:\Program Files`,
		`  C:\Tools  `:       `C:\Tools`,
		`C:\a\.\b\..\c`:      `C:\a\c`,
		`C:\`:                `C:\`,
		`C:\..`:              `C:\`,
		`c:\tools`:           `c:\tools`,
		`\\srv\share\bin\`:   `\\srv\share\bin`,
		`//srv/share`:        `\\srv\share`,
		`bin`:                `bin`,
		`.`:                  `.`,
		`C:foo`:              `C:foo`,
	}
	for in, want := range cases {
		if got := normDir(in); got != want {
			t.Errorf("normDir(%q) = %q, want %q", in, got, want)
		}
	}
	if dirKey(normDir(`C:\Tools\`)) != dirKey(normDir(`c:/TOOLS`)) {
		t.Error("spellings of one folder share a key")
	}
	if dirKey(normDir(`C:\`)) == dirKey(normDir(`C:`)) {
		t.Error("a drive root and a drive-relative path are different")
	}
}

func TestIsAbs(t *testing.T) {
	for _, s := range []string{`C:\a`, `c:/a`, `\\srv\x`, `//srv/x`, `D:\`} {
		if !isAbs(s) {
			t.Errorf("%q is absolute", s)
		}
	}
	for _, s := range []string{``, `a`, `.\a`, `..\a`, `C:a`, `C:`, `\a`, `%X%\a`} {
		if isAbs(s) {
			t.Errorf("%q is not absolute", s)
		}
	}
}

func TestExpand(t *testing.T) {
	vars := map[string]string{"A": `C:\a`, "B": "bee"}
	look := func(n string) (string, bool) { v, ok := vars[strings.ToUpper(n)]; return v, ok }
	cases := map[string]string{
		`%A%\bin`:      `C:\a\bin`,
		`%a%\%b%`:      `C:\a\bee`,
		`%NOPE%\x`:     `%NOPE%\x`,
		`100%`:         `100%`,
		`%A%%B%`:       `C:\abee`,
		`plain`:        `plain`,
		`%A%\%NOPE%\z`: `C:\a\%NOPE%\z`,
	}
	for in, want := range cases {
		if got := expand(in, look); got != want {
			t.Errorf("expand(%q) = %q, want %q", in, got, want)
		}
	}
	if !unresolved(`%NOPE%\x`) || unresolved(`C:\x`) || unresolved(`100%`) {
		t.Error("unresolved spots a leftover reference only")
	}
}

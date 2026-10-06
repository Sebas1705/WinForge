package profile_test

import (
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/profile"
)

func TestShareCodeRoundTripsAndIsOneLine(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	p := catalog.Profile{ID: "mine", Name: "Mi PC ñ", Kind: "custom", Apps: []catalog.ProfileApp{{ID: "git"}, {ID: "vlc"}}}
	code, err := profile.EncodeCode(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(code, "WF1.") || strings.ContainsAny(code, " \n+/=") {
		t.Fatalf("code must be a single URL-safe token: %q", code)
	}
	// Chat apps wrap and pad what is pasted.
	wrapped := "  " + code[:20] + "\n" + code[20:] + "  "
	got, err := profile.DecodeCode(wrapped, cat)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile.Name != "Mi PC ñ" || len(got.Profile.Apps) != 2 {
		t.Fatalf("%+v", got.Profile)
	}
}

func TestShareCodeRefusesGarbageAndBombs(t *testing.T) {
	cat, _ := catalog.Load(catalogdata.FS)
	for name, code := range map[string]string{
		"empty":     "",
		"no prefix": "hello",
		"bad base":  "WF1.!!!",
		"not gzip":  "WF1.aGVsbG8",
		"huge":      "WF1." + strings.Repeat("A", 2<<20),
	} {
		if _, err := profile.DecodeCode(code, cat); err == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
}

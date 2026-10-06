package icons_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/icons"
)

const iconsDir = "../../frontend/public/icons"

// The shipped icons are data the app trusts blindly (it renders them), so the
// index is checked like a contract: every entry names a real, small, plain image
// of the type its extension claims, for an app that exists.
func TestShippedIconIndexIsConsistentAndSafe(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(iconsDir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]string
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(index) < 400 {
		t.Fatalf("suspiciously few icons: %d", len(index))
	}
	for id, file := range index {
		if cat.Apps[id] == nil {
			t.Errorf("%s: icon for an app that is not in the catalog", id)
		}
		if file != filepath.Base(file) || strings.ContainsAny(file, `/\`) {
			t.Errorf("%s: %q is not a plain file name", id, file)
			continue
		}
		b, err := os.ReadFile(filepath.Join(iconsDir, file))
		if err != nil {
			t.Errorf("%s: %v", id, err)
			continue
		}
		ext, ok := icons.Sniff(b)
		if !ok {
			t.Errorf("%s: %s is not a safe image", id, file)
		} else if "."+ext != strings.ToLower(filepath.Ext(file)) && !(ext == "jpg" && strings.EqualFold(filepath.Ext(file), ".jpg")) {
			t.Errorf("%s: %s is really a %s", id, file, ext)
		}
	}
	// And no stray files nobody points at.
	entries, _ := os.ReadDir(iconsDir)
	used := map[string]bool{"index.json": true}
	for _, f := range index {
		used[f] = true
	}
	for _, e := range entries {
		if !used[e.Name()] {
			t.Errorf("orphan icon file %s (run winforge-icons)", e.Name())
		}
	}
}

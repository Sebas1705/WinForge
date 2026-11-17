package pcread

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// TestLiveSmoke runs every tool against this PC. It only reads. It runs only when WINFORGE_LIVE=1 and
// logs counts and timings, never what was found.
func TestLiveSmoke(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to read this PC")
	}
	dir := t.TempDir()
	for i, n := range []string{"a.bin", "b.bin", "c.bin"} {
		if err := os.WriteFile(filepath.Join(dir, n), make([]byte, (i+1)*1000), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := &mcpkit.Env{Now: time.Now, Elevated: func() bool { return false }}
	args := map[string]string{
		"winforge_storage_biggest":  `{"root":` + jsonString(dir) + `,"kind":"files"}`,
		"winforge_storage_projects": `{"roots":[` + jsonString(dir) + `]}`,
	}
	for _, tl := range Tools(env) {
		a := args[tl.Name]
		if a == "" {
			a = "{}"
		}
		start := time.Now()
		res, err := tl.Handler(context.Background(), &mcpkit.Call{Args: json.RawMessage(a), Env: env})
		if err != nil || res == nil {
			t.Errorf("%s: %v", tl.Name, err)
			continue
		}
		if home, _ := os.UserHomeDir(); home != "" && strings.Contains(strings.ToLower(res.Text), strings.ToLower(home)) {
			t.Errorf("%s leaked the home folder", tl.Name)
		}
		n := -1
		var m map[string]any
		if b, _ := json.Marshal(res.Structured); json.Unmarshal(b, &m) == nil {
			for _, k := range []string{"items", "findings", "categories", "volumes", "features", "settings", "distros", "disks", "timeline", "batteries"} {
				if l, ok := m[k].([]any); ok {
					n = len(l)
					break
				}
			}
		}
		t.Logf("%-28s error=%-5v items=%-4d bytes=%-7d %v", tl.Name, res.IsError, n, len(res.Text), time.Since(start).Round(time.Millisecond))
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

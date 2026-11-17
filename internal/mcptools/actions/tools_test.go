package actions

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

var wantTools = []string{
	"winforge_prepare_storage_clean", "winforge_prepare_remove_project_junk", "winforge_prepare_privacy", "winforge_prepare_privacy_restore",
	"winforge_prepare_env_fix", "winforge_prepare_env_restore", "winforge_prepare_startup_toggle", "winforge_prepare_service_mode",
	"winforge_prepare_service_restore", "winforge_prepare_task_toggle", "winforge_prepare_task_restore", "winforge_prepare_feature",
	"winforge_prepare_optimize",
}

func TestEveryToolIsAStrictPrepareTool(t *testing.T) {
	admin := false
	tools := ToolsWith(testEnv(t, &admin, nil), Deps{})
	if len(tools) != len(wantTools) {
		t.Fatalf("%d tools, want %d", len(tools), len(wantTools))
	}
	for i, tool := range tools {
		if tool.Name != wantTools[i] {
			t.Errorf("tool %d = %s, want %s", i, tool.Name, wantTools[i])
		}
		if tool.Class != mcpkit.Prepare {
			t.Errorf("%s: class %v, want prepare", tool.Name, tool.Class)
		}
		if tool.Handler == nil || tool.Title == "" {
			t.Errorf("%s: incomplete", tool.Name)
		}
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Errorf("%s: schema is not strict: %s", tool.Name, tool.InputSchema)
		}
		for _, must := range []string{"winforge_apply_plan", "explicit approval", "changes nothing"} {
			if !strings.Contains(tool.Description, must) {
				t.Errorf("%s: description lacks %q", tool.Name, must)
			}
		}
	}
}

// Every tool says whether it needs administrator rights.
func TestDescriptionsStateAdminNeed(t *testing.T) {
	admin := false
	for _, tool := range ToolsWith(testEnv(t, &admin, nil), Deps{}) {
		d := strings.ToLower(tool.Description)
		if !strings.Contains(d, "administrator") {
			t.Errorf("%s: description says nothing about administrator rights", tool.Name)
		}
	}
}

func TestUnknownArgumentsAreRefusedAndNothingIsStored(t *testing.T) {
	admin := true
	h := newHarness(t, testEnv(t, &admin, &fakeExec{}), newDeps(t).Deps)
	for _, name := range wantTools {
		h.refused(name, `{"bogus":1}`)
		h.refused(name, `{"bogus":1} trailing`)
	}
}

func TestMissingBackendsAreAnsweredNotPanicked(t *testing.T) {
	admin := true
	h := newHarness(t, testEnv(t, &admin, nil), Deps{})
	args := map[string]string{
		"winforge_prepare_storage_clean":       `{"categoryIds":["temp-user"]}`,
		"winforge_prepare_remove_project_junk": `{"paths":["/a"],"roots":["/"]}`,
		"winforge_prepare_privacy":             `{"apply":["advertising-id"]}`,
		"winforge_prepare_privacy_restore":     `{"backupId":"20260101T000000Z"}`,
		"winforge_prepare_env_fix":             `{"findingIds":["x"]}`,
		"winforge_prepare_env_restore":         `{"backupId":"20260101T000000Z"}`,
		"winforge_prepare_startup_toggle":      `{"id":"x","enabled":true}`,
		"winforge_prepare_service_mode":        `{"id":"x","mode":"manual"}`,
		"winforge_prepare_service_restore":     `{"id":"x"}`,
		"winforge_prepare_task_toggle":         `{"id":"x","enabled":true}`,
		"winforge_prepare_task_restore":        `{"id":"x"}`,
		"winforge_prepare_feature":             `{"id":"wsl","enabled":true}`,
		"winforge_prepare_optimize":            `{"recipeIds":["optimize-trim"]}`,
	}
	for _, name := range wantTools {
		got := h.refused(name, args[name])
		if got == "" {
			t.Errorf("%s: empty refusal", name)
		}
	}
}

// A plan runs once even if the store handed it out twice, and never after the request was cancelled.
func TestGuardRunsOnceAndStopsWhenCancelled(t *testing.T) {
	k := &kit{}
	n := 0
	run := k.guard(func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		n++
		return mcpkit.Text("done"), nil
	})
	if r, err := run(context.Background(), &mcpkit.Call{}); err != nil || r.IsError || n != 1 {
		t.Fatalf("first run: %v %v %d", r, err, n)
	}
	if r, _ := run(context.Background(), &mcpkit.Call{}); !r.IsError || n != 1 {
		t.Fatalf("second run must be refused: %+v n=%d", r, n)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n = 0
	run = k.guard(func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) { n++; return mcpkit.Text("x"), nil })
	if _, err := run(ctx, &mcpkit.Call{}); err == nil || n != 0 {
		t.Fatalf("a cancelled request must not run: %v n=%d", err, n)
	}
}

func TestStoreWithoutAPlanStoreRefuses(t *testing.T) {
	k := &kit{}
	r, err := k.store(&mcpkit.Call{}, "x", "s", false, nil)
	if err != nil || !r.IsError {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestHumanBytes(t *testing.T) {
	for in, want := range map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 3 << 30: "3.0 GiB"} {
		if got := human(in); got != want {
			t.Errorf("human(%d) = %s, want %s", in, got, want)
		}
	}
}

func TestMatchAcceptsTheRedactedFormOfOneKey(t *testing.T) {
	admin := false
	k := &kit{env: testEnv(t, &admin, nil)}
	keys := []string{`C:\Users\sebas\a`, `C:\Other\b`}
	if i, ok, _ := k.match(`C:\Users\<user>\a`, keys, identity); !ok || i != 0 {
		t.Errorf("redacted form: %d %v", i, ok)
	}
	if i, ok, _ := k.match(`C:\Other\b`, keys, identity); !ok || i != 1 {
		t.Errorf("raw form: %d %v", i, ok)
	}
	if _, ok, _ := k.match(`C:\Users\<user>\zzz`, keys, identity); ok {
		t.Error("an unlisted path must not match")
	}
	// Two keys that redact to the same text are ambiguous, not guessed.
	if _, ok, amb := k.match(`/home/<user>`, []string{"/home/sebas", "/home/sebas"}, identity); ok || !amb {
		// raw never equals; both redact the same
		t.Errorf("ambiguity: ok=%v amb=%v", ok, amb)
	}
}

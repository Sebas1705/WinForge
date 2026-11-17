package sched

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/system"
)

func TestHelpListsTheSharedExitCodesAndGlobalOptions(t *testing.T) {
	for _, want := range []string{"  7  ", "  8  ", "--json", "--quiet", "--no-color", "--non-interactive", "--timeout"} {
		if !strings.Contains(Help, want) {
			t.Errorf("--help does not mention %q", want)
		}
	}
}

func TestJSONDocumentIsOneObjectWithTheStableFields(t *testing.T) {
	r := runner(t, &fakeExec{}, &fakeSys{})
	code, out, errw := cli(t, &r, "--json", "--mode", "apply")
	if code != ExitOK || strings.Count(out, string(rune(10))) != 1 || strings.Contains(out, string(rune(27))) || strings.HasPrefix(out, string([]byte{0xef, 0xbb, 0xbf})) {
		t.Fatalf("%d %q", code, out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"schema", "command", "ok", "exitCode", "error", "mode", "scheduled", "started", "finished", "restartRequired", "counts", "items"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	if m["command"] != "updates" || m["ok"] != true || m["error"] != nil || m["mode"] != "apply" {
		t.Errorf("%v", m)
	}
	it := m["items"].([]any)[0].(map[string]any)
	for _, k := range []string{"id", "name", "from", "to", "result"} {
		if _, ok := it[k]; !ok {
			t.Errorf("item lacks %s", k)
		}
	}
	if !strings.Contains(errw, "updated") {
		t.Errorf("the human report goes to stderr with --json: %q", errw)
	}
	_, _, errw = cli(t, &r, "--json", "--quiet")
	if errw != "" {
		t.Errorf("--quiet: %q", errw)
	}
}

func TestJSONFailuresCarryTheExitCodeName(t *testing.T) {
	r := runner(t, &fakeExec{exit: map[string]int{"Git.Git": 1}}, &fakeSys{})
	code, out, _ := cli(t, &r, "--json", "--mode", "apply")
	var m struct {
		ExitCode int `json:"exitCode"`
		Error    struct{ Code string }
	}
	_ = json.Unmarshal([]byte(out), &m)
	if code != ExitFailures || m.ExitCode != 1 || m.Error.Code != "failures" {
		t.Errorf("%d %s", code, out)
	}
	r = runner(t, &fakeExec{}, &fakeSys{})
	r.Env.Upgrades = func(context.Context) ([]system.Upgrade, error) { return nil, system.ErrNoWinget }
	code, out, _ = cli(t, &r, "--json")
	_ = json.Unmarshal([]byte(out), &m)
	if code != ExitCheck || m.Error.Code != "prerequisite" || !strings.Contains(out, `"detail":"no-winget"`) {
		t.Errorf("%d %s", code, out)
	}
	code, out, _ = cli(t, &r, "--json", "--mode", "delete")
	if code != ExitUsage || !strings.Contains(out, `"code":"usage"`) {
		t.Errorf("%d %s", code, out)
	}
}

func TestARestartIsExit8(t *testing.T) {
	r := runner(t, &fakeExec{exit: map[string]int{"Git.Git": 3010}}, &fakeSys{})
	if code, _, _ := cli(t, &r, "--mode", "apply"); code != ExitReboot {
		t.Errorf("restart: %d", code)
	}
	if ExitReboot != 8 {
		t.Error("documented as 8")
	}
}

func TestTimeoutFlagOverridesTheDefaultLimit(t *testing.T) {
	r := runner(t, &fakeExec{}, &fakeSys{})
	r.Env.Upgrades = func(ctx context.Context) ([]system.Upgrade, error) { <-ctx.Done(); return nil, ctx.Err() }
	code, out, _ := cli(t, &r, "--json", "--timeout", "100ms")
	if code != ExitTimeout || !strings.Contains(out, `"detail":"timeout"`) {
		t.Errorf("%d %s", code, out)
	}
	if code, _, _ := cli(t, &r, "--timeout", "soon"); code != ExitUsage {
		t.Errorf("bad --timeout: %d", code)
	}
}

//go:build windows

package actions

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/system"
)

// refuseExec stands in for the installer's executor: preparing must never run a recipe, so any call fails the test.
type refuseExec struct{ t *testing.T }

func (r refuseExec) Run(context.Context, string, []string, func(string)) (int, error) {
	r.t.Error("a recipe was run during prepare")
	return 1, errors.New("not allowed")
}

// TestLivePrepareIsReadOnly builds a plan with every prepare tool on this PC's real state, and never runs
// one. It only runs when WINFORGE_LIVE=1. It proves prepare is safe to call for real: it logs, per tool,
// whether a plan was built or refused (and the kind), never values, paths or names.
func TestLivePrepareIsReadOnly(t *testing.T) {
	if os.Getenv("WINFORGE_LIVE") != "1" {
		t.Skip("set WINFORGE_LIVE=1 to build plans against the real PC (nothing is ever run)")
	}
	home, _ := os.UserHomeDir()
	user := home[strings.LastIndexAny(home, `\/`)+1:]
	env := &mcpkit.Env{
		Version:  "live",
		Catalog:  func() (*catalog.Catalog, error) { return catalog.Load(catalogdata.FS) },
		Elevated: system.IsElevated,
		Exec:     refuseExec{t},
		Now:      time.Now,
		Redact:   func(s string) string { return strings.NewReplacer(home, `C:\Users\<user>`, user, "<user>").Replace(s) },
	}
	d := RealDeps(env)
	store := newPlanStore()
	call := func(name, args string) {
		for _, tool := range ToolsWith(env, d) {
			if tool.Name != name {
				continue
			}
			before := store.puts
			res, err := tool.Handler(context.Background(), &mcpkit.Call{Args: json.RawMessage(args), Env: env, Plans: store, Progress: func(float64, float64, string) {}})
			switch {
			case err != nil:
				t.Logf("%-40s error (%s)", name, errKind(err))
			case res.IsError:
				t.Logf("%-40s refused: %.90s", name, strings.ReplaceAll(res.Text, "\n", " "))
			case store.puts == before+1:
				p := res.Structured.(prepared)
				t.Logf("%-40s planned kind=%s needsAdmin=%v", name, p.Kind, p.NeedsAdmin)
			default:
				t.Errorf("%s: no plan and no refusal", name)
			}
		}
	}

	// Storage: the fixed, non-admin folder category that most PCs have.
	call("winforge_prepare_storage_clean", `{"categoryIds":["temp-user"]}`)
	// Project junk: a search of an empty temporary folder lists nothing, so the request is refused.
	call("winforge_prepare_remove_project_junk", `{"paths":["`+strings.ReplaceAll(os.TempDir(), `\`, `\\`)+`\\nothing"],"roots":["`+strings.ReplaceAll(os.TempDir(), `\`, `\\`)+`"]}`)
	// Privacy: apply the first setting that is not already private.
	if list, err := d.Privacy.List(); err == nil {
		for _, v := range list {
			if v.State != "private" && v.State != "unknown" {
				call("winforge_prepare_privacy", `{"apply":["`+v.ID+`"]}`)
				break
			}
		}
	}
	call("winforge_prepare_privacy_restore", `{"backupId":"20000101T000000Z"}`)
	if rep, err := d.Env.Scan(); err == nil {
		for _, f := range rep.Findings {
			if f.Fixable && f.Scope == "user" {
				b, _ := json.Marshal(f.ID)
				call("winforge_prepare_env_fix", `{"findingIds":[`+string(b)+`]}`)
				break
			}
		}
	}
	call("winforge_prepare_env_restore", `{"backupId":"20000101T000000Z"}`)
	if items, err := d.Startup.List(); err == nil && len(items) > 0 {
		id, _ := json.Marshal(items[0].ID)
		call("winforge_prepare_startup_toggle", `{"id":`+string(id)+`,"enabled":`+map[bool]string{true: "false", false: "true"}[items[0].Enabled]+`}`)
	}
	if items, err := d.Services.List(); err == nil && len(items) > 0 {
		id, _ := json.Marshal(items[0].ID)
		call("winforge_prepare_service_mode", `{"id":`+string(id)+`,"mode":"manual"}`)
		call("winforge_prepare_service_restore", `{"id":`+string(id)+`}`)
	}
	if items, err := d.Tasks.List(); err == nil && len(items) > 0 {
		id, _ := json.Marshal(items[0].ID)
		call("winforge_prepare_task_toggle", `{"id":`+string(id)+`,"enabled":`+map[bool]string{true: "false", false: "true"}[items[0].Enabled]+`}`)
		call("winforge_prepare_task_restore", `{"id":`+string(id)+`}`)
	}
	for _, f := range d.Features() {
		if f.State == "on" || f.State == "off" {
			call("winforge_prepare_feature", `{"id":"`+f.ID+`","enabled":`+map[bool]string{true: "false", false: "true"}[f.State == "on"]+`}`)
			break
		}
	}
	call("winforge_prepare_optimize", `{"recipeIds":["optimize-trim"]}`)
	t.Logf("%d plans were stored and none was run", store.puts)
}

func errKind(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	return "failed"
}

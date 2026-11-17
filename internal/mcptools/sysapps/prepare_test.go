package sysapps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/history"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

func (r *rig) history() []history.Entry {
	r.t.Helper()
	es, err := (&history.FileStore{Path: filepath.Join(r.appData, "history.json")}).All()
	must(r.t, err)
	return es
}

func stepsByID(m map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, s := range list(m, "steps") {
		out[str(obj(s), "id")] = obj(s)
	}
	return out
}

func TestPrepareInstallStoresAPlanAndChangesNothing(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0")
	m := r.ok("winforge_prepare_install", `{"profileId":"starter"}`)
	if str(m, "planId") == "" || str(m, "kind") != "install" || str(m, "expiresAt") != "2026-11-17T12:30:00Z" || boolean(m, "needsAdmin") {
		t.Fatalf("prepared %v", m)
	}
	if str(m, "applyWith") != "winforge_apply_plan" {
		t.Fatal("must say how to apply")
	}
	sum := obj(m["summary"])
	if got := ids(list(sum, "steps"), "id"); len(got) != 2 || got[0] != "gamma" || got[1] != "rec-a" {
		t.Fatalf("summary steps %v", got)
	}
	if al := list(sum, "alreadyInstalled"); len(al) != 1 || al[0] != "alpha" {
		t.Fatalf("already %v", al)
	}
	if str(sum, "target") != "starter" {
		t.Fatalf("target %v", sum["target"])
	}
	if r.plans.count() != 1 {
		t.Fatal("exactly one plan must be stored")
	}
	if len(r.exec.wingetCalls()) != 0 || len(r.exec.calls) != 0 {
		t.Fatalf("prepare ran commands: %v", r.exec.calls)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.inv) != 1 {
		t.Fatal("prepare changed the fake PC")
	}
}

func TestPrepareInstallNothingToDoIsNotStored(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0")
	m := r.ok("winforge_prepare_install", `{"appIds":["alpha"]}`)
	if !boolean(m, "nothingToDo") || str(m, "planId") != "" || r.plans.count() != 0 {
		t.Fatalf("nothing to do %v", m)
	}
}

func TestPrepareInstallWithoutWingetIsRefusedUpFront(t *testing.T) {
	r := newRig(t)
	r.snapErr = system.ErrNoWinget
	r.fails("winforge_prepare_install", `{"appIds":["gamma"]}`, "winget")
	if r.plans.count() != 0 {
		t.Fatal("stored")
	}
	// a plan of recipes only does not need winget
	r.cat.Profiles["recipeonly"] = &catalog.Profile{ID: "recipeonly", Name: "Recipes", Recipes: []string{"rec-d"}}
	m := r.ok("winforge_prepare_install", `{"profileId":"recipeonly"}`)
	if str(m, "planId") == "" {
		t.Fatalf("recipes-only plan should be stored: %v", m)
	}
}

func TestApplyInstallRunsThroughTheRunnerAndRecordsHistory(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0")
	id := str(r.ok("winforge_prepare_install", `{"profileId":"starter"}`), "planId")
	res := r.apply(context.Background(), id)
	if res.IsError {
		t.Fatalf("apply failed: %s", res.Text)
	}
	m := decodeText(t, res)
	if num(obj(m["counts"]), "ok") != 2 || num(obj(m["counts"]), "failed") != 0 || boolean(m, "rebootRequired") {
		t.Fatalf("counts %v", m)
	}
	st := stepsByID(m)
	if str(st["gamma"], "result") != "ok" || str(st["gamma"], "to") != "2.0.0" || str(st["gamma"], "from") != "" || str(st["rec-a"], "kind") != "recipe" {
		t.Fatalf("steps %v", st)
	}
	calls := r.exec.wingetCalls()
	if len(calls) != 1 || !strings.Contains(calls[0], "install --id Gamma.Tool --exact") || !strings.Contains(calls[0], "--version 1.2.3") {
		t.Fatalf("winget calls %v", calls)
	}
	h := r.history()
	if len(h) != 1 || h[0].App != "gamma" || h[0].Action != history.Install || h[0].Result != history.OK || h[0].To != "2.0.0" || h[0].Source != history.FromSingle {
		t.Fatalf("history %+v", h)
	}
	if !boolean(m, "historyRecorded") {
		t.Fatal("historyRecorded")
	}
	// a plan runs once
	if _, err := r.plans.Take(id); !errors.Is(err, mcpkit.ErrPlanUnknown) {
		t.Fatalf("plan must be single use: %v", err)
	}
}

func TestApplyInstallFailureAndSkippedDependants(t *testing.T) {
	r := newRig(t)
	r.exec.exit = func(name string, args []string) int {
		if name == "winget" && strings.Contains(strings.Join(args, " "), "Acme.Alpha") {
			return 1
		}
		return 0
	}
	id := str(r.ok("winforge_prepare_install", `{"profileId":"starter"}`), "planId")
	res := r.apply(context.Background(), id)
	if !res.IsError {
		t.Fatal("a failed step is an error result")
	}
	m := decodeText(t, res)
	st := stepsByID(m)
	if str(st["alpha"], "result") != "failed" || str(st["alpha"], "message") == "" {
		t.Fatalf("alpha %v", st["alpha"])
	}
	if str(st["rec-a"], "result") != "skipped" || str(st["rec-a"], "reason") != "dependency" {
		t.Fatalf("recipe depends on the failed app: %v", st["rec-a"])
	}
	if str(st["gamma"], "result") != "ok" {
		t.Fatalf("independent app still installs: %v", st["gamma"])
	}
	c := obj(m["counts"])
	if num(c, "failed") != 1 || num(c, "ok") != 1 || num(c, "skipped") != 1 {
		t.Fatalf("counts %v", c)
	}
	h := r.history()
	var failed bool
	for _, e := range h {
		if e.App == "alpha" && e.Result == history.Failed {
			failed = true
		}
	}
	if !failed {
		t.Fatalf("failure not recorded: %+v", h)
	}
}

func TestApplyAdminRefusalWhenNotElevated(t *testing.T) {
	r := newRig(t)
	m := r.ok("winforge_prepare_install", `{"appIds":["beta"]}`)
	if !boolean(m, "needsAdmin") {
		t.Fatal("beta needs admin")
	}
	res := r.apply(context.Background(), str(m, "planId"))
	if !res.IsError {
		t.Fatal("must be refused")
	}
	out := decodeText(t, res)
	if str(out, "refused") != "admin" || num(obj(out["counts"]), "skipped") != 2 || len(r.exec.wingetCalls()) != 0 {
		t.Fatalf("refusal %v calls %v", out, r.exec.calls)
	}
	for _, s := range list(out, "steps") {
		if str(obj(s), "result") != "skipped" || str(obj(s), "reason") != "admin" {
			t.Fatalf("step %v", s)
		}
	}
	if len(r.history()) != 0 {
		t.Fatal("a refused run writes no history")
	}

	// elevated: the same plan runs
	r.elev = true
	m = r.ok("winforge_prepare_install", `{"appIds":["beta"]}`)
	res = r.apply(context.Background(), str(m, "planId"))
	if res.IsError || num(obj(decodeText(t, res)["counts"]), "ok") != 2 {
		t.Fatalf("elevated run %s", res.Text)
	}
}

func TestApplyWingetMissingAtRunTime(t *testing.T) {
	r := newRig(t)
	id := str(r.ok("winforge_prepare_install", `{"appIds":["gamma"]}`), "planId")
	r.winget = false
	res := r.apply(context.Background(), id)
	m := decodeText(t, res)
	if !res.IsError || str(m, "refused") != "prerequisite" || !strings.Contains(str(m, "message"), "winget") || len(r.exec.wingetCalls()) != 0 {
		t.Fatalf("winget missing %s", res.Text)
	}
}

func TestApplyLockBusy(t *testing.T) {
	r := newRig(t)
	id := str(r.ok("winforge_prepare_install", `{"appIds":["gamma"]}`), "planId")
	release, err := sched.Lock{Path: r.lock}.Acquire()
	must(t, err)
	res := r.apply(context.Background(), id)
	m := decodeText(t, res)
	if !res.IsError || str(m, "refused") != "busy" || len(r.exec.wingetCalls()) != 0 {
		t.Fatalf("busy %s", res.Text)
	}
	release()
	// the lock is released once the plan has run, so a second plan can run
	id = str(r.ok("winforge_prepare_install", `{"appIds":["gamma"]}`), "planId")
	if res := r.apply(context.Background(), id); res.IsError {
		t.Fatalf("lock not free: %s", res.Text)
	}
	id = str(r.ok("winforge_prepare_install", `{"appIds":["delta"]}`), "planId")
	if res := r.apply(context.Background(), id); res.IsError {
		t.Fatalf("lock not released after the run: %s", res.Text)
	}
}

func TestApplyCancellation(t *testing.T) {
	r := newRig(t)
	id := str(r.ok("winforge_prepare_install", `{"appIds":["gamma","delta"]}`), "planId")
	r.exec.block = true
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	res := r.apply(ctx, id)
	m := decodeText(t, res)
	if !res.IsError || str(m, "stopped") != "cancelled" {
		t.Fatalf("cancel %s", res.Text)
	}
	if num(obj(m["counts"]), "ok") != 0 {
		t.Fatalf("nothing finished: %v", m["counts"])
	}
	for _, s := range list(m, "steps") {
		if str(obj(s), "result") == "ok" {
			t.Fatal("a cancelled step cannot be ok")
		}
	}
	// the lock is free again
	r.exec.block = false
	id = str(r.ok("winforge_prepare_install", `{"appIds":["gamma"]}`), "planId")
	if res := r.apply(context.Background(), id); res.IsError {
		t.Fatalf("lock stuck after cancel: %s", res.Text)
	}
}

func TestApplyReportsReboot(t *testing.T) {
	r := newRig(t)
	r.exec.exit = func(string, []string) int { return 3010 }
	id := str(r.ok("winforge_prepare_install", `{"appIds":["gamma"]}`), "planId")
	res := r.apply(context.Background(), id)
	m := decodeText(t, res)
	if res.IsError || !boolean(m, "rebootRequired") || !boolean(stepsByID(m)["gamma"], "reboot") {
		t.Fatalf("reboot %s", res.Text)
	}
	h := r.history()
	if len(h) != 1 || h[0].Result != history.Restart {
		t.Fatalf("history %+v", h)
	}
	// 1641 is the same "restart" family but only 3010 is what winget reports as success with restart: it is a plain failure
}

func TestApplyRedactsErrorsAndRefusesChangedCatalog(t *testing.T) {
	r := newRig(t)
	r.exec.err = errors.New("cannot start " + filepath.Join(r.root, "tools", "winget.exe"))
	id := str(r.ok("winforge_prepare_install", `{"appIds":["gamma"]}`), "planId")
	res := r.apply(context.Background(), id)
	if strings.Contains(res.Text, r.root) || !strings.Contains(res.Text, "home") {
		t.Fatalf("error text not redacted: %s", res.Text)
	}

	r.exec.err = nil
	r.exec.calls = nil
	id = str(r.ok("winforge_prepare_install", `{"appIds":["gamma"]}`), "planId")
	delete(r.cat.Apps, "gamma")
	res = r.apply(context.Background(), id)
	if !res.IsError || !strings.Contains(res.Text, "no longer in the catalog") || len(r.exec.wingetCalls()) != 0 {
		t.Fatalf("changed catalog: %s", res.Text)
	}
}

func TestApplyNoExecutorIsRefused(t *testing.T) {
	r := newRig(t)
	id := str(r.ok("winforge_prepare_install", `{"appIds":["gamma"]}`), "planId")
	r.env.Exec = nil
	if res := r.apply(context.Background(), id); !res.IsError {
		t.Fatal("no executor must not run anything")
	}
}

// ---- updates -------------------------------------------------------------------------------

func updateRig(t *testing.T) *rig {
	r := newRig(t)
	r.install("alpha", "1.0", "beta", "1.0", "gamma", "1.0", "delta", "1.0")
	r.upgrades = []system.Upgrade{
		{ID: "Acme.Alpha", Name: "Alpha", Current: "1.0", Available: "2.0"},
		{ID: "Bravo.Beta", Name: "Beta", Current: "1.0", Available: "2.0"},
		{ID: "Gamma.Tool", Name: "Gamma", Current: "1.0", Available: "2.0"},
		{ID: "Unrelated.Program", Name: "Other", Current: "1", Available: "2"},
	}
	must(t, (&updates.Store{Path: filepath.Join(r.appData, "ignored-updates.json")}).Set("gamma", updates.Rule{Mode: updates.Always}))
	return r
}

func TestPrepareUpdateHonorsIgnoreAndAdmin(t *testing.T) {
	r := updateRig(t)
	m := r.ok("winforge_prepare_update", `{"all":true}`)
	sum := obj(m["summary"])
	if got := ids(list(sum, "steps"), "id"); len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("steps %v", got)
	}
	if str(obj(list(sum, "steps")[0]), "current") != "1.0" || str(obj(list(sum, "steps")[0]), "available") != "2.0" {
		t.Fatalf("versions %v", list(sum, "steps"))
	}
	reasons := map[string]string{}
	for _, s := range list(sum, "skipped") {
		reasons[str(obj(s), "id")] = str(obj(s), "reason")
	}
	if reasons["beta"] != "needs-admin" || reasons["gamma"] != "ignored" || len(reasons) != 2 {
		t.Fatalf("skipped %v", reasons)
	}
	if boolean(m, "needsAdmin") || r.plans.count() != 1 {
		t.Fatalf("not elevated: no admin steps %v", m)
	}
	if len(r.exec.wingetCalls()) != 0 {
		t.Fatal("prepare ran winget")
	}

	// elevated: the admin app is included, the ignored one still is not
	r.elev = true
	m = r.ok("winforge_prepare_update", `{"all":true}`)
	if got := ids(list(obj(m["summary"]), "steps"), "id"); len(got) != 2 || got[0] != "alpha" || got[1] != "beta" || !boolean(m, "needsAdmin") {
		t.Fatalf("elevated steps %v", got)
	}

	// a named list says why an app is left out
	m = r.ok("winforge_prepare_update", `{"appIds":["alpha","gamma","delta"]}`)
	reasons = map[string]string{}
	for _, s := range list(obj(m["summary"]), "skipped") {
		reasons[str(obj(s), "id")] = str(obj(s), "reason")
	}
	if reasons["gamma"] != "ignored" || reasons["delta"] != "no-update" || len(list(obj(m["summary"]), "steps")) != 1 {
		t.Fatalf("named %v", reasons)
	}
}

func TestPrepareUpdateNothingToDoAndErrors(t *testing.T) {
	r := updateRig(t)
	m := r.ok("winforge_prepare_update", `{"appIds":["gamma"]}`) // the only one is ignored
	if !boolean(m, "nothingToDo") || r.plans.count() != 0 {
		t.Fatalf("nothing to do %v", m)
	}
	r.upErr = system.ErrNoWinget
	r.fails("winforge_prepare_update", `{"all":true}`, "winget")
	r.upErr = nil
	must(t, os.WriteFile(filepath.Join(r.appData, "ignored-updates.json"), []byte("{oops"), 0o644))
	r.fails("winforge_prepare_update", `{"all":true}`, "ignored-updates")
	if r.plans.count() != 0 {
		t.Fatal("stored despite a damaged ignore list")
	}
}

func TestApplyUpdateRunsUpgradeAndRecordsHistory(t *testing.T) {
	r := updateRig(t)
	id := str(r.ok("winforge_prepare_update", `{"all":true}`), "planId")
	res := r.apply(context.Background(), id)
	if res.IsError {
		t.Fatalf("update failed: %s", res.Text)
	}
	m := decodeText(t, res)
	st := stepsByID(m)
	if str(st["alpha"], "result") != "ok" || str(st["alpha"], "from") != "1.0" || str(st["alpha"], "to") != "2.0.0" || str(st["alpha"], "kind") != "upgrade" {
		t.Fatalf("alpha %v", st["alpha"])
	}
	calls := r.exec.wingetCalls()
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "winget upgrade --id Acme.Alpha --exact") {
		t.Fatalf("calls %v", calls)
	}
	h := r.history()
	if len(h) != 1 || h[0].Action != history.Update || h[0].Source != history.FromUpdate || h[0].From != "1.0" || h[0].To != "2.0.0" {
		t.Fatalf("history %+v", h)
	}
}

// ---- uninstall -----------------------------------------------------------------------------

func TestPrepareAndApplyUninstall(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0", "beta", "2.0")
	m := r.ok("winforge_prepare_uninstall", `{"appIds":["alpha"]}`)
	sum := obj(m["summary"])
	if str(m, "kind") != "uninstall" || !boolean(sum, "irreversible") || !boolean(sum, "reviewLeftoversInApp") {
		t.Fatalf("summary %v", sum)
	}
	if st := list(sum, "steps"); len(st) != 1 || str(obj(st[0]), "kind") != "uninstall" || str(obj(st[0]), "id") != "alpha" {
		t.Fatalf("steps %v", st)
	}
	ub := list(sum, "usedBy")
	if len(ub) != 1 || str(obj(ub[0]), "id") != "alpha" || obj(ub[0])["requiredBy"].([]any)[0] != "beta" {
		t.Fatalf("usedBy %v", ub)
	}
	if len(r.exec.wingetCalls()) != 0 {
		t.Fatal("prepare uninstalled something")
	}

	res := r.apply(context.Background(), str(m, "planId"))
	if res.IsError {
		t.Fatalf("uninstall failed: %s", res.Text)
	}
	out := decodeText(t, res)
	st := stepsByID(out)
	if str(st["alpha"], "result") != "ok" || str(st["alpha"], "from") != "1.0" || str(st["alpha"], "to") != "" {
		t.Fatalf("alpha %v", st["alpha"])
	}
	calls := r.exec.wingetCalls()
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "winget uninstall --id Acme.Alpha --exact") || !strings.Contains(calls[0], "--silent") {
		t.Fatalf("calls %v", calls)
	}
	if boolean(out, "historyRecorded") || len(r.history()) != 0 {
		t.Fatal("uninstalls are not app history")
	}
	r.mu.Lock()
	_, still := r.inv["acme.alpha"]
	r.mu.Unlock()
	if still {
		t.Fatal("the fake PC still has alpha")
	}
}

func TestPrepareUninstallRejectsWhatIsNotInstalled(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0")
	res := r.call("winforge_prepare_uninstall", `{"appIds":["alpha","gamma","delta"]}`)
	if !res.IsError || !strings.Contains(res.Text, "gamma, delta") || r.plans.count() != 0 {
		t.Fatalf("%s", res.Text)
	}
	r.snapErr = system.ErrNoWinget
	r.fails("winforge_prepare_uninstall", `{"appIds":["alpha"]}`, "winget")
}

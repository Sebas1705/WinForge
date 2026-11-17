package sysapps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/drift"
	"github.com/Sebas1705/WinForge/internal/history"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/profile"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

func TestToolContract(t *testing.T) {
	r := newRig(t)
	want := map[string]mcpkit.Class{
		"winforge_status": mcpkit.Read, "winforge_catalog_search": mcpkit.Read, "winforge_installed_apps": mcpkit.Read,
		"winforge_updates_check": mcpkit.Read, "winforge_profiles_list": mcpkit.Read, "winforge_profile_get": mcpkit.Read,
		"winforge_install_plan": mcpkit.Read, "winforge_eol_report": mcpkit.Read, "winforge_history": mcpkit.Read,
		"winforge_compare_profile": mcpkit.Read, "winforge_prepare_install": mcpkit.Prepare,
		"winforge_prepare_update": mcpkit.Prepare, "winforge_prepare_uninstall": mcpkit.Prepare,
	}
	if len(r.tools) != len(want) {
		t.Fatalf("%d tools, want %d", len(r.tools), len(want))
	}
	for name, class := range want {
		tl, ok := r.tools[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if tl.Class != class {
			t.Errorf("%s class %v, want %v", name, tl.Class, class)
		}
		if tl.Title == "" || len(tl.Description) < 80 || tl.Handler == nil {
			t.Errorf("%s: title, description or handler missing", name)
		}
		var s map[string]any
		if err := json.Unmarshal(tl.InputSchema, &s); err != nil {
			t.Fatalf("%s schema: %v", name, err)
		}
		if s["type"] != "object" || s["additionalProperties"] != false {
			t.Errorf("%s: schema must be a strict object, got %v", name, s)
		}
		if class == mcpkit.Prepare {
			if !strings.Contains(tl.Description, "winforge_apply_plan") || !strings.Contains(tl.Description, "approval") || !strings.Contains(tl.Description, "CHANGES NOTHING") {
				t.Errorf("%s: description must tell the model about apply and approval", name)
			}
		}
		if !strings.Contains(strings.ToLower(tl.Description), "never") && !strings.Contains(tl.Description, "CHANGES NOTHING") && !strings.Contains(tl.Description, "no installed software") {
			t.Errorf("%s: description does not say what it never does", name)
		}
	}
	// the tools that read personal data say so
	for _, n := range []string{"winforge_installed_apps", "winforge_updates_check", "winforge_eol_report", "winforge_compare_profile", "winforge_install_plan"} {
		if !strings.Contains(strings.ToLower(r.tools[n].Description), "personal data") {
			t.Errorf("%s must say it reads personal data", n)
		}
	}
}

func TestArgumentValidation(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0")
	long := strings.Repeat("a", 64)
	many := `["` + strings.Join(make([]string, 201), `","`) + `"]`
	cases := []struct{ tool, args string }{
		{"winforge_status", `{"x":1}`},
		{"winforge_catalog_search", `{}`},
		{"winforge_catalog_search", `{"query":1}`},
		{"winforge_catalog_search", `{"query":"a","extra":true}`},
		{"winforge_catalog_search", `{"query":"a","limit":0}`},
		{"winforge_catalog_search", `{"query":"a","limit":201}`},
		{"winforge_catalog_search", `{"query":"a","limit":"5"}`},
		{"winforge_catalog_search", `{"query":"a","withInstalled":"yes"}`},
		{"winforge_catalog_search", `{"query":"` + strings.Repeat("x", 101) + `"}`},
		{"winforge_installed_apps", `{"limit":-1}`},
		{"winforge_installed_apps", `[]`},
		{"winforge_updates_check", `{"limit":5}`},
		{"winforge_profiles_list", `{"limit":300}`},
		{"winforge_profile_get", `{}`},
		{"winforge_profile_get", `{"id":"Bad Id"}`},
		{"winforge_profile_get", `{"id":"` + long + `"}`},
		{"winforge_profile_get", `{"id":"../etc"}`},
		{"winforge_install_plan", `{}`},
		{"winforge_install_plan", `{"profileId":"starter","appIds":["alpha"]}`},
		{"winforge_install_plan", `{"appIds":[]}`},
		{"winforge_install_plan", `{"appIds":["alpha;rm"]}`},
		{"winforge_install_plan", `{"appIds":["ALPHA"]}`},
		{"winforge_install_plan", `{"appIds":"alpha"}`},
		{"winforge_install_plan", `{"appIds":["nope"]}`},
		{"winforge_install_plan", `{"profileId":"nope"}`},
		{"winforge_eol_report", `{"limit":0}`},
		{"winforge_history", `{"result":"great"}`},
		{"winforge_history", `{"app":"a b"}`},
		{"winforge_history", `{"limit":1000}`},
		{"winforge_compare_profile", `{}`},
		{"winforge_compare_profile", `{"path":"C:\\a.json","code":"WF1.x"}`},
		{"winforge_compare_profile", `{"path":5}`},
		{"winforge_prepare_install", `{}`},
		{"winforge_prepare_install", `{"profileId":"starter","appIds":["alpha"]}`},
		{"winforge_prepare_install", `{"appIds":` + many + `}`},
		{"winforge_prepare_install", `{"appIds":["alpha","bad id"]}`},
		{"winforge_prepare_install", `{"appIds":["not-there"]}`},
		{"winforge_prepare_install", `{"profileId":"not-there"}`},
		{"winforge_prepare_install", `{"appIds":["alpha"],"x":1}`},
		{"winforge_prepare_update", `{}`},
		{"winforge_prepare_update", `{"all":true,"appIds":["alpha"]}`},
		{"winforge_prepare_update", `{"all":false}`},
		{"winforge_prepare_update", `{"appIds":["not-there"]}`},
		{"winforge_prepare_update", `{"all":"yes"}`},
		{"winforge_prepare_uninstall", `{}`},
		{"winforge_prepare_uninstall", `{"appIds":["not-there"]}`},
		{"winforge_prepare_uninstall", `{"appIds":["gamma"]}`}, // known but not installed
		{"winforge_prepare_uninstall", `{"appIds":["a/b"]}`},
		{"winforge_prepare_uninstall", `{"appIds":null}`},
	}
	for _, c := range cases {
		t.Run(c.tool+" "+c.args[:min(len(c.args), 50)], func(t *testing.T) {
			res := r.call(c.tool, c.args)
			if !res.IsError {
				t.Fatalf("expected an error, got %s", res.Text)
			}
			if r.plans.count() != 0 {
				t.Fatal("a plan was stored for a rejected call")
			}
		})
	}
	if calls := r.exec.wingetCalls(); len(calls) != 0 {
		t.Fatalf("validation ran winget: %v", calls)
	}
}

func TestStatus(t *testing.T) {
	r := newRig(t)
	m := r.ok("winforge_status", `{}`)
	if str(m, "version") != "9.9.9-test" || boolean(m, "elevated") {
		t.Fatalf("status %v", m)
	}
	w := obj(m["winget"])
	if !boolean(w, "present") || str(w, "version") != "1.9.25180" {
		t.Fatalf("winget %v", w)
	}
	if strings.Contains(str(w, "path"), r.root) || !strings.Contains(str(w, "path"), "<home>") {
		t.Fatalf("winget path not redacted: %v", w)
	}
	if strings.Contains(str(m, "appData"), r.root) || !strings.HasPrefix(str(m, "appData"), "<home>") {
		t.Fatalf("appData not redacted: %v", m["appData"])
	}
	if num(m, "catalogApps") != len(r.cat.Apps) || num(m, "catalogProfiles") != 2 || num(m, "catalogRecipes") != 2 {
		t.Fatalf("counts %v", m)
	}
	if r.snapshots() != 0 {
		t.Fatal("status scanned the PC")
	}

	r.winget, r.elev = false, true
	m = r.ok("winforge_status", `{}`)
	if boolean(obj(m["winget"]), "present") || !boolean(m, "elevated") {
		t.Fatalf("status without winget %v", m)
	}
	if calls := r.exec.calls; len(calls) != 1 {
		t.Fatalf("winget --version should only run when winget exists: %v", calls)
	}
}

func TestCatalogSearchNeverScansUnlessAsked(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.5")
	m := r.ok("winforge_catalog_search", `{"query":"editor"}`)
	if r.snapshots() != 0 {
		t.Fatal("withInstalled false must not call Snapshot")
	}
	if boolean(m, "installedChecked") {
		t.Fatal("installedChecked must be false")
	}
	apps := list(m, "apps")
	if len(apps) != 1 || str(obj(apps[0]), "id") != "alpha" {
		t.Fatalf("apps %v", apps)
	}
	a := obj(apps[0])
	if _, has := a["installed"]; has {
		t.Fatal("installed field present without withInstalled")
	}
	if str(a, "wingetId") != "Acme.Alpha" || !boolean(a, "openSource") || str(a, "publisher") != "Acme" || str(a, "category") != "Editors" {
		t.Fatalf("fields %v", a)
	}

	m = r.ok("winforge_catalog_search", `{"query":"editor","withInstalled":true}`)
	if r.snapshots() != 1 {
		t.Fatalf("snapshots %d", r.snapshots())
	}
	a = obj(list(m, "apps")[0])
	if !boolean(a, "installed") || str(a, "installedVersion") != "1.5" {
		t.Fatalf("installed fields %v", a)
	}
	m = r.ok("winforge_catalog_search", `{"query":"tool","withInstalled":true}`)
	g := obj(list(m, "apps")[0])
	if inst, has := g["installed"]; !has || inst != false {
		t.Fatalf("not installed must say false: %v", g)
	}
}

func TestCatalogSearchFiltersRankingAndLimits(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0", "gamma", "2.0")
	// every word must match; ranking puts exact and prefix matches first
	m := r.ok("winforge_catalog_search", `{"query":"alpha text"}`)
	if got := ids(list(m, "apps"), "id"); len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("and-match %v", got)
	}
	m = r.ok("winforge_catalog_search", `{"query":"a","category":"tools"}`)
	if got := ids(list(m, "apps"), "id"); len(got) != 1 || got[0] != "gamma" {
		t.Fatalf("category %v", got)
	}
	m = r.ok("winforge_catalog_search", `{"query":"","installedOnly":true}`)
	if got := ids(list(m, "apps"), "id"); len(got) != 2 || got[0] != "alpha" || got[1] != "gamma" {
		t.Fatalf("installedOnly %v", got)
	}
	if !boolean(m, "installedChecked") {
		t.Fatal("installedOnly implies the scan")
	}
	m = r.ok("winforge_catalog_search", `{"query":"python 3.10"}`)
	if got := ids(list(m, "apps"), "id"); len(got) != 1 || got[0] != "python-3-10" {
		t.Fatalf("python %v", got)
	}

	m = r.ok("winforge_catalog_search", `{"query":"bulk"}`)
	if len(list(m, "apps")) != defaultLimit || !boolean(m, "truncated") || num(m, "total") != 250 {
		t.Fatalf("default limit: %d apps, truncated=%v total=%d", len(list(m, "apps")), m["truncated"], num(m, "total"))
	}
	m = r.ok("winforge_catalog_search", `{"query":"bulk","limit":200}`)
	if len(list(m, "apps")) != 200 || !boolean(m, "truncated") {
		t.Fatalf("max limit: %d", len(list(m, "apps")))
	}
	m = r.ok("winforge_catalog_search", `{"query":"bulk app 00","limit":3}`)
	if got := ids(list(m, "apps"), "id"); len(got) != 3 || got[0] != "bulk-001" {
		t.Fatalf("limit 3 %v", got)
	}
	m = r.ok("winforge_catalog_search", `{"query":"alpha"}`)
	if boolean(m, "truncated") {
		t.Fatal("one hit must not be truncated")
	}
}

func TestInstalledApps(t *testing.T) {
	r := newRig(t)
	r.install("beta", "3.1", "alpha", "1.0")
	for i := 1; i <= 60; i++ {
		r.install(fmt.Sprintf("bulk-%03d", i), "1.0")
	}
	m := r.ok("winforge_installed_apps", `{"limit":10}`)
	inst := list(m, "installed")
	if len(inst) != 10 || !boolean(m, "truncated") || num(m, "total") != 62 {
		t.Fatalf("%d installed, truncated=%v total=%v", len(inst), m["truncated"], m["total"])
	}
	if str(obj(inst[0]), "id") != "alpha" || str(obj(inst[0]), "version") != "1.0" || str(obj(inst[1]), "id") != "beta" {
		t.Fatalf("order/versions %v", ids(inst, "id"))
	}
	m = r.ok("winforge_installed_apps", `{}`)
	if len(list(m, "installed")) != 50 || !boolean(m, "truncated") {
		t.Fatal("default limit is 50")
	}
	if !strings.Contains(r.tools["winforge_installed_apps"].Description, "PERSONAL DATA") {
		t.Fatal("must be flagged as personal data")
	}

	// without winget the scan still answers, with a warning
	r.snapErr = system.ErrNoWinget
	m = r.ok("winforge_installed_apps", `{}`)
	if len(list(m, "warnings")) != 1 {
		t.Fatalf("warnings %v", m["warnings"])
	}
}

func TestUpdatesCheckHonorsIgnoreRules(t *testing.T) {
	r := newRig(t)
	r.upgrades = []system.Upgrade{
		{ID: "Acme.Alpha", Name: "Alpha", Current: "1.0", Available: "2.0"},
		{ID: "Bravo.Beta", Name: "Beta", Current: "1.0", Available: "2.0"},
		{ID: "Gamma.Tool", Name: "Gamma", Current: "1.0", Available: "2.0"},
		{ID: "Delta.Player", Name: "Delta", Current: "1.0", Available: "2.0"},
		{ID: "Unrelated.Program", Name: "Other", Current: "1", Available: "2"},
	}
	st := &updates.Store{Path: filepath.Join(r.appData, "ignored-updates.json")}
	must(t, st.Set("beta", updates.Rule{Mode: updates.Always}))
	must(t, st.Set("gamma", updates.Rule{Mode: updates.Version, Version: "2.0"}))
	must(t, st.Set("delta", updates.Rule{Mode: updates.Until, Until: fixedNow.Add(48 * time.Hour)}))
	must(t, st.Set("alpha", updates.Rule{Mode: updates.Version, Version: "1.5"})) // another version: still active

	m := r.ok("winforge_updates_check", `{}`)
	if got := ids(list(m, "active"), "id"); len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("active %v", got)
	}
	ign := list(m, "ignored")
	if got := ids(ign, "id"); len(got) != 3 {
		t.Fatalf("ignored %v", got)
	}
	modes := map[string]string{}
	for _, x := range ign {
		modes[str(obj(x), "id")] = str(obj(obj(x)["ignore"]), "mode")
	}
	if modes["beta"] != "always" || modes["gamma"] != "version" || modes["delta"] != "until" {
		t.Fatalf("modes %v", modes)
	}
	if num(m, "activeCount") != 1 || num(m, "ignoredCount") != 3 {
		t.Fatal("counts")
	}
	if r.snapshots() != 0 {
		t.Fatal("updates check should not take a snapshot")
	}

	// an expired pause shows again
	must(t, st.Set("delta", updates.Rule{Mode: updates.Until, Until: fixedNow.Add(-time.Hour)}))
	m = r.ok("winforge_updates_check", `{}`)
	if len(list(m, "active")) != 2 {
		t.Fatalf("expired pause must be active: %v", ids(list(m, "active"), "id"))
	}

	// a damaged rules file is an error, never "no rules"
	must(t, os.WriteFile(st.Path, []byte("{not json"), 0o644))
	r.fails("winforge_updates_check", `{}`, "ignored-updates")

	r.upErr = system.ErrNoWinget
	r.fails("winforge_updates_check", `{}`, "winget")
}

func TestProfilesListAndGet(t *testing.T) {
	r := newRig(t)
	mine := catalog.Profile{ID: "mine", Name: "My set", Kind: "custom", Apps: []catalog.ProfileApp{{ID: "alpha"}}}
	must(t, (&profile.Store{Dir: filepath.Join(r.appData, "profiles")}).Save(mine))

	m := r.ok("winforge_profiles_list", `{}`)
	if num(m, "total") != 3 {
		t.Fatalf("profiles %v", ids(list(m, "profiles"), "id"))
	}
	src := map[string]string{}
	for _, p := range list(m, "profiles") {
		src[str(obj(p), "id")] = str(obj(p), "source")
	}
	if src["starter"] != "builtin" || src["mine"] != "user" {
		t.Fatalf("sources %v", src)
	}
	if r.snapshots() != 0 {
		t.Fatal("listing profiles must not scan")
	}
	m = r.ok("winforge_profiles_list", `{"limit":1}`)
	if len(list(m, "profiles")) != 1 || !boolean(m, "truncated") {
		t.Fatal("limit")
	}

	m = r.ok("winforge_profile_get", `{"id":"starter"}`)
	if got := ids(list(m, "apps"), "id"); len(got) != 2 || got[0] != "alpha" || got[1] != "gamma" {
		t.Fatalf("apps %v", got)
	}
	if got := list(m, "recipes"); len(got) != 1 || got[0] != "rec-a" {
		t.Fatalf("recipes %v", got)
	}
	if str(obj(list(m, "apps")[1]), "version") != "1.2.3" {
		t.Fatal("pinned version")
	}
	if _, has := m["drift"]; has || r.snapshots() != 0 {
		t.Fatal("no baseline: no drift and no scan")
	}
	// requirements come first
	m = r.ok("winforge_profile_get", `{"id":"adminpack"}`)
	if got := ids(list(m, "apps"), "id"); len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("requires first: %v", got)
	}
	m = r.ok("winforge_profile_get", `{"id":"mine"}`)
	if str(m, "source") != "user" {
		t.Fatal("user profile")
	}
	r.fails("winforge_profile_get", `{"id":"ghost"}`, "no profile")

	// with a baseline the drift is computed from one scan
	base := map[string]string{"alpha": "1.0", "gamma": "1.2.3"}
	must(t, (&drift.Store{Path: filepath.Join(r.appData, "applied.json")}).Set("starter", base, fixedNow.Add(-72*time.Hour)))
	r.install("alpha", "1.1", "delta", "1.0") // gamma removed, alpha updated, delta added
	m = r.ok("winforge_profile_get", `{"id":"starter"}`)
	d := obj(m["drift"])
	if d == nil || r.snapshots() != 1 || num(d, "count") != 3 {
		t.Fatalf("drift %v (snaps %d)", d, r.snapshots())
	}
	if ids(list(d, "removed"), "id")[0] != "gamma" || ids(list(d, "added"), "id")[0] != "delta" || str(obj(list(d, "updated")[0]), "to") != "1.1" {
		t.Fatalf("drift detail %v", d)
	}
	if str(d, "at") != "2026-11-14T12:00:00Z" {
		t.Fatalf("at %v", d["at"])
	}
}

func TestInstallPlanIsAPureRead(t *testing.T) {
	r := newRig(t)
	r.install("alpha", "1.0")
	m := r.ok("winforge_install_plan", `{"profileId":"starter"}`)
	steps := list(m, "steps")
	if got := ids(steps, "id"); len(got) != 2 || got[0] != "gamma" || got[1] != "rec-a" {
		t.Fatalf("steps %v", got)
	}
	if str(obj(steps[0]), "kind") != "app" || str(obj(steps[0]), "version") != "1.2.3" || str(obj(steps[1]), "kind") != "recipe" {
		t.Fatalf("steps %v", steps)
	}
	if al := list(m, "alreadyInstalled"); len(al) != 1 || al[0] != "alpha" {
		t.Fatalf("already %v", al)
	}
	if boolean(m, "needsAdmin") {
		t.Fatal("no admin in starter")
	}

	m = r.ok("winforge_install_plan", `{"appIds":["beta"]}`)
	if !boolean(m, "needsAdmin") || len(list(m, "steps")) != 1 || str(obj(list(m, "steps")[0]), "id") != "beta" {
		t.Fatalf("admin plan %v", m)
	}
	m = r.ok("winforge_install_plan", `{"appIds":["delta"]}`)
	if got := ids(list(m, "steps"), "id"); len(got) != 2 || got[1] != "rec-d" {
		t.Fatalf("app recipes %v", got)
	}
	if r.plans.count() != 0 || len(r.exec.calls) != 0 {
		t.Fatal("a read stored a plan or ran a command")
	}
}

func TestEOLReport(t *testing.T) {
	r := newRig(t)
	r.install("python-3-10", "3.10.5", "alpha", "1.0")
	m := r.ok("winforge_eol_report", `{}`)
	if str(m, "asOf") == "" || boolean(m, "stale") {
		t.Fatalf("asOf/stale %v", m)
	}
	f := list(m, "findings")
	if len(f) != 1 || str(obj(f[0]), "appId") != "python-3-10" || str(obj(f[0]), "status") != "ended" {
		t.Fatalf("findings %v", f)
	}
	// a clock far in the future makes the table stale
	r.env.Now = func() time.Time { return fixedNow.AddDate(3, 0, 0) }
	m = r.ok("winforge_eol_report", `{}`)
	if !boolean(m, "stale") || num(m, "ageDays") < 365 {
		t.Fatalf("stale %v", m)
	}
}

func TestHistory(t *testing.T) {
	r := newRig(t)
	m := r.ok("winforge_history", `{}`)
	if num(m, "total") != 0 || len(list(m, "entries")) != 0 {
		t.Fatalf("empty history %v", m)
	}
	st := &history.FileStore{Path: filepath.Join(r.appData, "history.json")}
	var es []history.Entry
	for i := 0; i < 60; i++ {
		app, res := "alpha", history.OK
		if i%3 == 0 {
			app = "gamma"
		}
		if i%10 == 0 {
			res = history.Failed
		}
		es = append(es, history.Entry{ID: fmt.Sprintf("e%02d", i), App: app, Name: app, Action: history.Update, From: "1", To: "2", At: fixedNow.Add(time.Duration(i) * time.Minute), Result: res, Source: history.FromUpdate})
	}
	must(t, st.Add(es...))
	m = r.ok("winforge_history", `{}`)
	e := list(m, "entries")
	if num(m, "total") != 60 || len(e) != 50 || !boolean(m, "truncated") || str(obj(e[0]), "id") != "e59" {
		t.Fatalf("history %d entries, total %v, first %v", len(e), m["total"], obj(e[0])["id"])
	}
	m = r.ok("winforge_history", `{"app":"gamma","result":"failed","limit":5}`)
	for _, x := range list(m, "entries") {
		if str(obj(x), "app") != "gamma" || str(obj(x), "result") != "failed" {
			t.Fatalf("filter %v", x)
		}
	}
	if num(m, "total") != 2 { // i = 0 and 30 are both gamma and failed
		t.Fatalf("filtered total %v", m["total"])
	}
	if r.snapshots() != 0 {
		t.Fatal("history must not scan the PC")
	}
	must(t, os.WriteFile(st.Path, []byte("{broken"), 0o644))
	// a damaged history is moved aside by the store and reads as empty; it is never an error for the model
	m = r.ok("winforge_history", `{}`)
	if num(m, "total") != 0 {
		t.Fatal("damaged history")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

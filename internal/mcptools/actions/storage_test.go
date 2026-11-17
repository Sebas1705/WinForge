package actions

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/storage"
)

func storageBundle(t *testing.T) (*bundle, *harness) {
	b := newDeps(t)
	b.storage.cats = []storage.Category{
		{ID: "temp-user", Risk: storage.Safe, Available: true, Bytes: 5 << 20, Files: 12},
		{ID: "temp-windows", Risk: storage.Safe, NeedsAdmin: true, Available: true, Bytes: 7 << 20, Files: 4},
		{ID: "docker-build-cache", Risk: storage.Review, Available: true, Bytes: 3 << 30, Files: 9},
		{ID: "docker-dangling", Risk: storage.Review, Available: false},
		{ID: "windows-old", Risk: storage.Review, NeedsAdmin: true, Available: true, Bytes: 20 << 30, Files: 100},
		{ID: "recycle-bin", Risk: storage.Safe, Available: true, Bytes: 1 << 20, Files: 2},
	}
	return b, b.env(t, nil)
}

func TestStorageCleanPrepareListsEachCategoryFromAFreshScan(t *testing.T) {
	b, h := storageBundle(t)
	p := h.prepare("winforge_prepare_storage_clean", `{"categoryIds":["temp-user","docker-build-cache","recycle-bin","windows-old"]}`)
	if p.Kind != "storage-clean" || !p.NeedsAdmin {
		t.Fatalf("%+v", p)
	}
	sum := p.Summary.(storageSummary)
	if len(sum.Categories) != 4 || sum.TotalBytes != 5<<20+3<<30+1<<20+20<<30 {
		t.Fatalf("%+v", sum)
	}
	got := map[string]storageItem{}
	for _, c := range sum.Categories {
		got[c.ID] = c
	}
	if c := got["temp-user"]; c.Bytes != 5<<20 || c.Files != 12 || c.Risk != "safe" || c.How != "folders" || len(c.Folders) != 1 || strings.Contains(c.Folders[0], planted) {
		t.Errorf("temp-user: %+v", c)
	}
	// The exact commands, the way storage.Clean runs them.
	if c := got["docker-build-cache"]; !reflect.DeepEqual(c.Commands, []string{"docker builder prune --all --force"}) || c.Risk != "review" {
		t.Errorf("docker: %+v", c)
	}
	if c := got["windows-old"]; len(c.Commands) != 3 || !strings.HasPrefix(c.Commands[0], "takeown.exe /F ") || !c.NeedsAdmin {
		t.Errorf("windows-old: %+v", c)
	}
	if len(sum.Warnings) != 2 || !strings.Contains(sum.Undo, "Not undoable") {
		t.Errorf("warnings %v undo %q", sum.Warnings, sum.Undo)
	}
	// Prepare read the disk once and wrote nothing.
	if !reflect.DeepEqual(b.storage.reads, []string{"scan"}) || len(b.storage.writes) != 0 {
		t.Errorf("reads %v writes %v", b.storage.reads, b.storage.writes)
	}
}

func TestStorageCleanRunDoesExactlyWhatTheSummarySaid(t *testing.T) {
	b, h := storageBundle(t)
	b.storage.cleanResult = storage.Result{FreedBytes: 6 << 20, Files: 14, Failed: 1, Outcomes: []storage.Outcome{
		{ID: "temp-user", FreedBytes: 5 << 20, Files: 12, Failed: 1},
		{ID: "recycle-bin", FreedBytes: 1 << 20, Files: 2, Error: `could not open C:\Users\` + planted + `\x`},
	}}
	p := h.prepare("winforge_prepare_storage_clean", `{"categoryIds":["temp-user","recycle-bin","temp-user"]}`)
	if len(b.storage.writes) != 0 {
		t.Fatal("prepare wrote")
	}
	res := h.ran(p.PlanID)
	if res.IsError {
		t.Fatalf("%s", res.Text)
	}
	if !reflect.DeepEqual(b.storage.cleaned, [][]string{{"temp-user", "recycle-bin"}}) {
		t.Fatalf("cleaned %v", b.storage.cleaned)
	}
	r := res.Structured.(storageResult)
	if r.FreedBytes != 6<<20 || r.Files != 14 || r.Failed != 1 || len(r.Categories) != 2 || r.Categories[1].Error == "" || r.Undo == "" || r.Cancelled {
		t.Errorf("%+v", r)
	}
	if len(h.prog) == 0 {
		t.Error("progress was not reported")
	}
	for _, m := range h.prog {
		assertNoUser(t, m)
	}
	// A plan is used once.
	if _, err := h.store.Take(p.PlanID); err == nil {
		t.Error("the plan must be single use")
	}
}

func TestStorageCleanValidation(t *testing.T) {
	b, h := storageBundle(t)
	cases := map[string]string{
		`{"categoryIds":[]}`:                      "",
		`{"categoryIds":["nope"]}`:                "not a storage category",
		`{"categoryIds":["../../Windows"]}`:       "not a storage category",
		`{"categoryIds":["docker-dangling"]}`:     "not present",
		`{}`:                                      "",
		`{"categoryIds":"temp-user"}`:             "",
		`{"categoryIds":["temp-user"],"force":1}`: "",
	}
	for args, want := range cases {
		if got := h.refused("winforge_prepare_storage_clean", args); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", args, got, want)
		}
	}
	// A category the scan does not list at all.
	b.storage.cats = b.storage.cats[:1]
	h.refused("winforge_prepare_storage_clean", `{"categoryIds":["recycle-bin"]}`)
	if len(b.storage.writes) != 0 {
		t.Fatal("a refusal must not write")
	}
}

func TestStorageCleanRefusesAdminCategoriesWhenNotElevated(t *testing.T) {
	b, h := storageBundle(t)
	*b.admin = false
	got := h.refused("winforge_prepare_storage_clean", `{"categoryIds":["temp-user","temp-windows","windows-old"]}`)
	if !strings.Contains(got, "temp-windows") || !strings.Contains(got, "windows-old") || !strings.Contains(got, "administrator") {
		t.Errorf("%s", got)
	}
	// What needs no admin is still fine.
	h.prepare("winforge_prepare_storage_clean", `{"categoryIds":["temp-user"]}`)
}

func TestStorageCleanRefusesAtRunWhenTheWorldChanged(t *testing.T) {
	cases := map[string]func(b *bundle){
		"category vanished": func(b *bundle) { b.storage.cats[0].Available = false },
		"grew a lot":        func(b *bundle) { b.storage.cats[0].Bytes = 2 << 30 },
		"lost admin":        func(b *bundle) { *b.admin = false },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			b, h := storageBundle(t)
			id := "temp-user"
			if name == "lost admin" {
				id = "temp-windows"
			}
			p := h.prepare("winforge_prepare_storage_clean", `{"categoryIds":["`+id+`"]}`)
			change(b)
			got := h.ranRefused(p.PlanID)
			if !strings.Contains(got, "Nothing was changed") || len(b.storage.writes) != 0 {
				t.Errorf("%s / writes %v", got, b.storage.writes)
			}
		})
	}
}

func TestStorageCleanSmallGrowthIsTolerated(t *testing.T) {
	b, h := storageBundle(t)
	p := h.prepare("winforge_prepare_storage_clean", `{"categoryIds":["temp-user"]}`)
	b.storage.cats[0].Bytes += 10 << 20 // temp files come and go
	if res := h.ran(p.PlanID); res.IsError || len(b.storage.writes) != 1 {
		t.Fatalf("%+v %v", res, b.storage.writes)
	}
}

func TestStorageCleanCancelled(t *testing.T) {
	b, h := storageBundle(t)
	p := h.prepare("winforge_prepare_storage_clean", `{"categoryIds":["temp-user"]}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.runCtx(ctx, p.PlanID); err == nil {
		t.Fatal("a cancelled run must return the context error")
	}
	if len(b.storage.writes) != 0 {
		t.Fatal("nothing may be cleaned")
	}
	// Cancelled while the plan re-measures: still nothing is cleaned.
	p = h.prepare("winforge_prepare_storage_clean", `{"categoryIds":["temp-user"]}`)
	ctx, cancel = context.WithCancel(context.Background())
	b.storage.onScan = cancel
	if _, err := h.runCtx(ctx, p.PlanID); err == nil || len(b.storage.writes) != 0 {
		t.Fatalf("%v %v", err, b.storage.writes)
	}
}

func TestStorageCleanStopsEarlyWhenCancelledMidWay(t *testing.T) {
	b, h := storageBundle(t)
	b.storage.cleanResult = storage.Result{Outcomes: []storage.Outcome{{ID: "temp-user", FreedBytes: 1, Files: 1}}, FreedBytes: 1, Files: 1}
	p := h.prepare("winforge_prepare_storage_clean", `{"categoryIds":["temp-user","recycle-bin"]}`)
	res := h.ran(p.PlanID)
	r := res.Structured.(storageResult)
	if !r.Cancelled || !strings.Contains(r.Note, "1 of 2") {
		t.Errorf("%+v", r)
	}
}

func TestStorageCleanAllFailedIsAnError(t *testing.T) {
	b, h := storageBundle(t)
	b.storage.cleanResult = storage.Result{Outcomes: []storage.Outcome{{ID: "recycle-bin", Error: "boom"}}}
	p := h.prepare("winforge_prepare_storage_clean", `{"categoryIds":["recycle-bin"]}`)
	if res := h.ran(p.PlanID); !res.IsError {
		t.Fatal("nothing worked, so it is an error result")
	}
}

func TestStorageCleanSchemaListsOnlyTheFixedCategories(t *testing.T) {
	_, h := storageBundle(t)
	schema := string(h.tools["winforge_prepare_storage_clean"].InputSchema)
	for _, s := range storage.Specs {
		if !strings.Contains(schema, `"`+s.ID+`"`) {
			t.Errorf("schema lacks %s", s.ID)
		}
	}
}

// ---- project junk ----

func junkBundle(t *testing.T) (*bundle, *harness) {
	b := newDeps(t)
	touched := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	b.storage.junk = []storage.ProjectJunk{
		{Path: "/home/" + planted + "/code/a/node_modules", Project: "a", ProjectPath: "/home/" + planted + "/code/a", Kind: "node_modules", SizeBytes: 800 << 20, Files: 9000, LastTouched: touched},
		{Path: "/home/" + planted + "/code/b/target", Project: "b", ProjectPath: "/home/" + planted + "/code/b", Kind: "cargo", SizeBytes: 2 << 30, Files: 300, LastTouched: touched},
	}
	return b, b.env(t, nil)
}

func TestProjectJunkPrepareOnlyAcceptsWhatAFreshSearchLists(t *testing.T) {
	b, h := junkBundle(t)
	p := h.prepare("winforge_prepare_remove_project_junk", `{"paths":["/home/sebas/code/a/node_modules"],"roots":["/home/sebas/code"],"minDays":60}`)
	if p.Kind != "projects-remove" || p.NeedsAdmin {
		t.Fatalf("%+v", p)
	}
	sum := p.Summary.(junkSummary)
	if len(sum.Folders) != 1 || sum.Folders[0].Project != "a" || sum.Folders[0].Files != 9000 || sum.Folders[0].LastTouched != "2026-01-02" {
		t.Fatalf("%+v", sum)
	}
	if !strings.Contains(strings.Join(sum.Warnings, " "), "permanent") || !strings.Contains(sum.Headline, "PERMANENTLY") {
		t.Errorf("deletion must be called permanent: %+v", sum)
	}
	// It searched with the roots and the age given and wrote nothing.
	if len(b.storage.writes) != 0 || !reflect.DeepEqual(b.storage.junkRoots, [][]string{{"/home/sebas/code"}}) || b.storage.junkAge[0] != 60*24*time.Hour {
		t.Errorf("writes %v roots %v age %v", b.storage.writes, b.storage.junkRoots, b.storage.junkAge)
	}
}

func TestProjectJunkValidation(t *testing.T) {
	_, h := junkBundle(t)
	cases := map[string]string{
		`{"paths":["/home/sebas/code/zzz/node_modules"],"roots":["/home/sebas/code"]}`: "not listed",
		`{"paths":["/etc"],"roots":["/home/sebas/code"]}`:                              "not listed",
		`{"paths":["/home/sebas/code/a/node_modules","/etc"],"roots":["/r"]}`:          "not listed",
		`{"paths":[],"roots":["/r"]}`:                                                  "",
		`{"paths":["/x"],"roots":[]}`:                                                  "",
		`{"paths":["/x"]}`:                                                             "",
		`{"paths":["/x"],"roots":["/r"],"minDays":0}`:                                  "minDays",
		`{"paths":["/x"],"roots":["/r"],"minDays":99999}`:                              "minDays",
	}
	for args, want := range cases {
		if got := h.refused("winforge_prepare_remove_project_junk", args); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", args, got, want)
		}
	}
}

func TestProjectJunkAcceptsTheRedactedPathAModelRead(t *testing.T) {
	b, h := junkBundle(t)
	// The server redacts what a read tool returns, so the model sends back <user>; the real listed path is what runs.
	p := h.prepare("winforge_prepare_remove_project_junk", `{"paths":["/home/<user>/code/b/target"],"roots":["/home/sebas/code"]}`)
	h.ran(p.PlanID)
	if !reflect.DeepEqual(b.storage.removed, [][]string{{"/home/sebas/code/b/target"}}) {
		t.Fatalf("removed %v", b.storage.removed)
	}
}

func TestProjectJunkRunRemovesExactlyTheListedPathsAndRedactsTheResult(t *testing.T) {
	b, h := junkBundle(t)
	b.storage.removeRes = storage.ProjectResult{FreedBytes: 800 << 20, Removed: 1, Files: 9000,
		Failed: []string{"/home/sebas/code/b/target"}, Reasons: []string{"the project was touched recently"}}
	p := h.prepare("winforge_prepare_remove_project_junk", `{"paths":["/home/sebas/code/a/node_modules","/HOME/sebas/code/b/target","/home/sebas/code/a/node_modules"],"roots":["/home/sebas/code"]}`)
	if len(b.storage.writes) != 0 {
		t.Fatal("prepare wrote")
	}
	res := h.ran(p.PlanID)
	if res.IsError {
		t.Fatalf("%s", res.Text)
	}
	want := [][]string{{"/home/sebas/code/a/node_modules", "/home/sebas/code/b/target"}}
	if !reflect.DeepEqual(b.storage.removed, want) {
		t.Fatalf("removed %v, want %v", b.storage.removed, want)
	}
	r := res.Structured.(junkResult)
	if r.Removed != 1 || len(r.Failed) != 1 || len(r.Reasons) != 1 || !strings.Contains(r.Undo, "Not undoable") {
		t.Errorf("%+v", r)
	}
	// It searched again at run time (once to prepare, once to check).
	if n := len(b.storage.junkRoots); n != 2 {
		t.Errorf("searches: %d", n)
	}
}

func TestProjectJunkRefusesAtRunWhenAFolderIsNoLongerListed(t *testing.T) {
	b, h := junkBundle(t)
	p := h.prepare("winforge_prepare_remove_project_junk", `{"paths":["/home/sebas/code/a/node_modules","/home/sebas/code/b/target"],"roots":["/home/sebas/code"]}`)
	b.storage.junk = b.storage.junk[:1] // project b was touched, so it is no longer listed
	got := h.ranRefused(p.PlanID)
	if !strings.Contains(got, "no longer listed") || !strings.Contains(got, "Nothing was changed") || len(b.storage.writes) != 0 {
		t.Errorf("%s %v", got, b.storage.writes)
	}
}

func TestProjectJunkSearchErrorIsARefusalAndCancelStopsRemoval(t *testing.T) {
	b, h := junkBundle(t)
	b.storage.junkErr = context.DeadlineExceeded
	h.refused("winforge_prepare_remove_project_junk", `{"paths":["/x"],"roots":["/home/sebas/code"]}`)
	b.storage.junkErr = nil
	p := h.prepare("winforge_prepare_remove_project_junk", `{"paths":["/home/sebas/code/a/node_modules"],"roots":["/home/sebas/code"]}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.runCtx(ctx, p.PlanID); err == nil || len(b.storage.writes) != 0 {
		t.Fatalf("%v %v", err, b.storage.writes)
	}
}

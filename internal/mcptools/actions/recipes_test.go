package actions

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/winfeatures"
)

// No recipe is ever run for real: the executor is a fake that records which recipe each call would run.

type featureWorld struct {
	list []winfeatures.Feature
}

func (w *featureWorld) read() []winfeatures.Feature {
	return append([]winfeatures.Feature(nil), w.list...)
}
func (w *featureWorld) set(id, state string) {
	for i := range w.list {
		if w.list[i].ID == id {
			w.list[i].State = state
		}
	}
}

// pending says the feature is on but waits for a restart.
func (w *featureWorld) pending(id string) {
	for i := range w.list {
		if w.list[i].ID == id {
			w.list[i].State, w.list[i].RestartPending = winfeatures.On, true
		}
	}
}

// without is a copy of the catalog that lacks one recipe.
func without(c *catalog.Catalog, id string) *catalog.Catalog {
	out := &catalog.Catalog{Recipes: map[string]*catalog.Recipe{}}
	for k, v := range c.Recipes {
		if k != id {
			out.Recipes[k] = v
		}
	}
	return out
}

func newFeatureWorld() *featureWorld {
	w := &featureWorld{}
	for id, st := range map[string]string{"wsl": winfeatures.Off, "vmp": winfeatures.On, "hyperv": winfeatures.Unavailable, "sandbox": winfeatures.Unknown, "netfx3": winfeatures.Off} {
		w.list = append(w.list, winfeatures.Feature{ID: id, State: st, Enable: "feature-" + id + "-on", Disable: "feature-" + id + "-off"})
	}
	return w
}

func featureHarness(t *testing.T) (*bundle, *harness, *fakeExec, *featureWorld) {
	b := newDeps(t)
	w := newFeatureWorld()
	b.Features = w.read
	ex := &fakeExec{codes: map[string]int{}}
	return b, b.env(t, ex), ex, w
}

func TestFeaturePrepareBuildsTheRestorePointPlanAndRunsNothing(t *testing.T) {
	_, h, ex, _ := featureHarness(t)
	p := h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	if p.Kind != "feature" || !p.NeedsAdmin {
		t.Fatalf("%+v", p)
	}
	sum := p.Summary.(recipeSummary)
	var ids []string
	for _, s := range sum.Steps {
		ids = append(ids, s.ID)
	}
	if !reflect.DeepEqual(ids, []string{"repair-restore-point", "feature-wsl-on"}) || !sum.RestorePoint || !sum.RestartMayBe || sum.From != "off" || sum.To != "on" {
		t.Fatalf("%+v", sum)
	}
	if !strings.Contains(sum.Undo, "enabled=false") {
		t.Errorf("undo: %s", sum.Undo)
	}
	if n := len(ex.recipes()); n != 0 {
		t.Fatalf("prepare ran %d recipes", n)
	}
}

func TestFeatureRunExecutesExactlyTheSummarySteps(t *testing.T) {
	_, h, ex, _ := featureHarness(t)
	ex.codes["feature-wsl-on"] = 3010 // Windows needs a restart
	p := h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	res := h.ran(p.PlanID)
	if res.IsError {
		t.Fatalf("%s", res.Text)
	}
	if got := ex.recipes(); !reflect.DeepEqual(got, []string{"repair-restore-point", "feature-wsl-on"}) {
		t.Fatalf("ran %v", got)
	}
	r := res.Structured.(recipeResult)
	if !r.RestartNeeded || len(r.Steps) != 2 || r.Steps[0].Status != "ok" || r.Steps[1].Status != "ok" || !r.Steps[1].Reboot || len(r.Failed) != 0 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.Note, "restart") || !strings.Contains(r.Undo, "opposite") {
		t.Errorf("%+v", r)
	}
	if len(r.Steps[1].Output) == 0 {
		t.Error("the last lines of output are kept")
	}
	for _, m := range h.prog {
		assertNoUser(t, m)
	}
	if len(h.prog) == 0 {
		t.Error("progress was not reported")
	}
}

func TestFeatureFailureIsReported(t *testing.T) {
	_, h, ex, _ := featureHarness(t)
	ex.codes["feature-wsl-on"] = 1
	p := h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	res := h.ran(p.PlanID)
	r := res.Structured.(recipeResult)
	if res.IsError || !reflect.DeepEqual(r.Failed, []string{"feature-wsl-on"}) || r.Steps[1].Status != "failed" || r.Steps[0].Status != "ok" {
		t.Fatalf("%+v", r)
	}
	ex.codes["repair-restore-point"] = 1
	p = h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	if res := h.ran(p.PlanID); !res.IsError {
		t.Error("when nothing worked the result is an error")
	}
}

func TestFeatureValidation(t *testing.T) {
	b, h, ex, _ := featureHarness(t)
	cases := map[string]string{
		`{"id":"vmp","enabled":true}`:       "already on",
		`{"id":"wsl","enabled":false}`:      "already off",
		`{"id":"hyperv","enabled":true}`:    "not available",
		`{"id":"sandbox","enabled":true}`:   "could not be read",
		`{"id":"docker","enabled":true}`:    "not a known",
		`{"id":"wsl"}`:                      "required",
		`{"enabled":true}`:                  "not a known",
		`{"id":"wsl","enabled":true,"x":1}`: "",
	}
	for args, want := range cases {
		if got := h.refused("winforge_prepare_feature", args); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", args, got, want)
		}
	}
	*b.admin = false
	if got := h.refused("winforge_prepare_feature", `{"id":"wsl","enabled":true}`); !strings.Contains(got, "administrator") {
		t.Errorf("%s", got)
	}
	if len(ex.recipes()) != 0 {
		t.Fatal("nothing may run")
	}
}

func TestFeatureAlreadyPendingRestartIsExplained(t *testing.T) {
	b, h, _, w := featureHarness(t)
	w.pending("wsl")
	b.Features = w.read
	h = b.env(t, &fakeExec{})
	if got := h.refused("winforge_prepare_feature", `{"id":"wsl","enabled":true}`); !strings.Contains(got, "restart") {
		t.Errorf("%s", got)
	}
}

func TestFeatureRunRefusesWhenTheWorldChanged(t *testing.T) {
	b, h, ex, w := featureHarness(t)
	// The feature was turned on by somebody else.
	p := h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	w.set("wsl", winfeatures.On)
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "no longer") || len(ex.recipes()) != 0 {
		t.Errorf("%s", got)
	}
	// The catalog lost the recipe.
	w.set("wsl", winfeatures.Off)
	p = h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	cat := h.env.Catalog
	h.env.Catalog = func() (*catalog.Catalog, error) {
		c, _ := cat()
		c = without(c, "feature-wsl-on")
		return c, nil
	}
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "differ") || len(ex.recipes()) != 0 {
		t.Errorf("%s", got)
	}
	h.env.Catalog = cat
	// Lost admin.
	p = h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	*b.admin = false
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "administrator") || len(ex.recipes()) != 0 {
		t.Errorf("%s", got)
	}
}

func TestFeatureRunRefusesWhenAnotherRunHoldsTheLock(t *testing.T) {
	_, h, ex, _ := featureHarness(t)
	p := h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	path, _ := h.env.LockPath()
	release, err := sched.Lock{Path: path}.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "already in progress") || len(ex.recipes()) != 0 {
		t.Errorf("%s", got)
	}
	release()
	// The lock is released after a run, so a second plan can run.
	p = h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	h.ran(p.PlanID)
	if _, err := os.Stat(path); err == nil {
		t.Error("the lock must be released after the run")
	}
}

func TestFeatureCancelStopsTheRemainingSteps(t *testing.T) {
	_, h, ex, _ := featureHarness(t)
	p := h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	ctx, cancel := context.WithCancel(context.Background())
	ex.onRun = func(id string) {
		if id == "repair-restore-point" {
			cancel()
		}
	}
	res, err := h.runCtx(ctx, p.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	r := res.Structured.(recipeResult)
	if got := ex.recipes(); !reflect.DeepEqual(got, []string{"repair-restore-point"}) {
		t.Fatalf("ran %v", got)
	}
	if !r.Cancelled || r.Steps[1].Status != "skipped" {
		t.Errorf("%+v", r)
	}
	// Cancelled before anything starts.
	p = h.prepare("winforge_prepare_feature", `{"id":"wsl","enabled":true}`)
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := h.runCtx(ctx, p.PlanID); err == nil || len(ex.recipes()) != 1 {
		t.Fatalf("%v %v", err, ex.recipes())
	}
}

// ---- optimize ----

func TestOptimizePrepareAndRun(t *testing.T) {
	b, h, ex, _ := featureHarness(t)
	p := h.prepare("winforge_prepare_optimize", `{"recipeIds":["optimize-trim","optimize-hibernate-off"]}`)
	if p.Kind != "optimize" || !p.NeedsAdmin {
		t.Fatalf("%+v", p)
	}
	sum := p.Summary.(recipeSummary)
	if len(sum.Steps) != 2 || sum.RestorePoint || len(sum.Warnings) != 1 || !strings.Contains(sum.Undo, "powercfg /hibernate on") {
		t.Fatalf("%+v", sum)
	}
	if len(ex.recipes()) != 0 {
		t.Fatal("prepare ran a recipe")
	}
	res := h.ran(p.PlanID)
	if got := ex.recipes(); !reflect.DeepEqual(got, []string{"optimize-trim", "optimize-hibernate-off"}) {
		t.Fatalf("ran %v", got)
	}
	if r := res.Structured.(recipeResult); res.IsError || len(r.Failed) != 0 || !strings.Contains(r.Undo, "powercfg") {
		t.Fatalf("%+v", r)
	}
	_ = b
}

func TestOptimizeOnlyAcceptsOptimizeRecipes(t *testing.T) {
	b, h, ex, _ := featureHarness(t)
	cases := map[string]string{
		`{"recipeIds":["repair-windows-files"]}`:                 "not an optimization",
		`{"recipeIds":["feature-wsl-on"]}`:                       "not an optimization",
		`{"recipeIds":["optimize-nope"]}`:                        "not an optimization",
		`{"recipeIds":["optimize-trim","repair-restore-point"]}`: "not an optimization",
		`{"recipeIds":[]}`:                                       "",
		`{"recipeIds":["optimize-trim; rm -rf /"]}`:              "not an optimization",
		`{}`: "",
	}
	for args, want := range cases {
		if got := h.refused("winforge_prepare_optimize", args); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", args, got, want)
		}
	}
	*b.admin = false
	if got := h.refused("winforge_prepare_optimize", `{"recipeIds":["optimize-trim"]}`); !strings.Contains(got, "administrator") {
		t.Errorf("%s", got)
	}
	if len(ex.recipes()) != 0 {
		t.Fatal("nothing may run")
	}
}

func TestOptimizeRunNeverRunsAnythingButOptimizeRecipes(t *testing.T) {
	_, h, ex, _ := featureHarness(t)
	p := h.prepare("winforge_prepare_optimize", `{"recipeIds":["optimize-defrag","optimize-defrag"]}`)
	if n := len(p.Summary.(recipeSummary).Steps); n != 1 {
		t.Fatalf("duplicates are dropped: %d steps", n)
	}
	// A catalog that now gives the recipe a different identity (a repair swapped in) cannot sneak through the plan.
	cat := h.env.Catalog
	h.env.Catalog = func() (*catalog.Catalog, error) {
		c, _ := cat()
		c = without(c, "optimize-defrag")
		return c, nil
	}
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "differ") || len(ex.recipes()) != 0 {
		t.Errorf("%s", got)
	}
}

func TestRecipeRunsWithoutAnExecutorAreRefused(t *testing.T) {
	b := newDeps(t)
	b.Features = newFeatureWorld().read
	h := b.env(t, &fakeExec{})
	p := h.prepare("winforge_prepare_optimize", `{"recipeIds":["optimize-trim"]}`)
	h.env.Exec = nil
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "no way to run commands") {
		t.Errorf("%s", got)
	}
}

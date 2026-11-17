package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/winfeatures"
)

// The Windows features and the disk optimizations are recipes of the catalog (feature-<id>-on|off and
// optimize-*), run through install.RepairPlan and install.Run like the GUI does. Only those prefixes are
// ever planned here: RepairPlan alone would also accept repair-* recipes, which belong to the health page.

// ---- winforge_prepare_feature ----

type featureArgs struct {
	ID      string `json:"id"`
	Enabled *bool  `json:"enabled"`
}

func (k *kit) featureTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_feature",
		Title: "Prepare turning a Windows feature on or off",
		Description: "Prepares turning one Windows optional feature on or off: wsl (Windows Subsystem for Linux with Virtual Machine Platform), vmp (Virtual Machine " +
			"Platform), hyperv (Hyper-V), sandbox (Windows Sandbox) or netfx3 (.NET Framework 3.5). It runs WinForge's own feature-<id>-on / feature-<id>-off " +
			"recipe through the normal installer runner, after a System Restore point (Windows allows one per 24 hours; if none can be made the change goes on). " +
			"This changes the Windows image: it needs administrator rights (refused now when WinForge is not elevated), can take minutes and often needs a " +
			"restart to finish, which the result reports (WinForge does not restart the PC). Turning a feature off deletes no distributions, virtual machines or " +
			"virtual disks. A feature already in the asked state, or not available on this Windows edition, is refused. The opposite call undoes it." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"id":{"type":"string","enum":["wsl","vmp","hyperv","sandbox","netfx3"]},"enabled":{"type":"boolean","description":"true to turn the feature on, false to turn it off."}},"required":["id","enabled"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareFeature,
	}
}

func (k *kit) optimizeTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_optimize",
		Title: "Prepare disk optimizations",
		Description: "Prepares running WinForge's disk maintenance recipes: optimize-trim (TRIM on solid-state drives), optimize-defrag (defragment hard drives, can take a long time), " +
			"optimize-component-cleanup (DISM cleanup of old Windows update files, several minutes) and optimize-hibernate-off (turns hibernation and Fast Startup off, " +
			"which frees about as much space as the PC has memory). Only recipe ids starting with optimize- that the catalog has right now are accepted. None " +
			"deletes the person's files. All need administrator rights, so this is refused now when WinForge is not elevated. Each recipe skips itself when there is nothing " +
			"to do. TRIM, defrag and component cleanup need no undo; hibernation can be turned on again with `powercfg /hibernate on` (the plan says so)." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"recipeIds":{"type":"array","items":{"type":"string","pattern":"^optimize-[a-z0-9-]+$","maxLength":64},"minItems":1,"maxItems":8,"uniqueItems":true}},"required":["recipeIds"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareOptimize,
	}
}

type recipeStep struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Description string `json:"description"`
	NeedsAdmin  bool   `json:"needsAdmin"`
}

type recipeSummary struct {
	Headline     string       `json:"headline"`
	Feature      string       `json:"feature,omitempty"`
	From         string       `json:"from,omitempty"`
	To           string       `json:"to,omitempty"`
	Steps        []recipeStep `json:"steps"`
	RestorePoint bool         `json:"restorePointFirst"`
	RestartMayBe bool         `json:"restartMayBeNeeded"`
	Warnings     []string     `json:"warnings,omitempty"`
	Undo         string       `json:"undo"`
}

func summarizeSteps(plan install.Plan) []recipeStep {
	out := make([]recipeStep, 0, len(plan.Steps))
	for _, s := range plan.Steps {
		out = append(out, recipeStep{Kind: string(s.Kind), ID: s.ID, Description: s.Name, NeedsAdmin: s.Admin})
	}
	return out
}

func stepIDs(plan install.Plan) []string {
	out := make([]string, len(plan.Steps))
	for i, s := range plan.Steps {
		out[i] = s.ID
	}
	return out
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (k *kit) catalog() (*catalog.Catalog, error) {
	if k.env == nil || k.env.Catalog == nil {
		return nil, errors.New("the catalog is not available")
	}
	return k.env.Catalog()
}

func (k *kit) findFeature(id string) (winfeatures.Feature, bool) {
	for _, f := range k.d.Features() {
		if f.ID == id {
			return f, true
		}
	}
	return winfeatures.Feature{}, false
}

func (k *kit) prepareFeature(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Features == nil {
		return notAvailable("the Windows features list")
	}
	if k.env == nil || k.env.Exec == nil {
		return notAvailable("running recipes")
	}
	var a featureArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	if a.Enabled == nil {
		return refuse("enabled is required")
	}
	enabled := *a.Enabled
	f, ok := k.findFeature(a.ID)
	if !ok {
		return refuse("%q is not a known Windows feature (wsl, vmp, hyperv, sandbox, netfx3)", a.ID)
	}
	switch f.State {
	case winfeatures.Unavailable:
		return refuse("the feature %q is not available on this edition of Windows", f.ID)
	case winfeatures.Unknown:
		return refuse("the state of the feature %q could not be read, so no plan was made", f.ID)
	}
	if (f.State == winfeatures.On) == enabled {
		if f.RestartPending {
			return refuse("the feature %q is already %s, waiting for a restart of Windows to finish; nothing to do", f.ID, f.State)
		}
		return refuse("the feature %q is already %s; nothing to change", f.ID, f.State)
	}
	if !k.elevated() {
		return refuse("turning a Windows feature on or off needs administrator rights and WinForge is not running elevated. Nothing was prepared")
	}
	recipe := f.Disable
	if enabled {
		recipe = f.Enable
	}
	cat, err := k.catalog()
	if err != nil {
		return nil, err
	}
	plan, err := install.RepairPlan(cat, []string{recipe})
	if err != nil {
		return refuse("the recipe for this change is not available: %s", err.Error())
	}
	if len(plan.Steps) == 0 {
		return refuse("there is nothing to run for this change")
	}
	word := map[bool]string{true: "on", false: "off"}[enabled]
	sum := recipeSummary{
		Headline: fmt.Sprintf("Turn the Windows feature %s %s", f.ID, word),
		Feature:  f.ID, From: f.State, To: word, Steps: summarizeSteps(plan), RestorePoint: plan.Steps[0].ID == "repair-restore-point", RestartMayBe: true,
		Warnings: []string{"This changes the Windows image and may need a restart to finish; WinForge does not restart the PC.",
			"It can take several minutes."},
		Undo: fmt.Sprintf("Yes: winforge_prepare_feature with id=%s and enabled=%v (a restart may be needed again). Nothing the person made (distributions, virtual machines, disks) is deleted.", f.ID, !enabled),
	}
	want := stepIDs(plan)
	before := f
	return k.store(c, "feature", sum, plan.NeedsAdmin, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		cur, ok := k.findFeature(before.ID)
		if !ok || cur.State != before.State {
			return refuse("the feature %q is no longer %s (it changed since the plan was made). Nothing was changed; prepare the plan again", before.ID, before.State)
		}
		return k.runRecipes(ctx, c, want, featurePrefix)
	})
}

const optimizeIDs = "optimize-trim, optimize-defrag, optimize-component-cleanup, optimize-hibernate-off"

type optimizeArgs struct {
	RecipeIDs []string `json:"recipeIds"`
}

func (k *kit) prepareOptimize(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.env == nil || k.env.Exec == nil {
		return notAvailable("running recipes")
	}
	var a optimizeArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	ids := dedupe(a.RecipeIDs)
	if len(ids) == 0 {
		return refuse("no recipe was chosen")
	}
	cat, err := k.catalog()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if !strings.HasPrefix(id, "optimize-") || cat.Recipes[id] == nil {
			return refuse("%q is not an optimization recipe of the catalog (known: %s)", id, optimizeIDs)
		}
	}
	plan, err := install.RepairPlan(cat, ids)
	if err != nil {
		return refuse("%s", err.Error())
	}
	if len(plan.Steps) == 0 {
		return refuse("there is nothing to run")
	}
	if plan.NeedsAdmin && !k.elevated() {
		return refuse("the disk optimizations need administrator rights and WinForge is not running elevated. Nothing was prepared")
	}
	var warn []string
	undo := "No undo is needed: TRIM, defragmentation and component cleanup change no settings."
	for _, id := range ids {
		switch id {
		case "optimize-hibernate-off":
			warn = append(warn, "Turning hibernation off also turns Fast Startup off.")
			undo = "Hibernation can be turned on again with `powercfg /hibernate on` in an administrator terminal; the other optimizations need no undo."
		case "optimize-defrag", "optimize-component-cleanup":
			warn = append(warn, id+" can take a long time.")
		}
	}
	sum := recipeSummary{
		Headline: fmt.Sprintf("Run %d disk optimization%s", len(plan.Steps), plural(len(plan.Steps), "", "s")),
		Steps:    summarizeSteps(plan), RestorePoint: false, Warnings: warn, Undo: undo,
	}
	want := stepIDs(plan)
	return k.store(c, "optimize", sum, plan.NeedsAdmin, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		return k.runRecipes(ctx, c, want, "optimize-")
	})
}

type recipeStepResult struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"` // ok, failed, skipped
	Reboot  bool     `json:"restartNeeded,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Error   string   `json:"error,omitempty"`
	Output  []string `json:"lastLines,omitempty"`
	Skipped string   `json:"skippedBecause,omitempty"`
}

type recipeResult struct {
	Steps         []recipeStepResult `json:"steps"`
	Failed        []string           `json:"failed"`
	RestartNeeded bool               `json:"restartNeeded"`
	Cancelled     bool               `json:"cancelled,omitempty"`
	Note          string             `json:"note,omitempty"`
	Undo          string             `json:"undo,omitempty"`
}

// runRecipes rebuilds the plan from the catalog as it is now, refuses when it differs from what was
// approved, takes the shared run lock and runs it with the install runner. onlyPrefix, when set, is a
// last guard: every step must start with it, except the restore point.
func (k *kit) runRecipes(ctx context.Context, c *mcpkit.Call, want []string, onlyPrefix string) (*mcpkit.Result, error) {
	if k.env == nil || k.env.Exec == nil {
		return refuse("no way to run commands is available. Nothing was changed")
	}
	cat, err := k.catalog()
	if err != nil {
		return refuse("the catalog could not be read (%s). Nothing was changed", k.redact(err.Error()))
	}
	var ids []string
	for _, id := range want {
		if id != "repair-restore-point" {
			ids = append(ids, id)
		}
	}
	plan, err := install.RepairPlan(cat, ids)
	if err != nil || !sameIDs(stepIDs(plan), want) {
		return refuse("the recipes differ from the ones the plan listed. Nothing was changed; prepare the plan again")
	}
	for _, s := range plan.Steps {
		if s.Kind != install.StepRecipe || (onlyPrefix != "" && s.ID != "repair-restore-point" && !strings.HasPrefix(s.ID, onlyPrefix)) {
			return refuse("the plan holds a step it should not (%s). Nothing was changed", s.ID)
		}
	}
	if plan.NeedsAdmin && !k.elevated() {
		return refuse("administrator rights are needed and WinForge no longer has them. Nothing was changed")
	}
	if k.env.LockPath != nil {
		if p, err := k.env.LockPath(); err == nil {
			release, lerr := sched.Lock{Path: p}.Acquire()
			if errors.Is(lerr, sched.ErrBusy) {
				return refuse("another WinForge run is already in progress. Nothing was changed; try again when it ends")
			}
			if lerr == nil {
				defer release()
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	res := recipeResult{Steps: []recipeStepResult{}, Failed: []string{}}
	at := map[string]int{}
	for _, s := range plan.Steps {
		at[s.ID] = len(res.Steps)
		res.Steps = append(res.Steps, recipeStepResult{ID: s.ID, Status: "skipped", Skipped: "not reached"})
	}
	total := float64(len(plan.Steps))
	done := 0.0
	failed := install.Run(ctx, cat, plan, k.env.Exec, func(e install.Event) {
		i, ok := at[e.Step.ID]
		if !ok {
			return
		}
		st := &res.Steps[i]
		switch e.Status {
		case install.StatusStart:
			st.Status, st.Skipped = "running", ""
			progress(c, done, total, "Running "+e.Step.ID)
		case install.StatusOutput:
			st.Output = append(st.Output, k.redact(e.Line))
			if len(st.Output) > 6 {
				st.Output = st.Output[len(st.Output)-6:]
			}
		case install.StatusOK:
			st.Status = "ok"
			if e.Reason == install.ReasonReboot {
				st.Reboot, st.Reason = true, e.Reason
				res.RestartNeeded = true
			}
			done++
			progress(c, done, total, "Finished "+e.Step.ID)
		case install.StatusFailed:
			st.Status, st.Reason, st.Error = "failed", e.Reason, k.redact(e.Err)
			done++
			progress(c, done, total, "Failed "+e.Step.ID)
		case install.StatusSkipped:
			st.Status, st.Skipped = "skipped", k.redact(e.Err)
			done++
		}
	})
	res.Failed = nonNilStrings(failed)
	res.Cancelled = ctx.Err() != nil
	allFailed := len(failed) > 0
	for _, s := range res.Steps {
		if s.Status == "ok" {
			allFailed = false
		}
	}
	if res.RestartNeeded {
		res.Note = "Windows needs a restart to finish the change; WinForge has not restarted the PC."
	}
	switch {
	case strings.HasPrefix(plan.Steps[len(plan.Steps)-1].ID, featurePrefix):
		res.Undo = "winforge_prepare_feature with the opposite enabled value undoes it (a restart may be needed again)."
	case contains(want, "optimize-hibernate-off"):
		res.Undo = "Hibernation can be turned on again with `powercfg /hibernate on` in an administrator terminal."
	}
	return outcome(res, allFailed), nil
}

const featurePrefix = "feature-"

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

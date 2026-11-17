package sysapps

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/history"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/planrun"
	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// preparedOut is what every Prepare tool returns. The plan only runs when winforge_apply_plan is called
// with planId, after the person has seen the summary and approved it.
type preparedOut struct {
	PlanID      string `json:"planId"`
	Kind        string `json:"kind"`
	Summary     any    `json:"summary"`
	NeedsAdmin  bool   `json:"needsAdmin"`
	ExpiresAt   string `json:"expiresAt"`
	ApplyWith   string `json:"applyWith"`
	NothingToDo bool   `json:"nothingToDo,omitempty"`
}

const approvalNote = " The plan only runs when winforge_apply_plan is called with the returned planId: show the person the summary and get their explicit approval first."

type installSummary struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	planOut
	Elevated bool     `json:"elevated"`
	Warnings []string `json:"warnings"`
}

type updateStepOut struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Current   string `json:"current"`
	Available string `json:"available"`
	Admin     bool   `json:"admin,omitempty"`
}

type skippedOut struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"` // needs-admin | ignored | no-update
}

type updateSummary struct {
	Kind       string          `json:"kind"`
	Steps      []updateStepOut `json:"steps"`
	NeedsAdmin bool            `json:"needsAdmin"`
	Skipped    []skippedOut    `json:"skipped"`
	Elevated   bool            `json:"elevated"`
	Warnings   []string        `json:"warnings"`
}

type usedByOut struct {
	ID         string   `json:"id"`
	RequiredBy []string `json:"requiredBy"`
}

type uninstallSummary struct {
	Kind string `json:"kind"`
	planOut
	Irreversible         bool        `json:"irreversible"`
	ReviewLeftoversInApp bool        `json:"reviewLeftoversInApp"`
	UsedBy               []usedByOut `json:"usedBy"`
	Elevated             bool        `json:"elevated"`
	Warnings             []string    `json:"warnings"`
}

// store keeps a plan and builds the answer. A plan with nothing to do is not stored.
func store(c *mcpkit.Call, kind string, summary any, plan install.Plan, run func(context.Context, *mcpkit.Call) (*mcpkit.Result, error)) *mcpkit.Result {
	if len(plan.Steps) == 0 {
		return mcpkit.JSON(preparedOut{Kind: kind, Summary: summary, ApplyWith: "winforge_apply_plan", NothingToDo: true})
	}
	if c.Plans == nil {
		return mcpkit.Fail("plans cannot be stored here")
	}
	id, expires := c.Plans.Put(mcpkit.Plan{Kind: kind, Summary: summary, Admin: plan.NeedsAdmin, Run: run})
	return mcpkit.JSON(preparedOut{PlanID: id, Kind: kind, Summary: summary, NeedsAdmin: plan.NeedsAdmin,
		ExpiresAt: expires.UTC().Format(time.RFC3339), ApplyWith: "winforge_apply_plan"})
}

func needsWinget(p install.Plan) bool {
	for _, s := range p.Steps {
		if s.Kind != install.StepRecipe {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------------------------
// winforge_prepare_install

func (r *rt) prepareInstallTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_install",
		Title: "Prepare an install",
		Description: "Builds the plan to install a profile (profileId) or catalog apps (appIds), exactly one of the two, and stores it. " +
			"Returns planId, kind \"install\", expiresAt, needsAdmin and the summary: steps (kind app | recipe, id, name, version, admin) in run order, alreadyInstalled and skippedRecipes. " +
			"It reads the PC's software list once (personal data) and CHANGES NOTHING: it installs nothing and runs no recipe. Unknown ids or an unknown profile are an error and nothing is stored." +
			approvalNote + " Apply refuses when winget is missing, when another WinForge run is in progress, and (for administrator steps) when this server is not elevated.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{"profileId":` + idSchema + `,"appIds":` + idsSchema("catalog app ids to install") + `}}`),
		Class:       mcpkit.Prepare,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a installArgs
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			e := r.env(c)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			ids, err := a.validate(cat)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			s, err := r.scan(ctx, e, cat)
			if err != nil {
				return mcpkit.Fail("could not scan this PC: %s", err.Error()), nil
			}
			built, target, err := buildInstallPlan(e, cat, a.ProfileID, ids, s.installed)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			if s.noWinget && needsWinget(built.plan) {
				return mcpkit.Fail("%s; nothing was stored", system.ErrNoWinget.Error()), nil
			}
			sum := installSummary{Kind: "install", Target: target, planOut: built.out, Elevated: elevated(e), Warnings: warnings(s.warning)}
			return store(c, "install", sum, built.plan, r.execute("install", "install", built.plan, nil)), nil
		},
	}
}

// ---------------------------------------------------------------------------------------------
// winforge_prepare_update

func (r *rt) prepareUpdateTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_update",
		Title: "Prepare updates",
		Description: "Builds the plan to update the given catalog apps (appIds) or every catalog app with an update (all: true), exactly one of the two, and stores it. " +
			"It honors the person's ignore rules exactly like the Updates page's Update all (ignored updates are left out and listed in skipped), and when this server is not elevated " +
			"it leaves out apps whose installer needs administrator rights (skipped, reason needs-admin). Returns planId, kind \"update\", expiresAt, needsAdmin and the summary: " +
			"steps (id, name, current, available), skipped and warnings. It asks winget for the list of updates and reveals which apps are installed (personal data); it CHANGES NOTHING and updates nothing." +
			approvalNote + " Apps outside the catalog are never touched.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{"appIds":` + idsSchema("catalog app ids to update") + `,"all":{"type":"boolean","description":"update every catalog app that has an update"}}}`),
		Class:       mcpkit.Prepare,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct {
				AppIDs []string `json:"appIds"`
				All    bool     `json:"all"`
			}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			switch {
			case a.All && len(a.AppIDs) > 0:
				return mcpkit.Fail("give appIds or all, not both"), nil
			case !a.All && len(a.AppIDs) == 0:
				return mcpkit.Fail("give appIds or all: true"), nil
			}
			e := r.env(c)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			var only []string
			if !a.All {
				ids, err := checkIDs("appIds", a.AppIDs)
				if err == nil {
					err = knownApps(cat, ids)
				}
				if err != nil {
					return mcpkit.Fail("%s", err.Error()), nil
				}
				only = ids
			}
			rules, err := ignoreRules(e)
			if err != nil {
				return mcpkit.Fail("the ignored-updates list cannot be read (%s); nothing was stored so no ignored update is installed by mistake", redactor(e)(err.Error())), nil
			}
			ups, err := r.d.Upgrades(ctx)
			if err != nil {
				return mcpkit.Fail("could not ask winget for updates: %s", redactor(e)(err.Error())), nil
			}
			plan := updates.PlanUpgrades(cat, ups, only, rules, now(e))
			sum := updateSummary{Kind: "update", Steps: []updateStepOut{}, Skipped: []skippedOut{}, Elevated: elevated(e), Warnings: []string{}}

			// What the person's choices (and, for a named list, "no update") left out, so the summary is honest.
			all := install.UpgradePlan(cat, ups, only)
			have := map[string]bool{}
			for _, s := range all.Steps {
				have[s.ID] = true
			}
			kept := map[string]bool{}
			for _, s := range plan.Steps {
				kept[s.ID] = true
			}
			for _, s := range all.Steps {
				if !kept[s.ID] {
					sum.Skipped = append(sum.Skipped, skippedOut{ID: s.ID, Name: s.Name, Reason: "ignored"})
				}
			}
			for _, id := range only {
				if !have[id] {
					sum.Skipped = append(sum.Skipped, skippedOut{ID: id, Name: appName(cat, id), Reason: "no-update"})
				}
			}
			if !elevated(e) {
				steps := plan.Steps[:0:0]
				for _, s := range plan.Steps {
					if s.Admin {
						sum.Skipped = append(sum.Skipped, skippedOut{ID: s.ID, Name: s.Name, Reason: "needs-admin"})
						continue
					}
					steps = append(steps, s)
				}
				plan.Steps, plan.NeedsAdmin = steps, false
			}
			sort.SliceStable(sum.Skipped, func(i, j int) bool { return sum.Skipped[i].ID < sum.Skipped[j].ID })
			for _, s := range plan.Steps {
				cur := ""
				for _, u := range ups {
					if app := cat.Apps[s.ID]; app != nil && u.ID == app.Winget {
						cur = u.Current
					}
				}
				sum.Steps = append(sum.Steps, updateStepOut{ID: s.ID, Name: s.Name, Current: cur, Available: s.Version, Admin: s.Admin})
			}
			sum.NeedsAdmin = plan.NeedsAdmin
			return store(c, "update", sum, plan, r.execute("update", "updates", plan, ups)), nil
		},
	}
}

// ---------------------------------------------------------------------------------------------
// winforge_prepare_uninstall

func (r *rt) prepareUninstallTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_uninstall",
		Title: "Prepare an uninstall",
		Description: "Builds the plan to uninstall catalog apps (appIds) with winget (uninstall --id <id> --exact --silent) and stores it. Only catalog apps that are installed here are accepted; " +
			"an unknown or not installed id is an error and nothing is stored. Returns planId, kind \"uninstall\", expiresAt, needsAdmin and the summary: steps, usedBy " +
			"(other installed apps that require one being removed), irreversible true (WinForge cannot undo an uninstall) and reviewLeftoversInApp true (leftover files and settings can be reviewed in the WinForge app afterwards). " +
			"It reads the PC's software list (personal data) and CHANGES NOTHING: it uninstalls nothing." + approvalNote,
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{"appIds":` + idsSchema("catalog app ids to uninstall") + `},"required":["appIds"]}`),
		Class:       mcpkit.Prepare,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct {
				AppIDs []string `json:"appIds"`
			}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			ids, err := checkIDs("appIds", a.AppIDs)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			e := r.env(c)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			if err := knownApps(cat, ids); err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			s, err := r.scan(ctx, e, cat)
			if err != nil {
				return mcpkit.Fail("could not scan this PC: %s", err.Error()), nil
			}
			var missing []string
			for _, id := range ids {
				if _, ok := s.installed[id]; !ok {
					missing = append(missing, id)
				}
			}
			if len(missing) > 0 {
				return mcpkit.Fail("not installed on this PC: %s (nothing was stored)", strings.Join(missing, ", ")), nil
			}
			if s.noWinget {
				return mcpkit.Fail("%s; nothing was stored", system.ErrNoWinget.Error()), nil
			}
			plan := install.UninstallPlan(cat, ids)
			sum := uninstallSummary{Kind: "uninstall", planOut: toPlanOut(plan), Irreversible: true, ReviewLeftoversInApp: true,
				UsedBy: usedBy(cat, ids, s.installed), Elevated: elevated(e), Warnings: warnings(s.warning)}
			return store(c, "uninstall", sum, plan, r.execute("uninstall", "uninstall", plan, nil)), nil
		},
	}
}

// usedBy lists, for each app being removed, the other installed apps that require it.
func usedBy(cat *catalog.Catalog, ids []string, installed map[string]system.Installed) []usedByOut {
	removing := map[string]bool{}
	for _, id := range ids {
		removing[id] = true
	}
	out := []usedByOut{}
	for _, id := range ids {
		var by []string
		for other := range installed {
			if removing[other] || cat.Apps[other] == nil {
				continue
			}
			for _, req := range cat.Apps[other].Requires {
				if req == id {
					by = append(by, other)
				}
			}
		}
		if len(by) > 0 {
			sort.Strings(by)
			out = append(out, usedByOut{ID: id, RequiredBy: by})
		}
	}
	return out
}

// ---------------------------------------------------------------------------------------------
// running a stored plan

type runOut struct {
	Kind            string         `json:"kind"`
	Refused         string         `json:"refused,omitempty"` // prerequisite | admin | busy
	Stopped         string         `json:"stopped,omitempty"` // cancelled | timeout
	Message         string         `json:"message,omitempty"`
	Counts          map[string]int `json:"counts"`
	RebootRequired  bool           `json:"rebootRequired"`
	Steps           []planrun.Step `json:"steps"`
	HistoryRecorded bool           `json:"historyRecorded"`
	Warnings        []string       `json:"warnings"`
}

// execute is the Run closure of a stored plan: the same checks and runner as `winforge-cli apply`
// (planrun), with the history written like the app does. The plan is a promise about the PC when it was
// made, so the PC is scanned again first for the versions the steps start from.
func (r *rt) execute(kind, title string, plan install.Plan, ups []system.Upgrade) func(context.Context, *mcpkit.Call) (*mcpkit.Result, error) {
	return func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		e := r.env(c)
		red := redactor(e)
		cat, bad := r.catalogOf(e)
		if bad != nil {
			return bad, nil
		}
		for _, s := range plan.Steps {
			known := cat.Apps[s.ID] != nil
			if s.Kind == install.StepRecipe {
				known = cat.Recipes[s.ID] != nil
			}
			if !known {
				return mcpkit.Fail("%s is no longer in the catalog; nothing was run", s.ID), nil
			}
		}
		if e.Exec == nil {
			return mcpkit.Fail("running commands is not available; nothing was run"), nil
		}
		ctx, cancel := context.WithTimeout(ctx, sched.RunTimeout)
		defer cancel()
		s, err := r.scan(ctx, e, cat)
		if err != nil {
			return mcpkit.Fail("could not scan this PC before running: %s; nothing was run", err.Error()), nil
		}
		_, lookErr := r.d.LookPath("winget")
		wingetMissing := lookErr != nil || s.noWinget

		warns := []string{}
		if s.warning != "" && !s.noWinget {
			warns = append(warns, s.warning)
		}
		var rec *history.Recording
		if p, perr := inAppData(e, "history.json"); perr == nil && kind != "uninstall" {
			before := versionsOf(s.installed)
			for _, st := range plan.Steps {
				if st.Kind != install.StepUpgrade {
					continue
				}
				for _, u := range ups {
					if app := cat.Apps[st.ID]; app != nil && u.ID == app.Winget && u.Current != "" {
						before[st.ID] = u.Current
					}
				}
			}
			rec = (&history.Recorder{
				Store:   &history.FileStore{Path: p},
				Now:     func() time.Time { return now(e) },
				OnError: func(err error) { warns = append(warns, "history: "+red(err.Error())) },
			}).Begin(plan, title, before)
		}

		finished := 0
		rep := planrun.Run(ctx, cat, plan, planrun.Options{
			Exec:          e.Exec,
			Elevated:      e.Elevated,
			RequireAdmin:  true, // nobody can answer a UAC prompt here
			LockPath:      e.LockPath,
			Installed:     s.installed,
			WingetMissing: wingetMissing,
			WingetMessage: system.ErrNoWinget.Error(),
			Detect: func() (map[string]system.Installed, error) {
				after, err := r.scan(ctx, e, cat)
				return after.installed, err
			},
			OnWarn: func(m string) { warns = append(warns, red(m)) },
			OnEvent: func(ev install.Event) {
				rec.Observe(ev)
				switch ev.Status {
				case install.StatusOK, install.StatusFailed, install.StatusSkipped:
					finished++
					if c.Progress != nil {
						c.Progress(float64(finished), float64(len(plan.Steps)), string(ev.Step.Kind)+" "+ev.Step.ID+": "+string(ev.Status))
					}
				}
			},
		})
		if rec != nil && rep.After != nil && rec.NeedsDetect() {
			rec.Finish(versionsOf(rep.After))
		}

		out := runOut{Kind: kind, Counts: rep.Counts, RebootRequired: rep.RebootRequired, Steps: make([]planrun.Step, len(rep.Steps)),
			HistoryRecorded: rec != nil && rep.Refused == "", Warnings: warns}
		for i, st := range rep.Steps {
			st.Message = red(st.Message)
			out.Steps[i] = st
		}
		switch {
		case rep.Refused != "":
			out.Refused, out.Message = rep.Refused, red(rep.RefusedMessage)
		case ctx.Err() != nil:
			out.Stopped = "cancelled"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				out.Stopped = "timeout"
			}
			out.Message = "the run was stopped; the remaining steps were not run"
		case len(rep.Failed) > 0:
			out.Message = fmt.Sprintf("%d of %d steps failed", len(rep.Failed), len(plan.Steps))
		}
		res := mcpkit.JSON(out)
		res.IsError = out.Refused != "" || out.Stopped != "" || len(rep.Failed) > 0
		return res, nil
	}
}

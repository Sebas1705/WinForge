// Package install turns a resolved profile into an ordered plan and runs it.
package install

import (
	"fmt"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/system"
	"strings"
)

// StepKind distinguishes installing an app from running a recipe.
type StepKind string

const (
	StepApp    StepKind = "app"
	StepRecipe StepKind = "recipe"
	// StepUpgrade updates an app that is already installed.
	StepUpgrade StepKind = "upgrade"
	// StepUninstall removes an installed app.
	StepUninstall StepKind = "uninstall"
)

// Step is one unit of work.
type Step struct {
	Kind    StepKind `json:"kind"`
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Version string   `json:"version,omitempty"`
	Admin   bool     `json:"admin,omitempty"`
}

// Plan is the ordered work needed to bring the PC to a profile.
type Plan struct {
	Steps []Step `json:"steps"`
	// AlreadyInstalled are profile apps that need nothing.
	AlreadyInstalled []string `json:"alreadyInstalled"`
	// SkippedRecipes could not run because an app they follow is neither
	// installed nor planned.
	SkippedRecipes []string `json:"skippedRecipes,omitempty"`
	NeedsAdmin     bool     `json:"needsAdmin"`
}

// BuildPlan diffs a resolved profile against the installed set. Apps come
// first, in dependency order, then recipes; a recipe whose `after` apps are
// missing is skipped rather than failed.
func BuildPlan(cat *catalog.Catalog, res *catalog.Resolved, installed map[string]system.Installed) Plan {
	p := Plan{Steps: []Step{}, AlreadyInstalled: []string{}}
	planned := map[string]bool{}
	for _, pa := range res.Apps {
		if _, ok := installed[pa.ID]; ok {
			p.AlreadyInstalled = append(p.AlreadyInstalled, pa.ID)
			continue
		}
		app := cat.Apps[pa.ID]
		planned[pa.ID] = true
		p.Steps = append(p.Steps, Step{Kind: StepApp, ID: pa.ID, Name: app.Name, Version: pa.Version, Admin: app.Admin})
		p.NeedsAdmin = p.NeedsAdmin || app.Admin
	}
	for _, id := range res.Recipes {
		r := cat.Recipes[id]
		ok := true
		for _, a := range r.After {
			if _, have := installed[a]; !have && !planned[a] {
				ok = false
			}
		}
		if !ok {
			p.SkippedRecipes = append(p.SkippedRecipes, id)
			continue
		}
		p.Steps = append(p.Steps, Step{Kind: StepRecipe, ID: id, Name: r.Description, Admin: r.Admin})
		p.NeedsAdmin = p.NeedsAdmin || r.Admin
	}
	return p
}

// UninstallPlan removes the given catalog apps. Only apps in the catalog can be
// named, so WinForge never uninstalls anything it does not know.
func UninstallPlan(cat *catalog.Catalog, ids []string) Plan {
	p := Plan{Steps: []Step{}, AlreadyInstalled: []string{}}
	for _, id := range ids {
		app := cat.Apps[id]
		if app == nil {
			continue
		}
		p.Steps = append(p.Steps, Step{Kind: StepUninstall, ID: id, Name: app.Name, Admin: app.Admin})
		p.NeedsAdmin = p.NeedsAdmin || app.Admin
	}
	return p
}

// riskyRepairs change drivers or system files, so a restore point is made first.
var riskyRepairs = map[string]bool{
	"repair-missing-drivers": true,
	"repair-driver-updates":  true,
	"repair-windows-updates": true,
	"repair-windows-files":   true,
}

// RepairPlan turns repair recipe ids into steps. Only recipes whose id starts
// with "repair-" are accepted, so this can never run a profile's recipes or
// anything else from the catalog. A restore point goes first when any repair
// touches drivers or system files; duplicates are dropped.
func RepairPlan(cat *catalog.Catalog, ids []string) (Plan, error) {
	p := Plan{Steps: []Step{}, AlreadyInstalled: []string{}}
	seen := map[string]bool{}
	var wanted []string
	risky := false
	for _, id := range ids {
		if !strings.HasPrefix(id, "repair-") || cat.Recipes[id] == nil {
			return Plan{}, fmt.Errorf("%q is not a repair", id)
		}
		if id == "repair-restore-point" || seen[id] {
			continue
		}
		seen[id] = true
		wanted = append(wanted, id)
		risky = risky || riskyRepairs[id]
	}
	if risky {
		wanted = append([]string{"repair-restore-point"}, wanted...)
	}
	for _, id := range wanted {
		r := cat.Recipes[id]
		p.Steps = append(p.Steps, Step{Kind: StepRecipe, ID: id, Name: r.Description, Admin: r.Admin})
		p.NeedsAdmin = p.NeedsAdmin || r.Admin
	}
	return p, nil
}

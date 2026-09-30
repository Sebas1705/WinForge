// Package install turns a resolved profile into an ordered plan and runs it.
package install

import (
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/system"
)

// StepKind distinguishes installing an app from running a recipe.
type StepKind string

const (
	StepApp    StepKind = "app"
	StepRecipe StepKind = "recipe"
	// StepUpgrade updates an app that is already installed.
	StepUpgrade StepKind = "upgrade"
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

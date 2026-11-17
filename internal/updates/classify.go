package updates

import (
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
)

// Entry is an installed catalog app with a newer version available. Rule is set while the person's
// choice hides that update right now.
type Entry struct {
	ID        string // catalog id
	Name      string
	Publisher string
	Current   string
	Available string
	Rule      *Rule
}

// Classify turns winget's list of upgrades into the entries the Updates page shows: only catalog
// apps, each with the rule that hides it at this moment (if any).
func Classify(cat *catalog.Catalog, ups []system.Upgrade, rules map[string]Rule, now time.Time) []Entry {
	out := []Entry{}
	for _, s := range install.UpgradePlan(cat, ups, nil).Steps {
		app := cat.Apps[s.ID]
		cur := ""
		for _, u := range ups {
			if u.ID == app.Winget {
				cur = u.Current
			}
		}
		e := Entry{ID: s.ID, Name: app.Name, Publisher: app.Publisher, Current: cur, Available: s.Version}
		if r, ok := rules[s.ID]; ok && r.Hides(s.Version, now) {
			r := r
			e.Rule = &r
		}
		out = append(out, e)
	}
	return out
}

// PlanUpgrades is install.UpgradePlan for the given catalog apps (all when only is empty) without the
// updates the person chose not to install, even when "all" is asked for.
func PlanUpgrades(cat *catalog.Catalog, ups []system.Upgrade, only []string, rules map[string]Rule, now time.Time) install.Plan {
	plan := install.UpgradePlan(cat, ups, only)
	kept := make([]install.Step, 0, len(plan.Steps))
	plan.NeedsAdmin = false
	for _, s := range plan.Steps {
		if r, ok := rules[s.ID]; ok && r.Hides(s.Version, now) {
			continue
		}
		kept = append(kept, s)
		plan.NeedsAdmin = plan.NeedsAdmin || s.Admin
	}
	plan.Steps = kept
	return plan
}

package install

import (
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/system"
)

// UpgradePlan turns winget's list of available upgrades into steps, keeping
// only apps the catalog knows: the catalog is the trust boundary, so an
// unrelated installed program is never touched by WinForge.
func UpgradePlan(cat *catalog.Catalog, upgrades []system.Upgrade, only []string) Plan {
	want := map[string]bool{}
	for _, id := range only {
		want[id] = true
	}
	byWinget := map[string]string{}
	for id, a := range cat.Apps {
		byWinget[a.Winget] = id
	}
	p := Plan{Steps: []Step{}, AlreadyInstalled: []string{}}
	for _, u := range upgrades {
		id, ok := byWinget[u.ID]
		if !ok || (len(want) > 0 && !want[id]) {
			continue
		}
		app := cat.Apps[id]
		p.Steps = append(p.Steps, Step{Kind: StepUpgrade, ID: id, Name: app.Name, Version: u.Available, Admin: app.Admin})
		p.NeedsAdmin = p.NeedsAdmin || app.Admin
	}
	return p
}

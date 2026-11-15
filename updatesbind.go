package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// UpgradeInfo is an installed catalog app with a newer version available. When the person
// chose not to install it, Ignore says how and the page lists it apart.
type UpgradeInfo struct {
	ID            string `json:"id"` // catalog id
	Name          string `json:"name"`
	Publisher     string `json:"publisher"`
	Current       string `json:"current"`
	Available     string `json:"available"`
	Ignore        string `json:"ignore,omitempty"`        // "always", "version" or "until" while the update is hidden
	IgnoreVersion string `json:"ignoreVersion,omitempty"` // the skipped version
	IgnoreUntil   string `json:"ignoreUntil,omitempty"`   // RFC 3339, when a pause ends
}

func (a *App) clock() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// ignoreRules returns what the person chose not to update. A damaged list is an error and
// never "nothing": reading it as empty would update apps they asked to leave alone.
func (a *App) ignoreRules() (map[string]updates.Rule, error) {
	if a.ignored == nil {
		return map[string]updates.Rule{}, nil
	}
	return a.ignored.Load()
}

// Upgrades asks winget which installed packages have updates and keeps those
// the catalog knows; everything else on the PC is left alone.
func (a *App) Upgrades() ([]UpgradeInfo, error) {
	ups, err := system.WingetUpgrades(a.ctx)
	if err != nil {
		return nil, err
	}
	rules, err := a.ignoreRules()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.upgrades = ups
	a.mu.Unlock()
	return a.upgradeInfos(ups, rules), nil
}

// upgradeInfos is the list the Updates page shows, with the person's choices applied.
func (a *App) upgradeInfos(ups []system.Upgrade, rules map[string]updates.Rule) []UpgradeInfo {
	now := a.clock()
	out := []UpgradeInfo{}
	for _, s := range install.UpgradePlan(a.cat, ups, nil).Steps {
		app := a.cat.Apps[s.ID]
		cur := ""
		for _, u := range ups {
			if u.ID == app.Winget {
				cur = u.Current
			}
		}
		info := UpgradeInfo{ID: s.ID, Name: app.Name, Publisher: app.Publisher, Current: cur, Available: s.Version}
		if r, ok := rules[s.ID]; ok && r.Hides(s.Version, now) {
			info.Ignore, info.IgnoreVersion = string(r.Mode), r.Version
			if r.Mode == updates.Until {
				info.IgnoreUntil = r.Until.Format(time.RFC3339)
			}
		}
		out = append(out, info)
	}
	return out
}

// currentUpgrades is the list from the last check, with today's choices, without asking winget.
func (a *App) currentUpgrades() ([]UpgradeInfo, error) {
	rules, err := a.ignoreRules()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	ups := a.upgrades
	a.mu.Unlock()
	return a.upgradeInfos(ups, rules), nil
}

// PlanUpgrades previews updating the given catalog apps (all when empty). Updates the person
// chose not to install are left out, even when "all" is asked for.
func (a *App) PlanUpgrades(ids []string) (install.Plan, error) {
	a.mu.Lock()
	ups := a.upgrades
	a.mu.Unlock()
	rules, err := a.ignoreRules()
	if err != nil {
		return install.Plan{}, err
	}
	plan := install.UpgradePlan(a.cat, ups, ids)
	now := a.clock()
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
	return plan, nil
}

var errNoIgnoreStore = errors.New("WinForge could not open its settings folder, so ignored updates cannot be saved")

// IgnoreUpdate hides an app's update and returns the list as it now stands. mode is "always",
// "version" (the version the last check found) or "until" (for days days, 1 to 365).
func (a *App) IgnoreUpdate(id, mode string, days int) ([]UpgradeInfo, error) {
	if a.ignored == nil {
		return nil, errNoIgnoreStore
	}
	app, ok := a.cat.Apps[id]
	if !ok {
		return nil, fmt.Errorf("%q is not a catalog app", id)
	}
	var rule updates.Rule
	switch updates.Mode(mode) {
	case updates.Always:
		rule = updates.Rule{Mode: updates.Always}
	case updates.Version:
		found := ""
		a.mu.Lock()
		for _, u := range a.upgrades {
			if u.ID == app.Winget {
				found = u.Available
			}
		}
		a.mu.Unlock()
		if found == "" {
			return nil, fmt.Errorf("no update of %s was found; check for updates first", app.Name)
		}
		rule = updates.Rule{Mode: updates.Version, Version: found}
	case updates.Until:
		if days < 1 || days > updates.MaxPauseDays {
			return nil, fmt.Errorf("a pause lasts between 1 and %d days", updates.MaxPauseDays)
		}
		rule = updates.Rule{Mode: updates.Until, Until: a.clock().AddDate(0, 0, days)}
	default:
		return nil, fmt.Errorf("unknown way to ignore an update: %q", mode)
	}
	if err := a.ignored.Set(id, rule); err != nil {
		return nil, err
	}
	return a.currentUpgrades()
}

// StopIgnoringUpdate makes an app's update count again and returns the list as it now stands.
func (a *App) StopIgnoringUpdate(id string) ([]UpgradeInfo, error) {
	if a.ignored == nil {
		return nil, errNoIgnoreStore
	}
	if err := a.ignored.Remove(id); err != nil {
		return nil, err
	}
	return a.currentUpgrades()
}

package main

import (
	"errors"
	"fmt"
	"log"
	"sort"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/drift"
	"github.com/Sebas1705/WinForge/internal/system"
)

// DriftApp is an app named in a drift report.
type DriftApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DriftChange is an app whose version moved since the baseline.
type DriftChange struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// DriftInfo is what changed on this PC since a profile was last applied.
type DriftInfo struct {
	HasBaseline bool          `json:"hasBaseline"`
	At          string        `json:"at,omitempty"` // RFC 3339: when the PC was last in step with the profile
	Removed     []DriftApp    `json:"removed"`      // the profile's apps that are gone
	Added       []DriftApp    `json:"added"`        // apps installed since that the profile does not list
	Updated     []DriftChange `json:"updated"`
	Count       int           `json:"count"`
}

var errNoBaselineStore = errors.New("WinForge could not open its settings folder, so it cannot remember when a profile was applied")

// versionsOf reduces a scan to id -> version.
func versionsOf(installed map[string]system.Installed) map[string]string {
	out := make(map[string]string, len(installed))
	for id, i := range installed {
		out[id] = i.Version
	}
	return out
}

// profileApps returns the ids of a profile's apps, whether it is built in or the person's own.
func (a *App) profileApps(id string) ([]string, error) {
	if p, ok := a.cat.Profiles[id]; ok {
		return a.resolvedIDs(*p)
	}
	if store := a.profileStore(); store != nil {
		list, err := store.List(a.cat)
		if err != nil {
			return nil, err
		}
		for _, p := range list {
			if p.ID == id {
				return a.resolvedIDs(p)
			}
		}
	}
	return nil, fmt.Errorf("there is no profile %q", id)
}

func (a *App) resolvedIDs(p catalog.Profile) ([]string, error) {
	res, err := a.cat.ResolveProfile(&p)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(res.Apps))
	for _, pa := range res.Apps {
		ids = append(ids, pa.ID)
	}
	return ids, nil
}

// wholeProfileID returns the profile's id when what is being applied is that whole profile, and ""
// for a selection or an edited copy. Only a full apply says the PC is "in step" with a profile; a
// selection whose name happens to match one must not rewrite its history.
func (a *App) wholeProfileID(p catalog.Profile) string {
	want, err := a.resolvedIDs(p)
	if err != nil {
		return ""
	}
	have, err := a.profileApps(p.ID)
	if err != nil || len(have) != len(want) {
		return ""
	}
	sort.Strings(want)
	sort.Strings(have)
	for i := range want {
		if want[i] != have[i] {
			return ""
		}
	}
	return p.ID
}

// rememberBaseline is called after a scan: when a whole profile has just been applied successfully, the
// scan that follows is the state the profile put the PC in.
func (a *App) rememberBaseline(installed map[string]system.Installed) {
	a.mu.Lock()
	id := a.baselineFor
	a.baselineFor = ""
	a.mu.Unlock()
	if id == "" || a.applied == nil {
		return
	}
	if err := a.applied.Set(id, versionsOf(installed), a.clock()); err != nil {
		log.Printf("winforge: remembering the state after applying %s: %v", id, err)
	}
}

// driftCounts returns, per profile id, how many things changed since its baseline. Profiles with no
// baseline are absent. A damaged history is logged and shows nothing rather than stopping the page.
func (a *App) driftCounts(profiles map[string][]string, installed map[string]system.Installed) map[string]int {
	out := map[string]int{}
	if a.applied == nil {
		return out
	}
	all, err := a.applied.All()
	if err != nil {
		log.Printf("winforge: profile history: %v", err)
		return out
	}
	now := versionsOf(installed)
	for id, base := range all {
		if apps, ok := profiles[id]; ok {
			out[id] = drift.Compare(base, apps, now).Count()
		}
	}
	return out
}

func (a *App) nameOf(id string) string {
	if app := a.cat.Apps[id]; app != nil {
		return app.Name
	}
	return id
}

// ProfileDrift says what changed on this PC since the profile was last applied, using the last scan.
func (a *App) ProfileDrift(id string) (DriftInfo, error) {
	info := DriftInfo{Removed: []DriftApp{}, Added: []DriftApp{}, Updated: []DriftChange{}}
	if a.applied == nil {
		return info, errNoBaselineStore
	}
	apps, err := a.profileApps(id)
	if err != nil {
		return info, err
	}
	base, ok, err := a.applied.Get(id)
	if err != nil {
		return info, err
	}
	if !ok {
		return info, nil
	}
	a.mu.Lock()
	installed := a.installed
	a.mu.Unlock()
	if installed == nil {
		return info, errors.New("this PC has not been scanned yet")
	}
	r := drift.Compare(base, apps, versionsOf(installed))
	info.HasBaseline, info.At, info.Count = true, base.At.Format("2006-01-02T15:04:05Z07:00"), r.Count()
	for _, x := range r.Removed {
		info.Removed = append(info.Removed, DriftApp{ID: x, Name: a.nameOf(x)})
	}
	for _, x := range r.Added {
		info.Added = append(info.Added, DriftApp{ID: x, Name: a.nameOf(x)})
	}
	for _, x := range r.Updated {
		info.Updated = append(info.Updated, DriftChange{ID: x.ID, Name: a.nameOf(x.ID), From: x.From, To: x.To})
	}
	return info, nil
}

// AcceptProfileState remembers the PC as it is now as the state the profile is in step with, so the
// changes shown so far stop being reported. It changes nothing on the PC.
func (a *App) AcceptProfileState(id string) error {
	if a.applied == nil {
		return errNoBaselineStore
	}
	if _, err := a.profileApps(id); err != nil {
		return err
	}
	a.mu.Lock()
	installed := a.installed
	a.mu.Unlock()
	if installed == nil {
		return errors.New("this PC has not been scanned yet")
	}
	return a.applied.Set(id, versionsOf(installed), a.clock())
}

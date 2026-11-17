package sysapps

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/drift"
	"github.com/Sebas1705/WinForge/internal/eol"
	"github.com/Sebas1705/WinForge/internal/history"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/updates"
)

// ---------------------------------------------------------------------------------------------
// winforge_status

type wingetOut struct {
	Present bool   `json:"present"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
}

type statusOut struct {
	Version         string    `json:"version"`
	Elevated        bool      `json:"elevated"`
	Winget          wingetOut `json:"winget"`
	CatalogApps     int       `json:"catalogApps"`
	CatalogProfiles int       `json:"catalogProfiles"`
	CatalogRecipes  int       `json:"catalogRecipes"`
	AppData         string    `json:"appData"`
}

func (r *rt) statusTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_status",
		Title: "WinForge status",
		Description: "Returns WinForge's version, whether this server runs with administrator rights, whether winget is present (and its version), " +
			"how many apps, profiles and recipes the catalog has, and WinForge's own data folder (user name redacted). " +
			"It reads no installed software and no personal data; it never installs, changes or writes anything.",
		InputSchema: mcpkit.NoArgs,
		Class:       mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct{}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			e := r.env(c)
			red := redactor(e)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			out := statusOut{Version: e.Version, Elevated: elevated(e), CatalogApps: len(cat.Apps),
				CatalogProfiles: len(cat.Profiles), CatalogRecipes: len(cat.Recipes)}
			if p, err := appDataOf(e); err == nil {
				out.AppData = red(p)
			}
			if p, err := r.d.LookPath("winget"); err == nil {
				out.Winget = wingetOut{Present: true, Path: red(p)}
				if e.Exec != nil {
					cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
					first := ""
					code, err := e.Exec.Run(cctx, "winget", []string{"--version"}, func(l string) {
						if first == "" {
							first = strings.TrimSpace(l)
						}
					})
					cancel()
					if err == nil && code == 0 {
						out.Winget.Version = strings.TrimPrefix(first, "v")
					}
				}
			}
			return mcpkit.JSON(out), nil
		},
	}
}

// ---------------------------------------------------------------------------------------------
// winforge_catalog_search

type catalogAppOut struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	Publisher        string `json:"publisher"`
	WingetID         string `json:"wingetId"`
	OpenSource       bool   `json:"openSource"`
	Installed        *bool  `json:"installed,omitempty"`
	InstalledVersion string `json:"installedVersion,omitempty"`
}

type catalogSearchOut struct {
	Total            int             `json:"total"`
	Truncated        bool            `json:"truncated"`
	InstalledChecked bool            `json:"installedChecked"`
	Apps             []catalogAppOut `json:"apps"`
	Warnings         []string        `json:"warnings"`
}

func matches(a *catalog.App, terms []string) bool {
	hay := strings.ToLower(strings.Join(append([]string{a.ID, a.Name, a.Category, a.Publisher, a.Winget, a.Description}, a.Tags...), " "))
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

func rank(a *catalog.App, q string) int {
	n := strings.ToLower(a.Name)
	switch {
	case q == "":
		return 3
	case a.ID == q || n == q:
		return 0
	case strings.HasPrefix(n, q) || strings.HasPrefix(a.ID, q):
		return 1
	case strings.Contains(n, q) || strings.Contains(a.ID, q):
		return 2
	}
	return 3
}

func (r *rt) catalogSearchTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_catalog_search",
		Title: "Search the app catalog",
		Description: "Searches WinForge's built-in app catalog (every word of the query must match the id, name, category, publisher, winget id, description or tags) and returns " +
			"id, name, category, publisher, wingetId and openSource per app, best matches first. " +
			"By default it does NOT look at this PC. Only when withInstalled is true (or installedOnly is true, which implies it) does it read the PC's installed software to add " +
			"installed and installedVersion: that list is personal data, so ask for it only when needed. " +
			"It never installs, updates or removes anything.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{` +
			`"query":{"type":"string","maxLength":100,"description":"words to look for; empty matches every app"},` +
			`"category":{"type":"string","maxLength":60,"description":"only apps of this category (case-insensitive)"},` +
			`"withInstalled":{"type":"boolean","default":false,"description":"also say which apps are installed here (reads the PC's software list)"},` +
			`"installedOnly":{"type":"boolean","default":false,"description":"only apps installed here (implies withInstalled)"},` +
			`"limit":` + limitSchema + `},"required":["query"]}`),
		Class: mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct {
				Query         *string `json:"query"`
				Category      string  `json:"category"`
				WithInstalled bool    `json:"withInstalled"`
				InstalledOnly bool    `json:"installedOnly"`
				Limit         *int    `json:"limit"`
			}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			if a.Query == nil {
				return mcpkit.Fail("query is required (it may be an empty string)"), nil
			}
			if len([]rune(*a.Query)) > maxQuery || len([]rune(a.Category)) > 60 {
				return mcpkit.Fail("query is too long (at most %d characters)", maxQuery), nil
			}
			limit, err := limitOf(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			e := r.env(c)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			out := catalogSearchOut{Apps: []catalogAppOut{}, Warnings: []string{}}
			var found map[string]system.Installed
			if a.WithInstalled || a.InstalledOnly {
				s, err := r.scan(ctx, e, cat)
				if err != nil {
					return mcpkit.Fail("could not scan this PC: %s", err.Error()), nil
				}
				found, out.InstalledChecked = s.installed, true
				out.Warnings = warnings(s.warning)
			}
			q := strings.ToLower(strings.TrimSpace(*a.Query))
			terms := strings.Fields(q)
			cats := strings.ToLower(strings.TrimSpace(a.Category))
			var hits []*catalog.App
			for _, app := range cat.Apps {
				if cats != "" && strings.ToLower(app.Category) != cats {
					continue
				}
				if !matches(app, terms) {
					continue
				}
				if a.InstalledOnly {
					if _, ok := found[app.ID]; !ok {
						continue
					}
				}
				hits = append(hits, app)
			}
			sort.Slice(hits, func(i, j int) bool {
				ri, rj := rank(hits[i], q), rank(hits[j], q)
				if ri != rj {
					return ri < rj
				}
				ni, nj := strings.ToLower(hits[i].Name), strings.ToLower(hits[j].Name)
				if ni != nj {
					return ni < nj
				}
				return hits[i].ID < hits[j].ID
			})
			out.Total = len(hits)
			var cut bool
			hits, cut = capList(hits, limit)
			out.Truncated = cut
			for _, app := range hits {
				o := catalogAppOut{ID: app.ID, Name: app.Name, Category: app.Category, Publisher: app.Publisher, WingetID: app.Winget, OpenSource: app.OpenSource}
				if out.InstalledChecked {
					i, ok := found[app.ID]
					o.Installed = &ok
					o.InstalledVersion = i.Version
				}
				out.Apps = append(out.Apps, o)
			}
			return mcpkit.JSON(out), nil
		},
	}
}

// ---------------------------------------------------------------------------------------------
// winforge_installed_apps

type installedAppOut struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Version string   `json:"version,omitempty"`
	Sources []string `json:"sources"`
}

type installedAppsOut struct {
	Total      int               `json:"total"`
	Truncated  bool              `json:"truncated"`
	CatalogApp int               `json:"catalogApps"`
	Installed  []installedAppOut `json:"installed"`
	Warnings   []string          `json:"warnings"`
}

func (r *rt) installedAppsTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_installed_apps",
		Title: "Installed catalog apps",
		Description: "PERSONAL DATA: reads the software installed on this PC (winget's list, the Uninstall registry keys and PATH) and returns the catalog apps found, " +
			"with id, name, version and which sources saw it, sorted by id. Only apps the catalog knows are listed; everything else on the PC is left out. " +
			"Use it only when the person's installed software is needed. It never installs, updates or removes anything.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{"limit":` + limitSchema + `}}`),
		Class:       mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct {
				Limit *int `json:"limit"`
			}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			limit, err := limitOf(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			e := r.env(c)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			s, err := r.scan(ctx, e, cat)
			if err != nil {
				return mcpkit.Fail("could not scan this PC: %s", err.Error()), nil
			}
			ids := make([]string, 0, len(s.installed))
			for id := range s.installed {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			out := installedAppsOut{Total: len(ids), CatalogApp: len(cat.Apps), Installed: []installedAppOut{}, Warnings: warnings(s.warning)}
			ids, out.Truncated = capList(ids, limit)
			for _, id := range ids {
				i := s.installed[id]
				out.Installed = append(out.Installed, installedAppOut{ID: id, Name: appName(cat, id), Version: i.Version, Sources: append([]string{}, i.Sources...)})
			}
			return mcpkit.JSON(out), nil
		},
	}
}

// ---------------------------------------------------------------------------------------------
// winforge_updates_check

type ignoreOut struct {
	Mode    string `json:"mode"`
	Version string `json:"version,omitempty"`
	Until   string `json:"until,omitempty"`
}

type updateOut struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Publisher string     `json:"publisher"`
	Current   string     `json:"current"`
	Available string     `json:"available"`
	Admin     bool       `json:"admin,omitempty"`
	Ignore    *ignoreOut `json:"ignore,omitempty"`
}

type updatesCheckOut struct {
	Active       []updateOut `json:"active"`
	Ignored      []updateOut `json:"ignored"`
	ActiveCount  int         `json:"activeCount"`
	IgnoredCount int         `json:"ignoredCount"`
	Truncated    bool        `json:"truncated"`
	Warnings     []string    `json:"warnings"`
}

// ignoreRules loads the person's "do not update" choices. A damaged file is an error, never "no rules".
func ignoreRules(e *mcpkit.Env) (map[string]updates.Rule, error) {
	p, err := inAppData(e, "ignored-updates.json")
	if err != nil {
		return nil, err
	}
	return (&updates.Store{Path: p}).Load()
}

func (r *rt) updatesCheckTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_updates_check",
		Title: "Check for app updates",
		Description: "Asks winget which installed catalog apps have a newer version (the same data the Updates page shows) and splits them into active updates and ignored ones " +
			"(the person chose never / this version / until a date, shown with the rule). It reveals which catalog apps are installed, so treat the answer as personal data. " +
			"Apps outside the catalog are never listed. It never installs or updates anything: winforge_prepare_update builds a plan for that.",
		InputSchema: mcpkit.NoArgs,
		Class:       mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct{}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			e := r.env(c)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			ups, err := r.d.Upgrades(ctx)
			if err != nil {
				return mcpkit.Fail("could not ask winget for updates: %s", redactor(e)(err.Error())), nil
			}
			out := updatesCheckOut{Active: []updateOut{}, Ignored: []updateOut{}, Warnings: []string{}}
			rules, rerr := ignoreRules(e)
			if rerr != nil {
				return mcpkit.Fail("the ignored-updates list cannot be read (%s); nothing was reported so no ignored update is shown as active", redactor(e)(rerr.Error())), nil
			}
			var active, ignored []updateOut
			for _, en := range updates.Classify(cat, ups, rules, now(e)) {
				u := updateOut{ID: en.ID, Name: en.Name, Publisher: en.Publisher, Current: en.Current, Available: en.Available, Admin: cat.Apps[en.ID].Admin}
				if en.Rule != nil {
					u.Ignore = &ignoreOut{Mode: string(en.Rule.Mode), Version: en.Rule.Version}
					if en.Rule.Mode == updates.Until {
						u.Ignore.Until = en.Rule.Until.UTC().Format(time.RFC3339)
					}
					ignored = append(ignored, u)
				} else {
					active = append(active, u)
				}
			}
			out.ActiveCount, out.IgnoredCount = len(active), len(ignored)
			var c1, c2 bool
			out.Active, c1 = capList(active, maxLimit)
			out.Ignored, c2 = capList(ignored, maxLimit)
			out.Truncated = c1 || c2
			return mcpkit.JSON(out), nil
		},
	}
}

// ---------------------------------------------------------------------------------------------
// profiles

type profileSummaryOut struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"`
	Source      string `json:"source"` // builtin | user
	Apps        int    `json:"apps"`
	Recipes     int    `json:"recipes"`
}

type profilesListOut struct {
	Total     int                 `json:"total"`
	Truncated bool                `json:"truncated"`
	Profiles  []profileSummaryOut `json:"profiles"`
	Warnings  []string            `json:"warnings"`
}

// allProfiles lists the built-in profiles and the person's own (a damaged or unreadable folder is a
// warning, never a failure), built-in first within the same name.
func allProfiles(e *mcpkit.Env, cat *catalog.Catalog) (list []catalog.Profile, source map[string]string, warn string) {
	source = map[string]string{}
	ids := make([]string, 0, len(cat.Profiles))
	for id := range cat.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		list = append(list, *cat.Profiles[id])
		source[id] = "builtin"
	}
	users, err := userProfiles(e, cat)
	if err != nil {
		warn = "your own profiles could not be read: " + redactor(e)(err.Error())
	}
	for _, p := range users {
		if _, dup := source[p.ID]; dup {
			continue
		}
		list = append(list, p)
		source[p.ID] = "user"
	}
	return list, source, warn
}

func resolvedCounts(cat *catalog.Catalog, p catalog.Profile) (apps, recipes int) {
	res, err := cat.ResolveProfile(&p)
	if err != nil {
		return len(p.Apps), len(p.Recipes)
	}
	return len(res.Apps), len(res.Recipes)
}

func (r *rt) profilesListTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_profiles_list",
		Title: "List profiles",
		Description: "Lists WinForge's built-in profiles and the person's own saved profiles: id, name, description, kind (base | dev | custom), source (builtin | user) and how many apps and recipes each resolves to. " +
			"Data only: it reads no installed software, installs nothing and never runs a recipe.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{"limit":` + limitSchema + `}}`),
		Class:       mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct {
				Limit *int `json:"limit"`
			}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			limit, err := limitOf(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			e := r.env(c)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			list, source, warn := allProfiles(e, cat)
			out := profilesListOut{Total: len(list), Profiles: []profileSummaryOut{}, Warnings: warnings(warn)}
			list, out.Truncated = capList(list, limit)
			for _, p := range list {
				apps, recipes := resolvedCounts(cat, p)
				out.Profiles = append(out.Profiles, profileSummaryOut{ID: p.ID, Name: p.Name, Description: p.Description, Kind: kindOf(p, source[p.ID]), Source: source[p.ID], Apps: apps, Recipes: recipes})
			}
			return mcpkit.JSON(out), nil
		},
	}
}

func kindOf(p catalog.Profile, source string) string {
	if p.Kind != "" {
		return p.Kind
	}
	if source == "user" {
		return "custom"
	}
	return "base"
}

type profileAppOut struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type driftAppOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type driftChangeOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

type driftOut struct {
	At      string           `json:"at"`
	Count   int              `json:"count"`
	Removed []driftAppOut    `json:"removed"`
	Added   []driftAppOut    `json:"added"`
	Updated []driftChangeOut `json:"updated"`
}

type profileGetOut struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Kind        string          `json:"kind"`
	Source      string          `json:"source"`
	Extends     []string        `json:"extends"`
	Apps        []profileAppOut `json:"apps"`
	AppsTotal   int             `json:"appsTotal"`
	Recipes     []string        `json:"recipes"`
	Truncated   bool            `json:"truncated"`
	Drift       *driftOut       `json:"drift,omitempty"`
	Warnings    []string        `json:"warnings"`
}

// findProfile looks a profile up by id: built-in first, then the person's own.
func findProfile(e *mcpkit.Env, cat *catalog.Catalog, id string) (*catalog.Profile, string, bool) {
	if p := cat.Profiles[id]; p != nil {
		return p, "builtin", true
	}
	users, _ := userProfiles(e, cat)
	for i := range users {
		if users[i].ID == id {
			return &users[i], "user", true
		}
	}
	return nil, "", false
}

func (r *rt) profileGetTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_profile_get",
		Title: "Get one profile",
		Description: "Returns one profile (built-in or the person's own): its extends, the apps it resolves to (id, name, pinned version; requirements come first) and its recipe ids. " +
			"Recipes are returned as ids only, never their scripts. If the profile was applied before and a baseline was remembered, it also returns the drift since then " +
			"(apps removed, added or updated), which reads the PC's installed software; without a baseline the PC is not scanned. " +
			"It never installs or changes anything.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{"id":` + idSchema + `,"limit":` + limitSchema + `},"required":["id"]}`),
		Class:       mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct {
				ID    string `json:"id"`
				Limit *int   `json:"limit"`
			}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			if err := checkID("id", a.ID); err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			limit, err := limitOf(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			e := r.env(c)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			p, source, ok := findProfile(e, cat, a.ID)
			if !ok {
				return mcpkit.Fail("there is no profile %q (use winforge_profiles_list)", a.ID), nil
			}
			res, err := cat.ResolveProfile(p)
			if err != nil {
				return mcpkit.Fail("the profile cannot be resolved: %s", err.Error()), nil
			}
			out := profileGetOut{ID: p.ID, Name: p.Name, Description: p.Description, Kind: kindOf(*p, source), Source: source,
				Extends: append([]string{}, p.Extends...), Apps: []profileAppOut{}, AppsTotal: len(res.Apps),
				Recipes: append([]string{}, res.Recipes...), Warnings: []string{}}
			apps, cut := capList(res.Apps, limit)
			out.Truncated = cut
			for _, pa := range apps {
				out.Apps = append(out.Apps, profileAppOut{ID: pa.ID, Name: appName(cat, pa.ID), Version: pa.Version})
			}
			if len(out.Recipes) > limit {
				out.Recipes, out.Truncated = out.Recipes[:limit], true
			}
			d, warn, err := r.driftOf(ctx, e, cat, p.ID, res)
			if err != nil {
				return mcpkit.Fail("could not scan this PC: %s", err.Error()), nil
			}
			out.Drift = d
			out.Warnings = warnings(warn)
			return mcpkit.JSON(out), nil
		},
	}
}

// driftOf reads the remembered baseline of a profile (read-only) and, only if there is one, compares it
// with a scan of the PC. nil means no baseline (or none could be read).
func (r *rt) driftOf(ctx context.Context, e *mcpkit.Env, cat *catalog.Catalog, id string, res *catalog.Resolved) (*driftOut, string, error) {
	path, err := inAppData(e, "applied.json")
	if err != nil {
		return nil, "", nil
	}
	base, ok, err := (&drift.Store{Path: path}).Get(id)
	if err != nil {
		return nil, "the remembered state of profiles could not be read: " + redactor(e)(err.Error()), nil
	}
	if !ok {
		return nil, "", nil
	}
	s, err := r.scan(ctx, e, cat)
	if err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(res.Apps))
	for _, pa := range res.Apps {
		ids = append(ids, pa.ID)
	}
	rep := drift.Compare(base, ids, versionsOf(s.installed))
	d := &driftOut{At: base.At.UTC().Format(time.RFC3339), Count: rep.Count(), Removed: []driftAppOut{}, Added: []driftAppOut{}, Updated: []driftChangeOut{}}
	for _, x := range rep.Removed {
		d.Removed = append(d.Removed, driftAppOut{ID: x, Name: appName(cat, x)})
	}
	for _, x := range rep.Added {
		d.Added = append(d.Added, driftAppOut{ID: x, Name: appName(cat, x)})
	}
	for _, x := range rep.Updated {
		d.Updated = append(d.Updated, driftChangeOut{ID: x.ID, Name: appName(cat, x.ID), From: x.From, To: x.To})
	}
	return d, s.warning, nil
}

// ---------------------------------------------------------------------------------------------
// winforge_install_plan

type planOut struct {
	Steps            []planStepOut `json:"steps"`
	AlreadyInstalled []string      `json:"alreadyInstalled"`
	SkippedRecipes   []string      `json:"skippedRecipes"`
	NeedsAdmin       bool          `json:"needsAdmin"`
}

type planStepOut struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Admin   bool   `json:"admin,omitempty"`
}

type installPlanOut struct {
	Target string `json:"target"` // profile id or "apps"
	planOut
	Warnings []string `json:"warnings"`
}

// buildInstallPlan resolves a profile id or a list of app ids against the catalog and scan, the way the
// app's plan does (requirements first, recipes of the apps, recipes whose app is missing skipped).
func buildInstallPlan(e *mcpkit.Env, cat *catalog.Catalog, profileID string, appIDs []string, installed map[string]system.Installed) (planOutAndPlan, string, error) {
	var p catalog.Profile
	target := "apps"
	if profileID != "" {
		found, _, ok := findProfile(e, cat, profileID)
		if !ok {
			return planOutAndPlan{}, "", errors.New("there is no profile " + quote(profileID) + " (use winforge_profiles_list); nothing was stored")
		}
		p, target = *found, profileID
	} else {
		p = catalog.Profile{ID: "selection", Name: "selection"}
		for _, id := range appIDs {
			p.Apps = append(p.Apps, catalog.ProfileApp{ID: id})
		}
	}
	res, err := cat.ResolveProfile(&p)
	if err != nil {
		return planOutAndPlan{}, "", err
	}
	plan := install.BuildPlan(cat, res, installed)
	return planOutAndPlan{plan: plan, out: toPlanOut(plan)}, target, nil
}

func (r *rt) installPlanTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_install_plan",
		Title: "Preview an install plan",
		Description: "Returns the plan WinForge would build to install a profile (profileId) or a list of catalog apps (appIds, exactly one of the two): " +
			"steps (kind app | recipe, id, name, version, admin) in run order, alreadyInstalled, skippedRecipes and needsAdmin. " +
			"To tell what is already installed it reads the PC's software list once (personal data). " +
			"It is a pure preview: it never stores a plan, never runs anything and never changes anything. Use winforge_prepare_install to prepare something that can be applied.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{"profileId":` + idSchema + `,"appIds":` + idsSchema("catalog app ids") + `}}`),
		Class:       mcpkit.Read,
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
			return mcpkit.JSON(installPlanOut{Target: target, planOut: built.out, Warnings: warnings(s.warning)}), nil
		},
	}
}

// ---------------------------------------------------------------------------------------------
// winforge_eol_report

type eolOut struct {
	AsOf      string        `json:"asOf"`
	AgeDays   int           `json:"ageDays"`
	Stale     bool          `json:"stale"`
	Total     int           `json:"total"`
	Truncated bool          `json:"truncated"`
	Findings  []eol.Finding `json:"findings"`
	Warnings  []string      `json:"warnings"`
}

func (r *rt) eolReportTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_eol_report",
		Title: "End-of-support report",
		Description: "Returns end-of-support findings for installed catalog apps whose release line ends (status ended | soon | ok | unknown, end date, days left, supported replacements), " +
			"matched against the table embedded in WinForge: nothing is downloaded and nothing leaves the PC. It reads the PC's installed software (personal data). " +
			"asOf is the date of the table and stale says it is older than a year, so the answer may be out of date. It never installs or changes anything.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{"limit":` + limitSchema + `}}`),
		Class:       mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct {
				Limit *int `json:"limit"`
			}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			limit, err := limitOf(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			e := r.env(c)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			data, err := r.d.EOL()
			if err != nil || data == nil {
				return mcpkit.Fail("the end-of-support table could not be read"), nil
			}
			s, err := r.scan(ctx, e, cat)
			if err != nil {
				return mcpkit.Fail("could not scan this PC: %s", err.Error()), nil
			}
			in := make([]eol.Installed, 0, len(s.installed))
			for id, i := range s.installed {
				in = append(in, eol.Installed{AppID: id, Version: i.Version})
			}
			sort.Slice(in, func(i, j int) bool { return in[i].AppID < in[j].AppID })
			rep := data.Build(in, now(e))
			out := eolOut{AsOf: rep.AsOf, AgeDays: rep.AgeDays, Stale: rep.Stale, Total: len(rep.Findings), Warnings: warnings(s.warning)}
			out.Findings, out.Truncated = capList(rep.Findings, limit)
			return mcpkit.JSON(out), nil
		},
	}
}

// ---------------------------------------------------------------------------------------------
// winforge_history

type historyOut struct {
	Total     int             `json:"total"`
	Truncated bool            `json:"truncated"`
	Entries   []history.Entry `json:"entries"`
}

func (r *rt) historyTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_history",
		Title: "Install and update history",
		Description: "Returns what WinForge installed, updated or rolled back (newest first): id, app, name, action, version from/to, time, result (ok | failed | restart), source and failure reason. " +
			"Filter by app id and/or result. It reads WinForge's own history file only: it never scans the PC, never installs anything and never rolls anything back.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{"app":` + idSchema + `,` +
			`"result":{"type":"string","enum":["ok","failed","restart"]},"limit":` + limitSchema + `}}`),
		Class: mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct {
				App    string `json:"app"`
				Result string `json:"result"`
				Limit  *int   `json:"limit"`
			}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			if a.App != "" {
				if err := checkID("app", a.App); err != nil {
					return mcpkit.Fail("%s", err.Error()), nil
				}
			}
			switch a.Result {
			case "", "ok", "failed", "restart":
			default:
				return mcpkit.Fail("result must be ok, failed or restart"), nil
			}
			limit, err := limitOf(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			e := r.env(c)
			out := historyOut{Entries: []history.Entry{}}
			p, err := inAppData(e, "history.json")
			if err != nil {
				return mcpkit.JSON(out), nil // no data folder: there is no history
			}
			all, err := (&history.FileStore{Path: p}).All()
			if err != nil {
				return mcpkit.Fail("the history could not be read: %s", redactor(e)(err.Error())), nil
			}
			page := history.Query(all, history.Filter{App: a.App, Result: a.Result}, func(string) string { return "" })
			out.Total = page.Total
			views, cut := capList(page.Entries, limit)
			out.Truncated = cut
			for _, v := range views {
				out.Entries = append(out.Entries, v.Entry)
			}
			return mcpkit.JSON(out), nil
		},
	}
}

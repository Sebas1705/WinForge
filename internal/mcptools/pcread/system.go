package pcread

import (
	"context"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// devTools is the developer-environment group: the environment doctor.
func (g *group) devTools() []mcpkit.Tool {
	env := g.read("winforge_env_scan", "Environment doctor",
		"Runs WinForge's environment doctor over the user and machine PATH and the *_HOME style variables and returns its findings: id, kind (for example a PATH entry that does not exist, a duplicate, a shadowed tool), severity, scope, subject, the paths involved, and whether WinForge could fix it. "+
			"By default it returns only the findings and counts; set includePaths to also get the PATH entries, the tools found on PATH and the *_HOME variables. It never dumps the whole environment (no secrets, tokens or other variables). "+
			"PRIVACY: it describes folders and settings of this PC and the model sees them; every path has the user name and user folder removed. It NEVER changes a variable, the PATH or the registry.",
		mcpkit.Schema(`{"type":"object","properties":{"includePaths":{"type":"boolean","default":false,"description":"Also return the PATH entries, tools found on PATH and *_HOME variables (redacted)."},`+limitProp+`},"additionalProperties":false}`),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			var a struct {
				IncludePaths bool `json:"includePaths"`
				Limit        *int `json:"limit"`
			}
			if err := mcpkit.Decode(x.c.Args, &a); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			limit, err := limitArg(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			rep, err := runCtx(ctx, g.d.EnvScan)
			if err != nil {
				return nil, err
			}
			redactAll := func(in []string, n int) []string {
				out := make([]string, 0, len(in))
				for _, s := range in {
					out = append(out, x.redact(short(s, maxText)))
				}
				out, _ = capped(out, n)
				return out
			}
			type fnd struct {
				ID       string   `json:"id"`
				Kind     string   `json:"kind"`
				Severity string   `json:"severity"`
				Scope    string   `json:"scope"`
				Subject  string   `json:"subject"`
				Paths    []string `json:"paths"`
				Fixable  bool     `json:"fixable"`
				FixKind  string   `json:"fixKind,omitempty"`
			}
			fs := make([]fnd, 0, len(rep.Findings))
			for _, f := range rep.Findings {
				fs = append(fs, fnd{x.redact(f.ID), f.Kind, f.Severity, string(f.Scope), x.redact(short(f.Subject, maxText)), redactAll(f.Paths, 10), f.Fixable, f.FixKind})
			}
			fs, cut := capped(fs, limit)
			out := map[string]any{
				"findings": fs, "truncated": cut,
				"counts": map[string]int{"findings": len(rep.Findings), "userPathEntries": len(rep.UserPath), "machinePathEntries": len(rep.MachinePath), "tools": len(rep.Tools), "homeVars": len(rep.Vars)},
			}
			if a.IncludePaths {
				type tool struct {
					Name  string   `json:"name"`
					Found []string `json:"found"`
				}
				type hv struct {
					Name  string `json:"name"`
					Scope string `json:"scope"`
					Value string `json:"value"`
				}
				tools := make([]tool, 0, len(rep.Tools))
				for _, t := range rep.Tools {
					tools = append(tools, tool{x.redact(t.Name), redactAll(t.Found, 10)})
				}
				vars := make([]hv, 0, len(rep.Vars))
				for _, v := range rep.Vars {
					vars = append(vars, hv{x.redact(v.Name), string(v.Scope), x.redact(short(v.Value, maxText))})
				}
				var c1, c2, c3, c4 bool
				up, c1 := capped(redactAll(rep.UserPath, len(rep.UserPath)), limit)
				mp, c2 := capped(redactAll(rep.MachinePath, len(rep.MachinePath)), limit)
				tools, c3 = capped(tools, limit)
				vars, c4 = capped(vars, limit)
				out["userPath"], out["machinePath"], out["tools"], out["homeVars"] = up, mp, tools, vars
				out["truncated"] = cut || c1 || c2 || c3 || c4
			}
			return mcpkit.JSON(out), nil
		})
	return []mcpkit.Tool{env}
}

// systemTools are Windows features, WSL and privacy settings.
func (g *group) systemTools() []mcpkit.Tool {
	feat := g.read("winforge_features_list", "Windows optional features",
		"Lists the Windows optional features WinForge knows (WSL, Virtual Machine Platform, Hyper-V, Windows Sandbox, .NET 3.5): id, state (on, off, unavailable or unknown), whether a restart is pending and whether changing it needs administrator rights. "+
			"It describes this PC's Windows settings and is seen by the model. It NEVER turns a feature on or off.",
		mcpkit.NoArgs,
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			if err := mcpkit.Decode(x.c.Args, &struct{}{}); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			elevated := x.elevated()
			fs, err := runCtx(ctx, func() ([]featureRow, error) {
				var rows []featureRow
				for _, f := range g.d.FeaturesList(elevated) {
					rows = append(rows, featureRow{f.ID, f.State, f.RestartPending, f.NeedsAdmin})
				}
				return rows, nil
			})
			if err != nil {
				return nil, err
			}
			if fs == nil {
				fs = []featureRow{}
			}
			return mcpkit.JSON(map[string]any{"features": fs, "ranAsAdmin": elevated}), nil
		})

	wsl := g.read("winforge_wsl_info", "WSL distributions",
		"Lists the WSL distributions installed on this PC (name, state, WSL version, whether it is the default) and the default WSL version, using `wsl.exe -l -v`. Names and states only: it never starts a distribution and never runs anything inside one. "+
			"It describes this PC's setup and is seen by the model (names are redacted). It NEVER changes anything.",
		mcpkit.Schema(limitOnly),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			limit, bad := listArgs(x)
			if bad != nil {
				return bad, nil
			}
			info, err := runCtx(ctx, func() (wslRow, error) {
				w := g.d.WSLInfo()
				r := wslRow{Installed: w.Installed, DefaultVersion: w.DefaultVersion, Distros: []distroRow{}}
				for _, d := range w.Distros {
					r.Distros = append(r.Distros, distroRow{x.redact(d.Name), x.redact(d.State), d.Version, d.IsDefault})
				}
				return r, nil
			})
			if err != nil {
				return nil, err
			}
			var cut bool
			info.Distros, cut = capped(info.Distros, limit)
			info.Truncated = cut
			return mcpkit.JSON(info), nil
		})

	priv := g.read("winforge_privacy_list", "Privacy settings",
		"Lists the privacy settings WinForge can tune (telemetry, advertising id, suggestions and similar): id, area, state (default, private, mixed or unknown), the translation keys of its name and explanation, and for each registry value it would write: the key path, value name, whether it exists now, its current type and data, and the type and data the tweak would write. "+
			"It describes this PC's current-user registry settings and is seen by the model. It reads those values and NEVER writes them: nothing is applied, reset or restored.",
		mcpkit.Schema(limitOnly),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			limit, bad := listArgs(x)
			if bad != nil {
				return bad, nil
			}
			list, err := runCtx(ctx, g.d.PrivacyList)
			if err != nil {
				return nil, err
			}
			type change struct {
				Path    string `json:"path"`
				Name    string `json:"name"`
				Existed bool   `json:"existed"`
				OldType string `json:"oldType,omitempty"`
				Old     string `json:"old,omitempty"`
				Type    string `json:"type"`
				Private uint32 `json:"private"`
				Error   bool   `json:"readError,omitempty"`
			}
			type tweak struct {
				ID      string   `json:"id"`
				Area    string   `json:"area"`
				NameKey string   `json:"nameKey"`
				WhyKey  string   `json:"whyKey"`
				State   string   `json:"state"`
				Changes []change `json:"changes"`
			}
			out := make([]tweak, 0, len(list))
			for _, t := range list {
				cs := make([]change, 0, len(t.Changes))
				for _, c := range t.Changes {
					cs = append(cs, change{x.redact(c.Path), x.redact(c.Name), c.Existed, c.OldType, x.redact(short(c.Old, maxText)), c.Type, c.Private, c.Error})
				}
				out = append(out, tweak{x.redact(t.ID), x.redact(t.Area), t.NameKey, t.WhyKey, t.State, cs})
			}
			out, cut := capped(out, limit)
			return mcpkit.JSON(map[string]any{"settings": out, "truncated": cut}), nil
		})
	return []mcpkit.Tool{feat, wsl, priv}
}

type featureRow struct {
	ID             string `json:"id"`
	State          string `json:"state"`
	RestartPending bool   `json:"restartPending"`
	NeedsAdmin     bool   `json:"needsAdmin"`
}

type distroRow struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Version   int    `json:"version"`
	IsDefault bool   `json:"isDefault"`
}

type wslRow struct {
	Installed      bool        `json:"installed"`
	DefaultVersion int         `json:"defaultVersion"`
	Distros        []distroRow `json:"distros"`
	Truncated      bool        `json:"truncated"`
}

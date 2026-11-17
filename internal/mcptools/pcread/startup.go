package pcread

import (
	"context"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// listArgs decodes the arguments of a tool that only takes an optional limit.
func listArgs(x *tc) (int, *mcpkit.Result) {
	var a struct {
		Limit *int `json:"limit"`
	}
	if err := mcpkit.Decode(x.c.Args, &a); err != nil {
		return 0, mcpkit.Fail("%s", err)
	}
	limit, err := limitArg(a.Limit)
	if err != nil {
		return 0, mcpkit.Fail("%s", err)
	}
	return limit, nil
}

const limitOnly = `{"type":"object","properties":{` + limitProp + `},"additionalProperties":false}`

func (g *group) startupTools() []mcpkit.Tool {
	st := g.read("winforge_startup_list", "Programs that start with Windows",
		"Lists the programs that start with Windows (Run keys, the Startup folders and the Task Manager on/off flags): name, where it is registered, publisher, whether it is enabled, whether switching it needs administrator rights, whether its program is missing, and the command line (shortened). "+
			"PRIVACY: it describes software installed on this PC and the model sees it; command lines and paths have the user name and user folder removed. It NEVER switches, removes or starts anything.",
		mcpkit.Schema(limitOnly),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			limit, bad := listArgs(x)
			if bad != nil {
				return bad, nil
			}
			items, err := runCtx(ctx, g.d.StartupList)
			if err != nil {
				return nil, err
			}
			type item struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Source     string `json:"source"`
				Command    string `json:"command"`
				Target     string `json:"target,omitempty"`
				Publisher  string `json:"publisher,omitempty"`
				Enabled    bool   `json:"enabled"`
				NeedsAdmin bool   `json:"needsAdmin"`
				Missing    bool   `json:"missing"`
				Caution    bool   `json:"caution"`
			}
			out := make([]item, 0, len(items))
			for _, it := range items {
				out = append(out, item{x.redact(it.ID), x.redact(it.Name), it.Source, x.redact(short(it.Command, maxText)), x.redact(it.Target),
					x.redact(it.Publisher), it.Enabled, it.NeedsAdmin, it.Missing, it.Caution})
			}
			out, cut := capped(out, limit)
			return mcpkit.JSON(map[string]any{"items": out, "truncated": cut}), nil
		})

	sv := g.read("winforge_services_list", "Third-party services",
		"Lists the third-party Windows services exactly as WinForge's Startup page shows them (Microsoft and protected services are hidden there, and stay hidden here): name, display name, publisher, state, start mode, program, whether it looks like a security product and which services depend on it. "+
			"PRIVACY: it describes software installed on this PC and the model sees it; paths have the user name and user folder removed. It NEVER starts, stops or reconfigures a service.",
		mcpkit.Schema(limitOnly),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			limit, bad := listArgs(x)
			if bad != nil {
				return bad, nil
			}
			items, err := runCtx(ctx, g.d.ServicesList)
			if err != nil {
				return nil, err
			}
			type item struct {
				Name        string   `json:"name"`
				DisplayName string   `json:"displayName"`
				Publisher   string   `json:"publisher,omitempty"`
				State       string   `json:"state"`
				StartMode   string   `json:"startMode"`
				Binary      string   `json:"binary"`
				Caution     bool     `json:"caution"`
				Dependents  []string `json:"dependents"`
			}
			out := make([]item, 0, len(items))
			for _, it := range items {
				deps := make([]string, 0, len(it.Dependents))
				for _, d := range it.Dependents {
					deps = append(deps, x.redact(d))
				}
				deps, _ = capped(deps, 10)
				out = append(out, item{x.redact(it.Name), x.redact(it.DisplayName), x.redact(it.Publisher), string(it.State), string(it.StartMode),
					x.redact(short(it.Binary, maxText)), it.Caution, deps})
			}
			out, cut := capped(out, limit)
			return mcpkit.JSON(map[string]any{"items": out, "truncated": cut}), nil
		})

	tk := g.read("winforge_tasks_list", "Third-party scheduled tasks",
		"Lists the third-party scheduled tasks exactly as WinForge's Startup page shows them (Microsoft tasks are hidden there, and stay hidden here): name, folder, triggers, the program it runs (shortened), state, enabled, last result, and whether it looks like a security product. "+
			"PRIVACY: it describes software installed on this PC and the model sees it; paths have the user name and user folder removed. It NEVER runs, enables, disables or deletes a task.",
		mcpkit.Schema(limitOnly),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			limit, bad := listArgs(x)
			if bad != nil {
				return bad, nil
			}
			items, err := runCtx(ctx, g.d.TasksList)
			if err != nil {
				return nil, err
			}
			type item struct {
				Name       string   `json:"name"`
				Folder     string   `json:"folder"`
				Triggers   []string `json:"triggers"`
				Action     string   `json:"action"`
				State      string   `json:"state"`
				Enabled    bool     `json:"enabled"`
				LastResult *int32   `json:"lastResult"`
				NeedsAdmin bool     `json:"needsAdmin"`
				Caution    bool     `json:"caution"`
			}
			out := make([]item, 0, len(items))
			for _, it := range items {
				trig := make([]string, 0, len(it.Triggers))
				for _, t := range it.Triggers {
					trig = append(trig, x.redact(short(t, 80)))
				}
				trig, _ = capped(trig, 10)
				out = append(out, item{x.redact(it.Name), x.redact(it.Folder), trig, x.redact(short(it.Action, maxText)), string(it.State),
					it.Enabled, it.LastResult, it.NeedsAdmin, it.Caution})
			}
			out, cut := capped(out, limit)
			return mcpkit.JSON(map[string]any{"items": out, "truncated": cut}), nil
		})
	return []mcpkit.Tool{st, sv, tk}
}

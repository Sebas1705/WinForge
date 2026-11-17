package mcpserver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/mcptools/actions"
	"github.com/Sebas1705/WinForge/internal/mcptools/pcread"
	"github.com/Sebas1705/WinForge/internal/mcptools/sysapps"
)

// Registry is the set of tools the server knows, independent of the mode.
type Registry struct {
	tools  []mcpkit.Tool // sorted by name
	byName map[string]int
}

// NewRegistry collects tool groups. A nameless tool, a tool without handler, a bad schema or a duplicate
// name is an error, so a mistake shows at start-up and not when the assistant calls the tool.
func NewRegistry(groups ...[]mcpkit.Tool) (*Registry, error) {
	r := &Registry{byName: map[string]int{}}
	var all []mcpkit.Tool
	seen := map[string]bool{}
	for _, g := range groups {
		for _, t := range g {
			if strings.TrimSpace(t.Name) == "" {
				return nil, fmt.Errorf("a tool has no name")
			}
			if seen[t.Name] {
				return nil, fmt.Errorf("duplicate tool name %q", t.Name)
			}
			seen[t.Name] = true
			if t.Handler == nil {
				return nil, fmt.Errorf("tool %q has no handler", t.Name)
			}
			if len(t.InputSchema) == 0 {
				t.InputSchema = mcpkit.NoArgs
			}
			var m map[string]any
			if err := json.Unmarshal(t.InputSchema, &m); err != nil || m["type"] != "object" {
				return nil, fmt.Errorf("tool %q: input schema is not a JSON object schema", t.Name)
			}
			if t.Class < mcpkit.Read || t.Class > mcpkit.Act {
				return nil, fmt.Errorf("tool %q: unknown class", t.Name)
			}
			all = append(all, t)
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	r.tools = all
	for i, t := range all {
		r.byName[t.Name] = i
	}
	return r, nil
}

// Tool finds a tool by name.
func (r *Registry) Tool(name string) (mcpkit.Tool, bool) {
	i, ok := r.byName[name]
	if !ok {
		return mcpkit.Tool{}, false
	}
	return r.tools[i], true
}

// Tools returns every tool, sorted by name.
func (r *Registry) Tools() []mcpkit.Tool { return append([]mcpkit.Tool(nil), r.tools...) }

// RealRegistry is the registry of the shipped server: the generic plan tools plus the three tool groups.
func RealRegistry(env *mcpkit.Env, store *PlanStore) (*Registry, error) {
	return NewRegistry(Builtin(store), sysapps.Tools(env), pcread.Tools(env), actions.Tools(env))
}

// Annotations are the MCP tool hints.
type Annotations struct {
	ReadOnly    bool `json:"readOnlyHint"`
	Destructive bool `json:"destructiveHint"`
	Idempotent  bool `json:"idempotentHint"`
	OpenWorld   bool `json:"openWorldHint"`
}

// ToolInfo is one entry of tools/list and of --list-tools.
type ToolInfo struct {
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations Annotations     `json:"annotations"`
}

// ToolList returns the tools a mode offers. Tools the mode does not allow are not in it.
func (r *Registry) ToolList(m Mode) []ToolInfo {
	out := []ToolInfo{}
	for _, t := range r.tools {
		if !m.Allows(t.Class) {
			continue
		}
		out = append(out, ToolInfo{
			Name: t.Name, Title: t.Title, Description: t.Description, InputSchema: t.InputSchema,
			Annotations: Annotations{
				ReadOnly:    t.Class != mcpkit.Act,
				Destructive: t.Class == mcpkit.Act,
				Idempotent:  t.Class == mcpkit.Read,
				OpenWorld:   false,
			},
		})
	}
	return out
}

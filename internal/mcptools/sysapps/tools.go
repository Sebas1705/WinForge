// Package sysapps is the MCP tool group for software on this PC: what WinForge's catalog knows, what is
// installed, what can be updated, profiles, plans, end of support, history and comparing setups, plus the
// three Prepare tools (install, update, uninstall) that build a plan and store it for winforge_apply_plan.
//
// Every tool here is data in, data out: Read tools change nothing, Prepare tools store a plan and change
// nothing. Running a plan is the server's generic apply tool, which calls the Run closure built here and
// goes through the same runner as winforge-cli apply (internal/planrun). See internal/mcpkit for the contract.
package sysapps

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/eol"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/system"
)

// Deps is what the tools need beyond mcpkit.Env. The zero value means the real machine; tests replace the
// fields they care about. Stores live under Env.AppData, so tests point that at a temp folder.
type Deps struct {
	// LookPath finds an executable on PATH (winget). Default: exec.LookPath.
	LookPath func(string) (string, error)
	// Upgrades lists packages with a newer version (winget upgrade). Default: system.WingetUpgrades.
	Upgrades func(context.Context) ([]system.Upgrade, error)
	// EOL loads the embedded end-of-support table. Default: the table embedded in the program.
	EOL func() (*eol.Data, error)
}

var (
	eolOnce sync.Once
	eolData *eol.Data
	eolErr  error
)

func (d Deps) withDefaults() Deps {
	if d.LookPath == nil {
		d.LookPath = exec.LookPath
	}
	if d.Upgrades == nil {
		d.Upgrades = system.WingetUpgrades
	}
	if d.EOL == nil {
		d.EOL = func() (*eol.Data, error) {
			eolOnce.Do(func() { eolData, eolErr = eol.Load(catalogdata.FS) })
			return eolData, eolErr
		}
	}
	return d
}

// Tools returns this group's tools for the real machine.
func Tools(env *mcpkit.Env) []mcpkit.Tool { return ToolsWith(env, Deps{}) }

// ToolsWith is Tools with injected dependencies, for tests.
func ToolsWith(env *mcpkit.Env, d Deps) []mcpkit.Tool {
	r := &rt{fallback: env, d: d.withDefaults()}
	return []mcpkit.Tool{
		r.statusTool(),
		r.catalogSearchTool(),
		r.installedAppsTool(),
		r.updatesCheckTool(),
		r.profilesListTool(),
		r.profileGetTool(),
		r.installPlanTool(),
		r.eolReportTool(),
		r.historyTool(),
		r.compareProfileTool(),
		r.prepareInstallTool(),
		r.prepareUpdateTool(),
		r.prepareUninstallTool(),
	}
}

// rt is the shared state of the group's handlers.
type rt struct {
	fallback *mcpkit.Env
	d        Deps
}

// Limits shared by the list tools.
const (
	defaultLimit = 50
	maxLimit     = 200
	maxIDs       = 200
	maxQuery     = 100
)

// idPattern is the shape of a catalog app or profile id (see catalog.validID).
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

const (
	idSchema    = `{"type":"string","pattern":"^[a-z0-9][a-z0-9-]{0,62}$","maxLength":63}`
	limitSchema = `{"type":"integer","minimum":1,"maximum":200,"default":50,"description":"how many items to return (default 50, max 200)"}`
)

func idsSchema(desc string) string {
	return fmt.Sprintf(`{"type":"array","items":%s,"minItems":1,"maxItems":%d,"uniqueItems":true,"description":%q}`, idSchema, maxIDs, desc)
}

// env returns the environment of this call.
func (r *rt) env(c *mcpkit.Call) *mcpkit.Env {
	if c != nil && c.Env != nil {
		return c.Env
	}
	if r.fallback != nil {
		return r.fallback
	}
	return &mcpkit.Env{}
}

func redactor(e *mcpkit.Env) func(string) string {
	if e.Redact != nil {
		return e.Redact
	}
	return func(s string) string { return s }
}

func now(e *mcpkit.Env) time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

func elevated(e *mcpkit.Env) bool { return e.Elevated != nil && e.Elevated() }

// catalogOf loads the catalog or returns the tool error to send.
func (r *rt) catalogOf(e *mcpkit.Env) (*catalog.Catalog, *mcpkit.Result) {
	if e.Catalog == nil {
		return nil, mcpkit.Fail("the catalog is not available")
	}
	cat, err := e.Catalog()
	if err != nil || cat == nil {
		return nil, mcpkit.Fail("could not read the catalog: %s", redactor(e)(fmt.Sprint(err)))
	}
	return cat, nil
}

// appDataOf returns WinForge's own folder.
func appDataOf(e *mcpkit.Env) (string, error) {
	if e.AppData == nil {
		return "", errors.New("WinForge's data folder is not available")
	}
	p, err := e.AppData()
	if err != nil || p == "" {
		return "", errors.New("WinForge's data folder is not available")
	}
	return p, nil
}

func inAppData(e *mcpkit.Env, name string) (string, error) {
	base, err := appDataOf(e)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, name), nil
}

// decode reads strict arguments; a non-nil result is the error to return.
func decode(c *mcpkit.Call, v any) *mcpkit.Result {
	if err := mcpkit.Decode(c.Args, v); err != nil {
		return mcpkit.Fail("%s", err.Error())
	}
	return nil
}

// limitOf validates the optional limit argument.
func limitOf(l *int) (int, error) {
	if l == nil {
		return defaultLimit, nil
	}
	if *l < 1 || *l > maxLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maxLimit)
	}
	return *l, nil
}

// capList returns at most limit items and whether anything was cut.
func capList[T any](all []T, limit int) ([]T, bool) {
	if all == nil {
		return []T{}, false
	}
	if len(all) > limit {
		return all[:limit], true
	}
	return all, false
}

func short(s string) string {
	r := []rune(s)
	if len(r) > 40 {
		return string(r[:40]) + "..."
	}
	return s
}

// checkIDs validates a list of ids from the model: bounded, well formed, without duplicates (the first
// occurrence wins). Nothing is looked up here.
func checkIDs(field string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%s must list at least one id", field)
	}
	if len(ids) > maxIDs {
		return nil, fmt.Errorf("%s lists %d ids; the most allowed is %d", field, len(ids), maxIDs)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !idPattern.MatchString(id) {
			return nil, fmt.Errorf("%s: %q is not an app id (lowercase letters, digits and dashes, up to 63 characters)", field, short(id))
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, nil
}

// checkID validates one id.
func checkID(field, id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%s: %q is not a valid id (lowercase letters, digits and dashes, up to 63 characters)", field, short(id))
	}
	return nil
}

// knownApps checks every id against the catalog and returns the unknown ones as one error.
func knownApps(cat *catalog.Catalog, ids []string) error {
	var bad []string
	for _, id := range ids {
		if cat.Apps[id] == nil {
			bad = append(bad, id)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	if len(bad) > 10 {
		bad = append(bad[:10], "...")
	}
	return fmt.Errorf("not in the catalog: %s (nothing was stored; use winforge_catalog_search to find ids)", strings.Join(bad, ", "))
}

// scanned is one look at the PC's installed catalog apps.
type scanned struct {
	installed map[string]system.Installed
	warning   string // set when the inventory was degraded (still usable)
	noWinget  bool
}

// scan takes the inventory once, the way every page and the CLI do. Only a cancelled context is an error:
// without winget the registry and PATH still say something, and the warning says it is incomplete.
func (r *rt) scan(ctx context.Context, e *mcpkit.Env, cat *catalog.Catalog) (scanned, error) {
	if e.Snapshot == nil {
		return scanned{}, errors.New("scanning this PC is not available")
	}
	inv, err := e.Snapshot(ctx)
	if cerr := ctx.Err(); cerr != nil {
		return scanned{}, cerr
	}
	s := scanned{installed: system.Detect(cat, inv)}
	if err != nil {
		s.noWinget = errors.Is(err, system.ErrNoWinget)
		s.warning = redactor(e)(err.Error())
	}
	return s, nil
}

func versionsOf(m map[string]system.Installed) map[string]string {
	out := make(map[string]string, len(m))
	for id, i := range m {
		out[id] = i.Version
	}
	return out
}

func warnings(w ...string) []string {
	out := []string{}
	for _, s := range w {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func appName(cat *catalog.Catalog, id string) string {
	if a := cat.Apps[id]; a != nil {
		return a.Name
	}
	return id
}

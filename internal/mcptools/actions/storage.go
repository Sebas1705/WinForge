package actions

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/storage"
)

// ---- winforge_prepare_storage_clean ----

func storageCleanSchema() []byte {
	ids := make([]string, len(storage.Specs))
	for i, s := range storage.Specs {
		ids[i] = s.ID
	}
	return schemaJSON(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"categoryIds": map[string]any{
				"type":        "array",
				"description": "Cleanup category ids to clean, from the fixed list of categories. Each is re-measured now and refused when it is unknown, not present on this PC, or needs administrator rights WinForge does not have.",
				"items":       map[string]any{"type": "string", "enum": ids},
				"minItems":    1,
				"maxItems":    len(ids),
				"uniqueItems": true,
			},
		},
		"required":             []string{"categoryIds"},
		"additionalProperties": false,
	})
}

type storageCleanArgs struct {
	CategoryIDs []string `json:"categoryIds"`
}

// storageItem is one category in a storage plan.
type storageItem struct {
	ID         string   `json:"id"`
	Risk       string   `json:"risk"`
	NeedsAdmin bool     `json:"needsAdmin"`
	Bytes      int64    `json:"bytes"`
	Size       string   `json:"size"`
	Files      int      `json:"files"`
	How        string   `json:"how"` // "folders" or "commands"
	Folders    []string `json:"folders,omitempty"`
	Commands   []string `json:"commands,omitempty"`
	Effect     string   `json:"effect"`
}

type storageSummary struct {
	Headline   string        `json:"headline"`
	Categories []storageItem `json:"categories"`
	TotalBytes int64         `json:"totalBytes"`
	Total      string        `json:"total"`
	Warnings   []string      `json:"warnings,omitempty"`
	Undo       string        `json:"undo"`
}

func (k *kit) storageCleanTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_storage_clean",
		Title: "Prepare a storage cleanup",
		Description: "Prepares cleaning the chosen storage categories (temporary files, caches, the Recycle Bin, Windows.old, " +
			"developer caches, Docker build cache and dangling images). Only the fixed categories of WinForge's Storage page can be " +
			"named; each one is measured again now and the plan lists its size, file count, risk, whether it needs administrator " +
			"rights, the folders whose contents are removed (the folders themselves stay) and, for Recycle Bin, Windows.old and " +
			"Docker, the exact commands that will run. Docker containers and volumes are never touched. Categories that need " +
			"administrator rights are refused now when WinForge is not elevated. Removed files are not moved to the Recycle Bin " +
			"and cannot be undone; caches are rebuilt by Windows and the apps, and the Recycle Bin and Windows.old do not come " +
			"back. When the plan runs it measures again and refuses, changing nothing, if a category vanished or grew a lot." + prepNote,
		InputSchema: storageCleanSchema(),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareStorageClean,
	}
}

func (k *kit) prepareStorageClean(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Storage == nil {
		return notAvailable("storage cleanup")
	}
	var a storageCleanArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	ids := dedupe(a.CategoryIDs)
	if len(ids) == 0 {
		return refuse("no category was chosen")
	}
	for _, id := range ids {
		if _, ok := storage.SpecFor(id); !ok {
			return refuse("%q is not a storage category", id)
		}
	}
	progress(c, 0, 0, "Measuring the categories")
	cats, err := k.d.Storage.Scan(ctx, nil)
	if err != nil {
		return nil, err
	}
	by := map[string]storage.Category{}
	for _, cat := range cats {
		by[cat.ID] = cat
	}
	var items []storageItem
	var total int64
	var warnings []string
	admin := false
	var needAdmin []string
	for _, id := range ids {
		cat, ok := by[id]
		if !ok || !cat.Available {
			return refuse("category %q is not present on this PC, so there is nothing to clean", id)
		}
		if cat.NeedsAdmin {
			admin = true
			if !k.elevated() {
				needAdmin = append(needAdmin, id)
			}
		}
		items = append(items, k.describeCategory(cat))
		total += cat.Bytes
	}
	if len(needAdmin) > 0 {
		return refuse("these categories need administrator rights and WinForge is not running elevated: %s. Nothing was prepared; ask the person to start WinForge as administrator or leave them out", strings.Join(needAdmin, ", "))
	}
	for _, it := range items {
		if it.Risk == string(storage.Review) {
			warnings = append(warnings, fmt.Sprintf("%s is a \"review\" category: %s", it.ID, reviewWhy(it.ID)))
		}
	}
	sum := storageSummary{
		Headline:   fmt.Sprintf("Permanently remove the contents of %d storage categor%s (about %s)", len(items), plural(len(items), "y", "ies"), human(total)),
		Categories: items,
		TotalBytes: total,
		Total:      human(total),
		Warnings:   warnings,
		Undo:       "Not undoable: the files are deleted, not moved to the Recycle Bin. Caches are rebuilt by Windows and the apps as needed.",
	}
	planned := map[string]int64{}
	for _, it := range items {
		planned[it.ID] = it.Bytes
	}
	return k.store(c, "storage-clean", sum, admin, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		return k.runStorageClean(ctx, c, ids, planned)
	})
}

func (k *kit) describeCategory(cat storage.Category) storageItem {
	it := storageItem{ID: cat.ID, Risk: string(cat.Risk), NeedsAdmin: cat.NeedsAdmin, Bytes: cat.Bytes, Size: human(cat.Bytes), Files: cat.Files}
	if cmds := storage.CommandsFor(cat.ID, k.d.StorageEnv); len(cmds) > 0 {
		it.How = "commands"
		for _, cmd := range cmds {
			it.Commands = append(it.Commands, k.redact(strings.Join(cmd, " ")))
		}
		switch cat.ID {
		case "recycle-bin":
			it.Effect = "Empties the Recycle Bin."
		case "windows-old":
			it.Effect = "Takes ownership of the Windows.old folder and deletes it; this removes the way back to the previous Windows version."
		case "docker-build-cache":
			it.Effect = "Docker removes its whole build cache; images, containers and volumes are not touched. The next build is slower."
		case "docker-dangling":
			it.Effect = "Docker removes images that have no name or tag; containers and volumes are not touched."
		}
		return it
	}
	it.How = "folders"
	it.Folders = k.redactAll(storage.Folders(cat.ID, k.d.StorageEnv))
	spec, _ := storage.SpecFor(cat.ID)
	effect := "Deletes the files inside these folders, then the folders left empty; the folders themselves stay."
	if len(spec.Files) > 0 {
		effect += " Only files named " + strings.Join(spec.Files, ", ") + "."
	}
	if spec.MinAge > 0 {
		effect += fmt.Sprintf(" Files newer than %s are kept.", spec.MinAge.Round(time.Hour))
	}
	it.Effect = effect
	return it
}

func reviewWhy(id string) string {
	switch id {
	case "windows-old":
		return "removing it ends the option to go back to the previous Windows version"
	case "shader-cache":
		return "games and apps rebuild it, so the first launch afterwards is slower"
	case "docker-build-cache", "docker-dangling":
		return "Docker has to download or build again what it removed"
	case "dev-pnpm", "dev-gradle", "dev-cargo", "dev-gradle-wrapper":
		return "it is downloaded again on the next build, which needs a connection and takes time"
	}
	return "it has a cost the person should know before cleaning"
}

func (k *kit) runStorageClean(ctx context.Context, c *mcpkit.Call, ids []string, planned map[string]int64) (*mcpkit.Result, error) {
	progress(c, 0, float64(len(ids)), "Checking the plan is still valid")
	cats, err := k.d.Storage.Scan(ctx, nil)
	if err != nil {
		return nil, err
	}
	by := map[string]storage.Category{}
	for _, cat := range cats {
		by[cat.ID] = cat
	}
	for _, id := range ids {
		cat, ok := by[id]
		if !ok || !cat.Available {
			return refuse("category %q is no longer present, so the plan was not run. Nothing was changed; prepare it again", id)
		}
		if cat.NeedsAdmin && !k.elevated() {
			return refuse("category %q needs administrator rights and WinForge no longer has them. Nothing was changed", id)
		}
		if grown := cat.Bytes - planned[id]; grown > max(planned[id]/4, 256<<20) {
			return refuse("category %q is now %s, much more than the %s the plan showed. Nothing was changed; prepare it again so the person sees the new size", id, human(cat.Bytes), human(planned[id]))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	res := k.d.Storage.Clean(ctx, ids, func(p storage.Progress) {
		progress(c, float64(p.Files), 0, fmt.Sprintf("%s: %d files, %s%s", p.Task, p.Files, human(p.Bytes), k.currentSuffix(p.Current)))
	})
	out := storageResult{FreedBytes: res.FreedBytes, Freed: human(res.FreedBytes), Files: res.Files, Failed: res.Failed, Categories: []storageOutcome{},
		Undo: "Not undoable: the files were deleted. Caches are rebuilt by Windows and the apps as needed."}
	errs := 0
	for _, o := range res.Outcomes {
		out.Categories = append(out.Categories, storageOutcome{ID: o.ID, FreedBytes: o.FreedBytes, Files: o.Files, Failed: o.Failed, Error: k.redact(o.Error)})
		if o.Error != "" {
			errs++
		}
	}
	if len(ids) > len(res.Outcomes) {
		out.Cancelled = true
		out.Note = fmt.Sprintf("Stopped early: %d of %d categories were processed.", len(res.Outcomes), len(ids))
	}
	return outcome(out, len(res.Outcomes) > 0 && errs == len(res.Outcomes) && res.FreedBytes == 0), nil
}

func (k *kit) currentSuffix(cur string) string {
	if cur == "" {
		return ""
	}
	return " - " + k.redact(cur)
}

type storageOutcome struct {
	ID         string `json:"id"`
	FreedBytes int64  `json:"freedBytes"`
	Files      int    `json:"files"`
	Failed     int    `json:"failed"`
	Error      string `json:"error,omitempty"`
}

type storageResult struct {
	FreedBytes int64            `json:"freedBytes"`
	Freed      string           `json:"freed"`
	Files      int              `json:"files"`
	Failed     int              `json:"failed"`
	Categories []storageOutcome `json:"categories"`
	Cancelled  bool             `json:"cancelled,omitempty"`
	Note       string           `json:"note,omitempty"`
	Undo       string           `json:"undo"`
}

// ---- winforge_prepare_remove_project_junk ----

type junkArgs struct {
	Paths   []string `json:"paths"`
	Roots   []string `json:"roots"`
	MinDays int      `json:"minDays"`
}

const defaultJunkDays = 90

func (k *kit) projectJunkTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_remove_project_junk",
		Title: "Prepare removing old project build folders",
		Description: "Prepares deleting dependency and build folders (node_modules, target, bin/obj, build...) of projects nobody has " +
			"touched for minDays days or more. `roots` are the folders to search; a fresh search runs now and only folders it lists " +
			"right now can be named in `paths`, so no other path is ever accepted. The plan lists each folder with its project, kind, " +
			"size, file count and last-touched date. Deleting is PERMANENT: the folders do not go to the Recycle Bin and cannot be " +
			"undone here (the project's own install or build command recreates them). Links inside are removed as links and never " +
			"followed, and every folder is proved again at the moment it is removed. No administrator rights are needed." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{` +
			`"paths":{"type":"array","items":{"type":"string","maxLength":1024},"minItems":1,"maxItems":200,"uniqueItems":true,"description":"Folders to remove, exactly as a project search lists them now."},` +
			`"roots":{"type":"array","items":{"type":"string","maxLength":1024},"minItems":1,"maxItems":20,"description":"Folders to search for projects (for example the person's code folder)."},` +
			`"minDays":{"type":"integer","minimum":1,"maximum":3650,"default":90,"description":"Only projects left alone for at least this many days are listed."}` +
			`},"required":["paths","roots"],"additionalProperties":false}`),
		Class:   mcpkit.Prepare,
		Handler: k.prepareProjectJunk,
	}
}

func pathKey(p string) string { return strings.ToLower(filepath.Clean(p)) }

type junkItem struct {
	Path        string `json:"path"`
	Project     string `json:"project"`
	Kind        string `json:"kind"`
	SizeBytes   int64  `json:"sizeBytes"`
	Size        string `json:"size"`
	Files       int    `json:"files"`
	LastTouched string `json:"lastTouched"`
}

type junkSummary struct {
	Headline   string     `json:"headline"`
	Folders    []junkItem `json:"folders"`
	TotalBytes int64      `json:"totalBytes"`
	Total      string     `json:"total"`
	Warnings   []string   `json:"warnings"`
	Undo       string     `json:"undo"`
}

func (k *kit) prepareProjectJunk(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Storage == nil {
		return notAvailable("the project folder search")
	}
	a := junkArgs{MinDays: defaultJunkDays}
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	if len(a.Paths) == 0 || len(a.Roots) == 0 {
		return refuse("paths and roots are both required")
	}
	if a.MinDays < 1 || a.MinDays > 3650 {
		return refuse("minDays must be between 1 and 3650")
	}
	progress(c, 0, 0, "Searching the folders for old projects")
	found, err := k.d.Storage.FindProjectJunk(ctx, a.Roots, time.Duration(a.MinDays)*24*time.Hour, func(p storage.Progress) {
		progress(c, float64(p.Files), 0, "searching"+k.currentSuffix(p.Current))
	})
	if err != nil {
		return refuse("the search could not run: %s", k.redact(err.Error()))
	}
	keys := make([]string, len(found))
	for i, j := range found {
		keys[i] = j.Path
	}
	var items []junkItem
	var chosen []string
	var total int64
	seen := map[string]bool{}
	var unknown []string
	for _, p := range a.Paths {
		// A model reads paths through the server's redaction, so the redacted form of a listed path is accepted too;
		// the listed (real) path is what the plan keeps.
		at, ok, _ := k.match(p, keys, pathKey)
		if !ok {
			unknown = append(unknown, k.redact(p))
			continue
		}
		j := found[at]
		key := pathKey(j.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		chosen = append(chosen, j.Path)
		total += j.SizeBytes
		items = append(items, junkItem{Path: k.redact(j.Path), Project: k.redact(j.Project), Kind: j.Kind, SizeBytes: j.SizeBytes, Size: human(j.SizeBytes),
			Files: j.Files, LastTouched: j.LastTouched.UTC().Format("2006-01-02")})
	}
	if len(unknown) > 0 {
		return refuse("these paths are not listed by a project search of the given roots right now, so they are refused: %s. Nothing was prepared", strings.Join(unknown, "; "))
	}
	sum := junkSummary{
		Headline:   fmt.Sprintf("PERMANENTLY delete %d project folder%s (about %s)", len(items), plural(len(items), "", "s"), human(total)),
		Folders:    items,
		TotalBytes: total,
		Total:      human(total),
		Warnings: []string{
			"Deletion is permanent: nothing goes to the Recycle Bin.",
			fmt.Sprintf("Only projects untouched for %d days or more were searched; each folder is proved again at the moment it is removed and refused if its project was touched since.", a.MinDays),
		},
		Undo: "Not undoable here. Running the project's own install or build command (npm install, cargo build, dotnet build...) recreates the folder.",
	}
	roots, minDays := append([]string(nil), a.Roots...), a.MinDays
	return k.store(c, "projects-remove", sum, false, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		return k.runProjectJunk(ctx, c, roots, minDays, chosen)
	})
}

type junkResult struct {
	FreedBytes int64    `json:"freedBytes"`
	Freed      string   `json:"freed"`
	Removed    int      `json:"removed"`
	Files      int      `json:"files"`
	Failed     []string `json:"failed"`
	Reasons    []string `json:"reasons"`
	Cancelled  bool     `json:"cancelled,omitempty"`
	Undo       string   `json:"undo"`
}

func (k *kit) runProjectJunk(ctx context.Context, c *mcpkit.Call, roots []string, minDays int, chosen []string) (*mcpkit.Result, error) {
	progress(c, 0, float64(len(chosen)), "Checking the plan is still valid")
	found, err := k.d.Storage.FindProjectJunk(ctx, roots, time.Duration(minDays)*24*time.Hour, nil)
	if err != nil {
		return refuse("the folders could not be checked again (%s). Nothing was changed", k.redact(err.Error()))
	}
	listed := map[string]bool{}
	for _, j := range found {
		listed[pathKey(j.Path)] = true
	}
	for _, p := range chosen {
		if !listed[pathKey(p)] {
			return refuse("%s is no longer listed by the project search (it was removed, or its project was touched). Nothing was changed; prepare the plan again", k.redact(p))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	res := k.d.Storage.RemoveProjectJunk(ctx, chosen, func(p storage.Progress) {
		progress(c, float64(p.Files), 0, "removing"+k.currentSuffix(p.Current))
	})
	out := junkResult{FreedBytes: res.FreedBytes, Freed: human(res.FreedBytes), Removed: res.Removed, Files: res.Files,
		Failed: k.redactAll(nonNilStrings(res.Failed)), Reasons: nonNilStrings(res.Reasons),
		Cancelled: ctx.Err() != nil, Undo: "Not undoable: the folders were deleted, not moved to the Recycle Bin. The project's own install or build command recreates them."}
	return outcome(out, res.Removed == 0 && len(res.Failed) > 0), nil
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func dedupe(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

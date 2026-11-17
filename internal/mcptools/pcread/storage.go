package pcread

import (
	"context"
	"errors"
	"time"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/storage"
)

// storageProgress turns the storage package's progress into a tool progress report. The current path is
// redacted by tc.progress.
func (x *tc) storageProgress(label string) storage.ProgressFunc {
	return func(p storage.Progress) {
		msg := label
		if p.Current != "" {
			msg += ": " + short(p.Current, 120)
		}
		x.progress(float64(p.Files), 0, msg)
	}
}

func (g *group) storageTools() []mcpkit.Tool {
	vols := g.read("winforge_storage_volumes", "Drives and free space",
		"Lists this PC's fixed drives with their label, total size and free space in bytes. It describes this PC's drives and is seen by the model (volume labels are redacted). It NEVER changes anything and reads no files.",
		mcpkit.NoArgs,
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			if err := mcpkit.Decode(x.c.Args, &struct{}{}); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			type vol struct {
				Letter     string `json:"letter"`
				Label      string `json:"label"`
				TotalBytes int64  `json:"totalBytes"`
				FreeBytes  int64  `json:"freeBytes"`
			}
			vs, err := runCtx(ctx, func() ([]storage.Volume, error) { return g.d.Volumes(), nil })
			if err != nil {
				return nil, err
			}
			out := make([]vol, 0, len(vs))
			for _, v := range vs {
				out = append(out, vol{x.redact(v.Letter), x.redact(v.Label), v.TotalBytes, v.FreeBytes})
			}
			return mcpkit.JSON(map[string]any{"volumes": out}), nil
		})

	scan := g.read("winforge_storage_scan", "Cleanup categories found",
		"Measures the cleanup categories WinForge's Cleanup page lists (temporary files, caches, Recycle Bin, Windows.old, Docker build cache and so on) and returns for each: id, risk, needsAdmin, available, bytes, files and skipped (files that could not be read). "+
			"It only counts file sizes inside WinForge's fixed list of cleanup folders; no file names or contents are returned. It NEVER deletes or cleans anything (cleaning is a separate, approved action). It can take a while and reports progress.",
		mcpkit.NoArgs,
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			if err := mcpkit.Decode(x.c.Args, &struct{}{}); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			x.progress(0, 0, "measuring cleanup categories")
			cats, err := g.d.StorageScan(ctx, x.elevated(), x.storageProgress("scanning"))
			if err != nil {
				return nil, err
			}
			type cat struct {
				ID         string `json:"id"`
				Risk       string `json:"risk"`
				NeedsAdmin bool   `json:"needsAdmin"`
				Available  bool   `json:"available"`
				Bytes      int64  `json:"bytes"`
				Files      int    `json:"files"`
				Skipped    int    `json:"skipped"`
			}
			out := make([]cat, 0, len(cats))
			var total int64
			for _, c := range cats {
				out = append(out, cat{c.ID, string(c.Risk), c.NeedsAdmin, c.Available, c.Bytes, c.Files, c.Skipped})
				total += c.Bytes
			}
			return mcpkit.JSON(map[string]any{"categories": out, "totalBytes": total, "ranAsAdmin": x.elevated()}), nil
		})

	biggest := g.read("winforge_storage_biggest", "Biggest files or folders",
		"Lists the biggest files (kind=files) or the biggest immediate sub-folders (kind=folders) under an absolute folder the person named, with sizes in bytes. "+
			"PRIVACY: it returns names, paths and sizes of files and folders on this PC, and the model sees them; paths and names have the user name and user folder removed. It never returns file contents. "+
			"It refuses relative paths, network (UNC) and device paths, paths with .., a whole drive unless allowDriveRoot is true (only set that when the person asked for the whole drive), and folders that do not exist. It does not follow links. It NEVER deletes, moves or opens anything. Scans can be slow and report progress.",
		mcpkit.Schema(`{"type":"object","properties":{"root":{"type":"string","description":"Absolute folder on a local drive, for example C:\\Users\\me\\Downloads."},"kind":{"type":"string","enum":["files","folders"]},`+limitProp+`,"allowDriveRoot":{"type":"boolean","default":false,"description":"Allow a whole drive such as C:\\ as the root. Only with the person's explicit request."}},"required":["root","kind"],"additionalProperties":false}`),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			var a struct {
				Root           string `json:"root"`
				Kind           string `json:"kind"`
				Limit          *int   `json:"limit"`
				AllowDriveRoot bool   `json:"allowDriveRoot"`
			}
			if err := mcpkit.Decode(x.c.Args, &a); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			if a.Kind != "files" && a.Kind != "folders" {
				return mcpkit.Fail(`kind must be "files" or "folders"`), nil
			}
			limit, err := limitArg(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			root, err := g.folder(a.Root, a.AllowDriveRoot)
			if err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			x.progress(0, 0, "looking in "+root)
			out := map[string]any{"root": x.redact(root), "kind": a.Kind}
			if a.Kind == "files" {
				type item struct {
					Path      string    `json:"path"`
					Name      string    `json:"name"`
					SizeBytes int64     `json:"sizeBytes"`
					Modified  time.Time `json:"modified"`
				}
				fs, err := g.d.BigFiles(ctx, root, limit+1, x.storageProgress("scanning"))
				if err != nil {
					return nil, err
				}
				items := make([]item, 0, len(fs))
				for _, f := range fs {
					items = append(items, item{x.redact(f.Path), x.redact(f.Name), f.SizeBytes, f.Modified})
				}
				var cut bool
				items, cut = capped(items, limit)
				out["items"], out["truncated"] = items, cut
			} else {
				type item struct {
					Path      string `json:"path"`
					Name      string `json:"name"`
					SizeBytes int64  `json:"sizeBytes"`
					Files     int    `json:"files"`
				}
				fs, err := g.d.FolderSizes(ctx, root, x.storageProgress("measuring"))
				if err != nil {
					return nil, err
				}
				items := make([]item, 0, len(fs))
				for _, f := range fs {
					items = append(items, item{x.redact(f.Path), x.redact(f.Name), f.SizeBytes, f.Files})
				}
				var cut bool
				items, cut = capped(items, limit)
				// The backend itself lists at most 60 folders.
				out["items"], out["truncated"] = items, cut || len(fs) >= 60
			}
			return mcpkit.JSON(out), nil
		})

	projects := g.read("winforge_storage_projects", "Old project build folders",
		"Finds build-output and dependency folders (node_modules, target, bin/obj, .venv and similar) of projects nobody has touched for at least minDays days, under the absolute folders the person named, with size, file count and when the project was last touched. "+
			"PRIVACY: it returns project names and paths on this PC and the model sees them; they have the user name and user folder removed. A folder is listed only when a file beside it proves what it is. "+
			"Same path rules as winforge_storage_biggest (absolute, no UNC or device paths, no .., no whole drive unless allowDriveRoot). It NEVER deletes anything. Scans can be slow and report progress.",
		mcpkit.Schema(`{"type":"object","properties":{"roots":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":8,"description":"Absolute folders to search under."},"minDays":{"type":"integer","minimum":1,"maximum":3650,"default":90,"description":"Only projects untouched for at least this many days."},`+limitProp+`,"allowDriveRoot":{"type":"boolean","default":false}},"required":["roots"],"additionalProperties":false}`),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			var a struct {
				Roots          []string `json:"roots"`
				MinDays        *int     `json:"minDays"`
				Limit          *int     `json:"limit"`
				AllowDriveRoot bool     `json:"allowDriveRoot"`
			}
			if err := mcpkit.Decode(x.c.Args, &a); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			if len(a.Roots) == 0 || len(a.Roots) > maxRoots {
				return mcpkit.Fail("roots must list between 1 and %d folders", maxRoots), nil
			}
			days := 90
			if a.MinDays != nil {
				days = *a.MinDays
			}
			if days < 1 || days > 3650 {
				return mcpkit.Fail("minDays must be between 1 and 3650 (default 90)"), nil
			}
			limit, err := limitArg(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			roots := make([]string, 0, len(a.Roots))
			for _, r := range a.Roots {
				p, err := g.folder(r, a.AllowDriveRoot)
				if err != nil {
					return mcpkit.Fail("%s: %s", x.redact(short(r, maxText)), err), nil
				}
				roots = append(roots, p)
			}
			x.progress(0, 0, "looking for old project folders")
			js, err := g.d.ProjectJunk(ctx, roots, time.Duration(days)*24*time.Hour, x.now(), x.storageProgress("searching"))
			if err != nil {
				return nil, err
			}
			type item struct {
				Path        string    `json:"path"`
				Project     string    `json:"project"`
				ProjectPath string    `json:"projectPath"`
				Kind        string    `json:"kind"`
				SizeBytes   int64     `json:"sizeBytes"`
				Files       int       `json:"files"`
				LastTouched time.Time `json:"lastTouched"`
			}
			items := make([]item, 0, len(js))
			var total int64
			for _, j := range js {
				items = append(items, item{x.redact(j.Path), x.redact(j.Project), x.redact(j.ProjectPath), j.Kind, j.SizeBytes, j.Files, j.LastTouched})
				total += j.SizeBytes
			}
			items, cut := capped(items, limit)
			return mcpkit.JSON(map[string]any{"minDays": days, "items": items, "totalBytes": total, "truncated": cut}), nil
		})

	vdisks := g.read("winforge_virtual_disks", "WSL and Docker disk images",
		"Lists the virtual disk images WSL distributions and Docker Desktop keep for this user (name, kind wsl or docker, size in bytes, last modified, path). They often hold far more space than their contents need. "+
			"PRIVACY: it returns file paths on this PC, seen by the model, with the user name and user folder removed. It NEVER compacts, shuts down, moves or deletes anything.",
		mcpkit.Schema(`{"type":"object","properties":{`+limitProp+`},"additionalProperties":false}`),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			var a struct {
				Limit *int `json:"limit"`
			}
			if err := mcpkit.Decode(x.c.Args, &a); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			limit, err := limitArg(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			ds, err := runCtx(ctx, func() ([]storage.VirtualDisk, error) { return g.d.VirtualDisks(), nil })
			if err != nil {
				return nil, err
			}
			type item struct {
				Path      string    `json:"path"`
				Name      string    `json:"name"`
				Kind      string    `json:"kind"`
				SizeBytes int64     `json:"sizeBytes"`
				Modified  time.Time `json:"modified"`
			}
			items := make([]item, 0, len(ds))
			for _, d := range ds {
				items = append(items, item{x.redact(d.Path), x.redact(d.Name), d.Kind, d.SizeBytes, d.Modified})
			}
			items, cut := capped(items, limit)
			return mcpkit.JSON(map[string]any{"disks": items, "truncated": cut}), nil
		})
	return []mcpkit.Tool{vols, scan, biggest, projects, vdisks}
}

// folder validates a named folder and checks it exists as a folder.
func (g *group) folder(p string, allowDriveRoot bool) (string, error) {
	clean, err := checkFolder(p, allowDriveRoot)
	if err != nil {
		return "", err
	}
	if err := g.d.CheckFolder(clean); err != nil {
		return "", errors.New("that folder does not exist or cannot be read")
	}
	return clean, nil
}

package storage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Clean removes the contents of the chosen categories and reports what it
// freed. Each category is independent: one that fails (a folder in use, no
// administrator rights) does not stop the others. Files that cannot be removed
// are left as they were and counted as failed.
func Clean(ctx context.Context, ids []string, o Options) Result {
	var res Result
	th := newThrottle("clean", o.Progress)
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out := cleanOne(ctx, id, o, th)
		res.Outcomes = append(res.Outcomes, out)
		res.FreedBytes += out.FreedBytes
		res.Files += out.Files
		res.Failed += out.Failed
		if ctx.Err() != nil {
			break
		}
	}
	th.done()
	if res.Outcomes == nil {
		res.Outcomes = []Outcome{}
	}
	return res
}

func cleanOne(ctx context.Context, id string, o Options, th *throttle) Outcome {
	out := Outcome{ID: id}
	s, ok := SpecFor(id)
	if !ok {
		out.Error = "unknown category"
		return out
	}
	if s.NeedsAdmin && !o.Admin {
		out.Error = "needs-admin"
		return out
	}
	switch s.Special {
	case "recycle-bin":
		return viaCommand(ctx, o, s, out, func() error {
			if o.Runner == nil {
				return errors.New("no way to run commands")
			}
			_, err := o.Runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
				"Clear-RecycleBin -Force -ErrorAction SilentlyContinue")
			return err
		})
	case "docker-build-cache":
		return docker(viaCommand(ctx, o, s, out, func() error {
			if o.Runner == nil {
				return errors.New("no way to run commands")
			}
			_, err := o.Runner.Run(ctx, "docker", "builder", "prune", "--all", "--force")
			return err
		}))
	case "docker-dangling":
		return docker(viaCommand(ctx, o, s, out, func() error {
			if o.Runner == nil {
				return errors.New("no way to run commands")
			}
			_, err := o.Runner.Run(ctx, "docker", "image", "prune", "--force")
			return err
		}))
	case "windows-old":
		return viaCommand(ctx, o, s, out, func() error {
			r := o.windowsOld()
			if r == "" || o.Runner == nil {
				return errors.New("no way to run commands")
			}
			// Windows.old belongs to TrustedInstaller: take it over, then remove it.
			if _, err := o.Runner.Run(ctx, "takeown.exe", "/F", r, "/R", "/D", "Y"); err != nil {
				return err
			}
			if _, err := o.Runner.Run(ctx, "icacls.exe", r, "/grant", "*S-1-5-32-544:F", "/T", "/C", "/Q"); err != nil {
				return err
			}
			_, err := o.Runner.Run(ctx, "cmd.exe", "/C", "rd", "/s", "/q", r)
			return err
		})
	}
	dirs, _ := roots(o.Env, s)
	now := o.now()
	for _, d := range dirs {
		f, n, failed := deleteContents(ctx, s, d, now, th)
		out.FreedBytes += f
		out.Files += n
		out.Failed += failed
	}
	return out
}

// docker drops the count of items left over: what Docker keeps is in use, not a failure to remove it.
func docker(o Outcome) Outcome {
	o.Failed = 0
	return o
}

// viaCommand runs a command-based category and measures what it freed by
// scanning before and after, so the numbers are real and not the command's claim.
func viaCommand(ctx context.Context, o Options, s Spec, out Outcome, run func() error) Outcome {
	var before, after Category
	th := newThrottle("scan", nil)
	_ = measure(ctx, o, s, &before, th)
	if err := run(); err != nil {
		out.Error = err.Error()
	}
	_ = measure(ctx, o, s, &after, th)
	if d := before.Bytes - after.Bytes; d > 0 {
		out.FreedBytes = d
	}
	if d := before.Files - after.Files; d > 0 {
		out.Files = d
	}
	out.Failed = after.Files
	return out
}

// deleteContents removes the files under root that the category accepts, then
// the folders that are left empty. root itself stays.
func deleteContents(ctx context.Context, s Spec, root string, now time.Time, th *throttle) (freed int64, files, failed int) {
	var dirs []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if werr != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			failed++
			return nil
		}
		if !plain(d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root {
				dirs = append(dirs, p)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			failed++
			return nil
		}
		if !accept(s, now, info.Name(), info.ModTime()) {
			return nil
		}
		if removeFile(p) {
			freed += info.Size()
			files++
			th.add(1, info.Size(), p)
		} else {
			failed++
		}
		return nil
	})
	if len(s.Files) == 0 {
		// Deepest first, so a folder holding only folders that were emptied goes too.
		sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
		for _, d := range dirs {
			if within(root, d) {
				_ = os.Remove(d) // fails, harmlessly, when something is still inside
			}
		}
	}
	return freed, files, failed
}

// removeFile deletes a file, clearing the read-only flag if that is what blocks it.
func removeFile(p string) bool {
	if err := os.Remove(p); err == nil || errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err := os.Chmod(p, 0o666); err == nil {
		if err := os.Remove(p); err == nil {
			return true
		}
	}
	return false
}

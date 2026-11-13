package storage

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Options configures a scan or a clean.
type Options struct {
	Env    Env
	Runner Runner    // for Recycle Bin and Windows.old; nil disables them
	Admin  bool      // whether the process is elevated
	Now    time.Time // zero means now; tests fix it
	// RecycleRoots are the "$Recycle.Bin" folders, one per drive. Nil means the
	// ones on this PC's fixed drives.
	RecycleRoots []string
	Progress     ProgressFunc
}

func (o Options) now() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

func (o Options) recycleRoots() []string {
	if o.RecycleRoots != nil {
		return o.RecycleRoots
	}
	var out []string
	for _, v := range Volumes() {
		out = append(out, v.Letter+string(filepath.Separator)+"$Recycle.Bin")
	}
	return out
}

func (o Options) windowsOld() string {
	if o.Env.SystemDrive == "" {
		return ""
	}
	return filepath.Join(o.Env.SystemDrive+string(filepath.Separator), "Windows.old")
}

// Scan measures every category without changing anything. The numbers are what
// Clean would remove: it applies the same folders, name patterns and age limit.
func Scan(ctx context.Context, o Options) ([]Category, error) {
	th := newThrottle("scan", o.Progress)
	out := make([]Category, len(Specs))
	err := parallel(len(Specs), func(i int) error {
		s := Specs[i]
		out[i] = Category{ID: s.ID, Risk: s.Risk, NeedsAdmin: s.NeedsAdmin}
		if err := ctx.Err(); err != nil {
			return err
		}
		return measure(ctx, o, s, &out[i], th)
	})
	th.done()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// measure fills Bytes, Files, Skipped and Available for one category.
func measure(ctx context.Context, o Options, s Spec, c *Category, th *throttle) error {
	now := o.now()
	count := func(p string, info fs.FileInfo) {
		c.Files++
		c.Bytes += info.Size()
		th.add(1, info.Size(), p)
	}
	switch s.Special {
	case "recycle-bin":
		for _, r := range o.recycleRoots() {
			if st, err := os.Lstat(r); err != nil || !st.IsDir() {
				continue
			}
			c.Available = true
			// Only the per-user folders inside it are readable, and only the person's own.
			skipped, err := walk(ctx, r, func(p string, info fs.FileInfo) { count(p, info) })
			c.Skipped += skipped
			if err != nil {
				return err
			}
		}
	case "windows-old":
		r := o.windowsOld()
		if r == "" {
			return nil
		}
		if st, err := os.Lstat(r); err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		c.Available = true
		skipped, err := walk(ctx, r, func(p string, info fs.FileInfo) { count(p, info) })
		c.Skipped += skipped
		if err != nil {
			return err
		}
	default:
		dirs, refused := roots(o.Env, s)
		c.Available = len(dirs) > 0
		c.Skipped += len(refused)
		for _, d := range dirs {
			skipped, err := walk(ctx, d, func(p string, info fs.FileInfo) {
				if accept(s, now, info.Name(), info.ModTime()) {
					count(p, info)
				}
			})
			c.Skipped += skipped
			if err != nil {
				return err
			}
		}
	}
	return nil
}

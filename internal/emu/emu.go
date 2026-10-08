// Package emu finds an installed emulator on disk and starts it on a game file.
// It only runs programs found by file name inside well-known install folders,
// with arguments taken from the catalog and the game's own path.
package emu

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

// maxDepth is how far below an install root a folder is searched.
const maxDepth = 3

// DefaultRoots are the folders emulators usually end up in, whether installed by
// an installer, by winget, or unpacked as a portable app.
func DefaultRoots() []string {
	var roots []string
	add := func(p string) {
		if p != "" {
			roots = append(roots, p)
		}
	}
	local := os.Getenv("LOCALAPPDATA")
	add(os.Getenv("ProgramFiles"))
	add(os.Getenv("ProgramFiles(x86)"))
	if local != "" {
		add(filepath.Join(local, "Programs"))
		add(filepath.Join(local, "Microsoft", "WinGet", "Packages"))
		add(filepath.Join(local, "Microsoft", "WinGet", "Links"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "scoop", "apps"))
	}
	return roots
}

// Locate finds the emulator's program: on PATH first, then in a folder under
// one of roots whose name contains one of the spec's hints. It returns "" when
// the emulator cannot be found.
func Locate(spec catalog.RunSpec, roots []string) string {
	for _, exe := range spec.Exes {
		if p, err := exec.LookPath(exe); err == nil {
			return p
		}
	}
	for _, root := range roots {
		if p := searchRoot(root, spec); p != "" {
			return p
		}
	}
	return ""
}

func searchRoot(root string, spec catalog.RunSpec) string {
	var found string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fs.SkipDir
		}
		if !d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return fs.SkipDir
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(filepath.Separator)) + 1
		}
		if depth > maxDepth {
			return fs.SkipDir
		}
		// The hint may be in this folder's name or in a parent's: WinGet puts the
		// program inside "<Package>_Source" folders and installers use "<Name>\bin".
		if matches(rel, spec.Hints) {
			for _, exe := range spec.Exes {
				c := filepath.Join(p, exe)
				if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() {
					found = c
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	return found
}

func matches(rel string, hints []string) bool {
	l := strings.ToLower(rel)
	for _, h := range hints {
		if strings.Contains(l, strings.ToLower(h)) {
			return true
		}
	}
	return false
}

// Args fills "{file}" and "{dir}" in the catalog's argument template.
func Args(tpl []string, file, dir string) []string {
	out := make([]string, len(tpl))
	for i, a := range tpl {
		a = strings.ReplaceAll(a, "{file}", file)
		out[i] = strings.ReplaceAll(a, "{dir}", dir)
	}
	return out
}

// Start launches the emulator on its own, without waiting for it: it is a
// program the person will use, not a step of an install.
func Start(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe) // portable emulators keep their settings beside themselves
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// Register runs the one-time command that tells an emulator about a game (for
// ScummVM, "--add"), and waits for it.
func Register(exe string, args []string) error {
	if len(args) == 0 {
		return errors.New("nothing to run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = filepath.Dir(exe)
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

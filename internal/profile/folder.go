package profile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// storedName is what a profile file this app wrote is called: its id and .json. Any other
// .json in the folder (a sync client's "conflicted copy", someone's notes) is read but never
// copied somewhere else as if it were ours.
var storedName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}\.json$`)

// CheckDir reports whether a folder can hold profiles: it exists, is a folder, and a file can be
// created in it. A person can point the profiles at OneDrive or Dropbox, which may be offline,
// read-only or not mounted yet, so this is asked before every switch and at every start.
func CheckDir(dir string) error {
	if dir == "" {
		return errors.New("no folder given")
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%q is not a full path", dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%s is not available: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is a file, not a folder", dir)
	}
	probe, err := os.CreateTemp(dir, ".winforge-write-test-*")
	if err != nil {
		return fmt.Errorf("WinForge cannot write in %s: %w", dir, err)
	}
	name := probe.Name()
	_ = probe.Close()
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("WinForge cannot write in %s: %w", dir, err)
	}
	return nil
}

// SameDir reports whether two paths name the same folder. Windows paths ignore case.
func SameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if filepath.Separator == '\\' {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// CopyTo copies the stored profiles into another folder. A file that is already there is left
// as it is, never overwritten: the other folder may hold newer work from another PC.
func (s *Store) CopyTo(dst string) (copied, skipped int, err error) {
	if err := CheckDir(dst); err != nil {
		return 0, 0, err
	}
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		if e.IsDir() || !storedName.MatchString(e.Name()) {
			continue
		}
		target := filepath.Join(dst, e.Name())
		if _, err := os.Lstat(target); err == nil {
			skipped++
			continue
		}
		if err := copyFile(filepath.Join(s.Dir, e.Name()), target); err != nil {
			return copied, skipped, err
		}
		copied++
	}
	return copied, skipped, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

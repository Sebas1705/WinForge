package offlinekit

import (
	"io"
	"os"
	"path/filepath"
)

// Info describes one thing in a folder.
type Info struct {
	Name string
	Dir  bool
	// Link is true for anything that is not a plain file or folder: a symbolic link, a junction or other
	// reparse point, a device. Links are never followed.
	Link bool
	Size int64
}

// FS is the disk, as far as a kit needs it. The real one is OSFS; tests use MemFS.
type FS interface {
	// Lstat describes a path without following a link.
	Lstat(path string) (Info, error)
	// ReadDir lists a folder, each entry as Lstat would describe it.
	ReadDir(dir string) ([]Info, error)
	Open(path string) (io.ReadCloser, error)
	Create(path string) (io.WriteCloser, error)
	MkdirAll(path string) error
	RemoveAll(path string) error
	Rename(oldPath, newPath string) error
	// MkdirTemp makes a new private folder for the install copies.
	MkdirTemp(pattern string) (string, error)
	// FreeBytes is the free space on the drive that holds path.
	FreeBytes(path string) (int64, error)
}

// OSFS is the real disk.
type OSFS struct{}

func (OSFS) Lstat(path string) (Info, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return Info{}, err
	}
	return infoOf(fi), nil
}

func infoOf(fi os.FileInfo) Info {
	link := fi.Mode()&(os.ModeSymlink|os.ModeIrregular|os.ModeDevice|os.ModeNamedPipe|os.ModeSocket|os.ModeCharDevice) != 0 || isReparse(fi)
	return Info{Name: fi.Name(), Dir: fi.IsDir() && !link, Link: link, Size: fi.Size()}
}

func (OSFS) ReadDir(dir string) ([]Info, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(ents))
	for _, e := range ents {
		fi, err := e.Info() // lstat: a link is described, not followed
		if err != nil {
			return nil, err
		}
		out = append(out, infoOf(fi))
	}
	return out, nil
}

func (OSFS) Open(path string) (io.ReadCloser, error) { return os.Open(path) }

func (OSFS) Create(path string) (io.WriteCloser, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
}

func (OSFS) MkdirAll(path string) error               { return os.MkdirAll(path, 0o755) }
func (OSFS) RemoveAll(path string) error              { return os.RemoveAll(path) }
func (OSFS) Rename(oldPath, newPath string) error     { return replaceFile(oldPath, newPath) }
func (OSFS) MkdirTemp(pattern string) (string, error) { return os.MkdirTemp("", pattern) }

func (OSFS) FreeBytes(path string) (int64, error) { return freeBytes(path) }

// replaceFile renames oldPath over newPath. os.Rename replaces an existing file on Windows and elsewhere.
func replaceFile(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

// join is filepath.Join: every name that reaches it was checked with SafeName.
func join(elem ...string) string { return filepath.Join(elem...) }

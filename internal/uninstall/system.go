package uninstall

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/storage"
)

// Entry is one thing inside a folder, as System reports it.
type Entry struct {
	Name string
	Dir  bool
	// Link is true for anything that is not a plain file or folder: a symbolic link, a junction, a device.
	// Links are never entered, listed or moved.
	Link    bool
	Size    int64
	ModTime time.Time
}

// RegApp is an entry of Windows' list of installed programs.
type RegApp struct {
	Name string
	// SizeKB is the size the installer recorded (EstimatedSize); 0 when it did not.
	SizeKB int64
}

// System is everything the package needs from the machine. The real one reads the disk and the registry and
// moves to the Recycle Bin through storage's recycler; tests use an in-memory fake.
type System interface {
	// ReadDir lists the direct children of a folder.
	ReadDir(path string) ([]Entry, error)
	// Lstat describes one path without following a link. The error is os.ErrNotExist-like when it is gone.
	Lstat(path string) (Entry, error)
	// Measure adds up the files under a folder without following links, and finds the newest change.
	Measure(ctx context.Context, dir string) (bytes int64, files int, newest time.Time)
	// RecycleFolders and RecycleFiles move to the Recycle Bin. A path counts as moved only if it is gone afterwards.
	RecycleFolders(ctx context.Context, paths []string) (moved int, freed int64, failed []string)
	RecycleFiles(ctx context.Context, paths []string) (moved int, freed int64, failed []string)
	// Installed lists the programs Windows says are installed.
	Installed() []RegApp
}

// Env is where this PC keeps what the search looks at. Tests lay it out in a temporary folder.
type Env struct {
	Home            string
	AppData         string // %APPDATA%
	LocalAppData    string // %LOCALAPPDATA%
	ProgramData     string // %ProgramData%
	ProgramFiles    string // %ProgramFiles%
	ProgramFilesX86 string // %ProgramFiles(x86)%
	WinDir          string
	SystemDrive     string // "C:"
	Public          string // %PUBLIC%
	OneDrive        string // %OneDrive%, when there is one
}

// PlaceID names one of the fixed places.
type PlaceID string

const (
	PlaceAppData         PlaceID = "appdata"
	PlaceLocalAppData    PlaceID = "localappdata"
	PlaceLocalPrograms   PlaceID = "localprograms"
	PlaceProgramData     PlaceID = "programdata"
	PlaceProgramFiles    PlaceID = "programfiles"
	PlaceProgramFilesX86 PlaceID = "programfilesx86"
	PlaceStartMenu       PlaceID = "startmenu"
	PlaceStartMenuAll    PlaceID = "startmenuall"
	PlaceDesktop         PlaceID = "desktop"
	PlaceDesktopAll      PlaceID = "desktopall"
	PlaceDesktopOneDrive PlaceID = "desktoponedrive"
)

// Place is one folder the search reads.
type Place struct {
	ID  PlaceID `json:"place"`
	Dir string  `json:"path"`
	// Folders: folders directly inside it can be listed. Nested: so can "Publisher\Name" one level down.
	Folders, Nested bool
	// Shortcuts: .lnk files directly inside it can be listed.
	Shortcuts bool
}

func join(base string, parts ...string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, parts...)...)
}

// Places is the whole list of folders the search reads, in the order results are shown. Adding a place is
// adding a line here; nothing is discovered by scanning the disk. A place whose folder is not known on this
// PC is left out.
func (e Env) Places() []Place {
	var out []Place
	add := func(id PlaceID, dir string, folders, nested, shortcuts bool) {
		if dir != "" && filepath.IsAbs(dir) && filepath.Clean(dir) == dir {
			out = append(out, Place{ID: id, Dir: dir, Folders: folders, Nested: nested, Shortcuts: shortcuts})
		}
	}
	add(PlaceAppData, e.AppData, true, true, false)
	add(PlaceLocalAppData, e.LocalAppData, true, true, false)
	// Per-user installers put the program itself in Programs; it is the one folder below LocalAppData read.
	add(PlaceLocalPrograms, join(e.LocalAppData, "Programs"), true, false, false)
	add(PlaceProgramData, e.ProgramData, true, true, false)
	add(PlaceProgramFiles, e.ProgramFiles, true, true, false)
	add(PlaceProgramFilesX86, e.ProgramFilesX86, true, true, false)
	add(PlaceStartMenu, join(e.AppData, "Microsoft", "Windows", "Start Menu", "Programs"), true, false, true)
	add(PlaceStartMenuAll, join(e.ProgramData, "Microsoft", "Windows", "Start Menu", "Programs"), true, false, true)
	add(PlaceDesktop, join(e.Home, "Desktop"), false, false, true)
	add(PlaceDesktopOneDrive, join(e.OneDrive, "Desktop"), false, false, true)
	add(PlaceDesktopAll, join(e.Public, "Desktop"), false, false, true)
	return out
}

func (e Env) storageEnv() storage.Env {
	return storage.Env{Home: e.Home, LocalAppData: e.LocalAppData, AppData: e.AppData, ProgramData: e.ProgramData, WinDir: e.WinDir, SystemDrive: e.SystemDrive}
}

// protected are the folders that may never be listed, moved, or contain what is moved: storage's list
// (profile, data folders, Windows, drive root, Documents, Downloads, Desktop...) plus the program folders,
// the public profile and every place itself.
func (e Env) protected() []string {
	p := storage.ProtectedDirs(e.storageEnv())
	for _, d := range []string{e.ProgramFiles, e.ProgramFilesX86, e.Public, e.OneDrive} {
		if d != "" {
			p = append(p, filepath.Clean(d))
		}
	}
	for _, pl := range e.Places() {
		p = append(p, pl.Dir)
	}
	// The Programs folder of the Start menu is a place, and so is everything above it.
	p = append(p, join(e.AppData, "Microsoft"), join(e.AppData, "Microsoft", "Windows"), join(e.AppData, "Microsoft", "Windows", "Start Menu"),
		join(e.ProgramData, "Microsoft"), join(e.ProgramData, "Microsoft", "Windows"), join(e.ProgramData, "Microsoft", "Windows", "Start Menu"))
	out := p[:0]
	for _, d := range p {
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// untouchable are folders whose contents are never offered, though the folder may be searched: what is
// inside the person's own document folders and the Windows folder.
func (e Env) untouchable() []string {
	var out []string
	for _, d := range []string{e.WinDir} {
		if d != "" {
			out = append(out, filepath.Clean(d))
		}
	}
	for _, n := range []string{"Documents", "Downloads", "Pictures", "Videos", "Music"} {
		if d := join(e.Home, n); d != "" {
			out = append(out, d)
		}
		if d := join(e.OneDrive, n); d != "" {
			out = append(out, d)
		}
	}
	return out
}

func sameDir(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }

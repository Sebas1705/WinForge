package main

import (
	"errors"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/profile"
	"github.com/Sebas1705/WinForge/internal/settings"
)

// BackupSets lists the app settings that exist on this PC and can be saved.
func (a *App) BackupSets() []settings.Found {
	found := settings.Detect(settings.DefaultRoots(), settings.OSRunner{})
	if found == nil {
		return []settings.Found{}
	}
	return found
}

// PickFolder asks for a folder, to add to a backup or to restore into. It
// returns "" when the dialog is cancelled.
func (a *App) PickFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{})
}

// BackupRequest says what goes into a backup.
type BackupRequest struct {
	Sets    []string `json:"sets"`
	Apps    bool     `json:"apps"`    // the list of installed catalog apps, as a profile
	Folders []string `json:"folders"` // folders the person added
}

// BackupResult is what was written and whether it reads back intact.
type BackupResult struct {
	Path     string `json:"path"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
	Apps     int    `json:"apps"`
	Folders  int    `json:"folders"`
	Verified bool   `json:"verified"`
	Problems int    `json:"problems"`
}

// CreateBackup asks where to save, writes the backup there and reads it back to
// check it. It returns an empty result when the dialog is cancelled.
func (a *App) CreateBackup(req BackupRequest) (*BackupResult, error) {
	host, _ := os.Hostname()
	o := settings.Options{IDs: req.Sets, Roots: settings.DefaultRoots(), Run: settings.OSRunner{}, Folders: req.Folders, Host: host, App: version}
	if req.Apps {
		a.mu.Lock()
		versions := make(map[string]string, len(a.installed))
		for id, i := range a.installed {
			versions[id] = i.Version
		}
		a.mu.Unlock()
		if len(versions) > 0 {
			raw, err := profile.Export(profile.FromInstalled("backup", "Backup "+host, versions, false))
			if err != nil {
				return nil, err
			}
			o.Profile, o.Apps = raw, len(versions)
		}
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "winforge-backup.zip",
		Filters:         []runtime.FileFilter{{DisplayName: "WinForge backup (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || path == "" {
		return &BackupResult{}, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	m, err := settings.Create(f, o)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path) // never leave a half-written backup that looks real
		return nil, err
	}
	res := &BackupResult{Path: path, Apps: m.Apps, Folders: len(m.Folders)}
	for _, s := range m.Sets {
		res.Files += s.Files
		res.Bytes += s.Bytes
	}
	for _, fo := range m.Folders {
		res.Files += fo.Files
		res.Bytes += fo.Bytes
	}
	// Read it back: a backup that cannot be read is worse than none.
	if r, err := os.Open(path); err == nil {
		defer r.Close()
		if st, err := r.Stat(); err == nil {
			if in, err := settings.Verify(r, st.Size()); err == nil {
				res.Verified = in.OK()
				res.Problems = len(in.Damaged) + len(in.Missing) + len(in.Unlisted)
			}
		}
	}
	return res, nil
}

// SetPreview is what restoring one settings set would do.
type SetPreview struct {
	ID      string `json:"id"`
	Files   int    `json:"files"`
	New     int    `json:"new"`
	Changed int    `json:"changed"`
	Same    int    `json:"same"`
}

// AppPreview is an app a backup lists, and whether this PC already has it.
type AppPreview struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
}

// BackupPreview is everything a backup holds, shown before anything is written.
type BackupPreview struct {
	Host      string `json:"host"`
	App       string `json:"app"`
	CreatedAt string `json:"createdAt"`
	// Intact is false when an entry is damaged, missing or was added after the
	// backup was made; Checked is false for old backups without checksums.
	Intact   bool                  `json:"intact"`
	Checked  bool                  `json:"checked"`
	Problems []string              `json:"problems"`
	Sets     []SetPreview          `json:"sets"`
	Apps     []AppPreview          `json:"apps"`
	Missing  int                   `json:"missing"`
	Unknown  []string              `json:"unknown"`
	Profile  *catalog.Profile      `json:"profile"`
	Folders  []settings.FolderInfo `json:"folders"`
}

// PickBackup asks for a backup, checks it and reports what is in it. Nothing is
// written; the Restore methods do that for what the person keeps ticked.
func (a *App) PickBackup() (*BackupPreview, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "WinForge backup (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || path == "" {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	m, err := settings.ReadManifest(f, st.Size())
	if err != nil {
		return nil, err
	}
	in, err := settings.Verify(f, st.Size())
	if err != nil {
		return nil, err
	}
	p := &BackupPreview{
		Host: m.Host, App: m.App, CreatedAt: m.CreatedAt.Format("2006-01-02 15:04"),
		Intact: in.OK(), Checked: in.Checked, Problems: []string{}, Sets: []SetPreview{}, Apps: []AppPreview{}, Unknown: []string{}, Folders: m.Folders,
	}
	if p.Folders == nil {
		p.Folders = []settings.FolderInfo{}
	}
	p.Problems = append(p.Problems, in.Damaged...)
	p.Problems = append(p.Problems, in.Missing...)
	p.Problems = append(p.Problems, in.Unlisted...)

	ids := make([]string, 0, len(m.Sets))
	for _, s := range m.Sets {
		ids = append(ids, s.ID)
	}
	changes, _ := settings.Diff(f, st.Size(), ids, settings.DefaultRoots())
	for _, s := range m.Sets {
		sp := SetPreview{ID: s.ID, Files: s.Files}
		for _, c := range changes {
			if c.Set != s.ID {
				continue
			}
			switch c.State {
			case "new":
				sp.New++
			case "changed":
				sp.Changed++
			default:
				sp.Same++
			}
		}
		p.Sets = append(p.Sets, sp)
	}

	if m.HasProfile {
		if raw, err := settings.Profile(f, st.Size()); err == nil && raw != nil {
			if imp, err := profile.Import(raw, a.cat); err == nil {
				p.Profile = &imp.Profile
				p.Unknown = append(p.Unknown, imp.UnknownApps...)
				a.mu.Lock()
				installed := a.installed
				a.mu.Unlock()
				for _, pa := range imp.Profile.Apps {
					app := a.cat.Apps[pa.ID]
					if app == nil {
						continue
					}
					_, have := installed[pa.ID]
					p.Apps = append(p.Apps, AppPreview{ID: pa.ID, Name: app.Name, Installed: have})
					if !have {
						p.Missing++
					}
				}
			}
		}
	}
	a.mu.Lock()
	a.restorePath = path
	a.mu.Unlock()
	return p, nil
}

func (a *App) openRestore() (*os.File, int64, error) {
	a.mu.Lock()
	path := a.restorePath
	a.mu.Unlock()
	if path == "" {
		return nil, 0, errors.New("choose a backup first")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

// RestoreSettings writes the chosen sets of the backup picked by PickBackup.
// Files it replaces are kept next to the new ones with a .winforge-bak suffix.
func (a *App) RestoreSettings(ids []string) (*settings.Result, error) {
	f, size, err := a.openRestore()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return settings.Restore(f, size, ids, settings.DefaultRoots(), settings.OSRunner{})
}

// RestoreFolders writes the chosen folders of the picked backup into dest, each
// under its own name. Nothing that already exists is overwritten.
func (a *App) RestoreFolders(names []string, dest string) (*settings.FolderResult, error) {
	f, size, err := a.openRestore()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if strings.TrimSpace(dest) == "" {
		return nil, errors.New("choose a folder to restore into")
	}
	return settings.RestoreFolders(f, size, dest, names)
}

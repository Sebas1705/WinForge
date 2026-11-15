package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/Sebas1705/WinForge/internal/appconfig"
	"github.com/Sebas1705/WinForge/internal/profile"
)

// FolderInfo says where the profiles are kept.
type FolderInfo struct {
	Path    string `json:"path"`
	Default string `json:"default"` // where they are kept unless the person chose a folder
	Custom  bool   `json:"custom"`
	Count   int    `json:"count"` // profiles found there
	Problem string `json:"problem,omitempty"`
}

// FolderChange is what switching folders did.
type FolderChange struct {
	Copied  int        `json:"copied"`
	Skipped int        `json:"skipped"` // already in the new folder, left as they were
	Info    FolderInfo `json:"info"`
}

var errNoConfig = errors.New("WinForge could not open its settings folder, so the profiles folder cannot be changed")

func (a *App) profileStore() *profile.Store {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.store
}

func (a *App) defaultProfileStore() (*profile.Store, error) {
	if a.defaultStore != nil {
		return a.defaultStore()
	}
	return profile.DefaultStore()
}

// useConfiguredProfiles applies the folder the person chose, if any. A folder that is not
// available now (OneDrive not signed in, a drive that is unplugged) must not stop the app
// from starting or send profiles to a place they cannot be read back from: the default folder
// is used instead and the person is told.
func (a *App) useConfiguredProfiles() {
	if a.config == nil {
		return
	}
	cfg, err := a.config.Load()
	if err != nil {
		a.profilesNote = "WinForge could not read its settings (" + err.Error() + "), so the default profiles folder is used."
		log.Printf("winforge: config: %v", err)
		return
	}
	if cfg.ProfilesDir == "" {
		return
	}
	if err := profile.CheckDir(cfg.ProfilesDir); err != nil {
		a.profilesNote = "The profiles folder is not available (" + err.Error() + "). WinForge is using the default one until it is."
		log.Printf("winforge: profiles folder: %v", err)
		return
	}
	a.mu.Lock()
	a.store = &profile.Store{Dir: filepath.Clean(cfg.ProfilesDir)}
	a.mu.Unlock()
}

// ProfilesFolder says where profiles are kept right now.
func (a *App) ProfilesFolder() FolderInfo {
	a.mu.Lock()
	note := a.profilesNote
	a.mu.Unlock()
	info := FolderInfo{Problem: note}
	if def, err := a.defaultProfileStore(); err == nil {
		info.Default = def.Dir
	}
	s := a.profileStore()
	if s == nil {
		info.Problem = errStorageUnavailable.Error()
		return info
	}
	info.Path = s.Dir
	info.Custom = info.Default != "" && !profile.SameDir(s.Dir, info.Default)
	if list, err := s.List(a.cat); err == nil {
		info.Count = len(list)
	}
	return info
}

// SetProfilesFolder keeps the profiles in another folder, for example one that OneDrive or Dropbox
// keeps in sync between PCs. With copyExisting the profiles already stored are copied there first;
// a file already in the new folder is never overwritten. The old folder is left as it is.
func (a *App) SetProfilesFolder(dir string, copyExisting bool) (FolderChange, error) {
	if a.config == nil {
		return FolderChange{}, errNoConfig
	}
	dir = filepath.Clean(dir)
	if err := profile.CheckDir(dir); err != nil {
		return FolderChange{}, err
	}
	cur := a.profileStore()
	if cur != nil && profile.SameDir(cur.Dir, dir) {
		return FolderChange{}, errors.New("WinForge already keeps the profiles in that folder")
	}
	var change FolderChange
	if copyExisting && cur != nil {
		copied, skipped, err := cur.CopyTo(dir)
		if err != nil {
			return FolderChange{}, fmt.Errorf("could not copy the profiles: %w", err)
		}
		change.Copied, change.Skipped = copied, skipped
	}
	// Choosing the default folder is going back to the default, not a custom folder that happens to match.
	cfg := appconfig.Config{ProfilesDir: dir}
	if def, err := a.defaultProfileStore(); err == nil && profile.SameDir(def.Dir, dir) {
		cfg = appconfig.Config{}
	}
	if err := a.config.Save(cfg); err != nil {
		return FolderChange{}, err
	}
	a.mu.Lock()
	a.store = &profile.Store{Dir: dir}
	a.profilesNote = ""
	a.mu.Unlock()
	change.Info = a.ProfilesFolder()
	return change, nil
}

// ResetProfilesFolder goes back to the default folder. Nothing is copied or removed.
func (a *App) ResetProfilesFolder() (FolderInfo, error) {
	if a.config == nil {
		return FolderInfo{}, errNoConfig
	}
	def, err := a.defaultProfileStore()
	if err != nil {
		return FolderInfo{}, err
	}
	if err := a.config.Save(appconfig.Config{}); err != nil {
		return FolderInfo{}, err
	}
	a.mu.Lock()
	a.store = def
	a.profilesNote = ""
	a.mu.Unlock()
	return a.ProfilesFolder(), nil
}

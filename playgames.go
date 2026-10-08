package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/emu"
	"github.com/Sebas1705/WinForge/internal/games"
	"github.com/Sebas1705/WinForge/internal/settings"
)

// GamesView is what the games page needs to offer installs: the games folder,
// the installable games, which are installed, and which emulators WinForge can
// start (found on disk).
type GamesView struct {
	Root      string                   `json:"root"`
	Games     []catalog.Game           `json:"games"`
	Installed map[string]games.Receipt `json:"installed"`
	// Launchers maps an installed emulator's catalog id to the program found on
	// disk, for the emulators whose command line is known.
	Launchers map[string]string `json:"launchers"`
}

func (a *App) gamesRoot() string {
	def := games.DefaultRoot(settings.DefaultRoots()[settings.RootDocuments])
	if p, err := games.ConfigPath(); err == nil {
		return games.LoadRoot(p, def)
	}
	return def
}

func (a *App) game(id string) (catalog.Game, bool) {
	for _, g := range a.cat.Games {
		if g.ID == id {
			return g, true
		}
	}
	return catalog.Game{}, false
}

// GamesState lists the installable games and what is installed. It looks for
// the programs of emulators that are installed on this PC, so it can be slow
// the first time.
func (a *App) GamesState() GamesView {
	root := a.gamesRoot()
	v := GamesView{Root: root, Games: a.cat.Games, Installed: games.State(root, a.cat.Games), Launchers: map[string]string{}}
	if v.Games == nil {
		v.Games = []catalog.Game{}
	}
	if a.cat.Emulation == nil {
		return v
	}
	a.mu.Lock()
	installed := a.installed
	a.mu.Unlock()
	roots := emu.DefaultRoots()
	for id, spec := range a.cat.Emulation.Run {
		if _, ok := installed[id]; !ok {
			continue
		}
		if exe := emu.Locate(spec, roots); exe != "" {
			v.Launchers[id] = exe
		}
	}
	return v
}

// SetGamesRoot chooses the folder games are installed into, asking for it.
// It returns the new folder, or the current one when the dialog is cancelled.
func (a *App) SetGamesRoot() (string, error) {
	cur := a.gamesRoot()
	_ = os.MkdirAll(cur, 0o755) // so the dialog opens there
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Where should games be installed?", DefaultDirectory: cur})
	if err != nil || dir == "" {
		return cur, err
	}
	p, err := games.ConfigPath()
	if err != nil {
		return cur, err
	}
	if err := games.SaveRoot(p, dir); err != nil {
		return cur, err
	}
	return dir, nil
}

// GameInstalled is what an install did.
type GameInstalled struct {
	Receipt games.Receipt `json:"receipt"`
	// Registered lists the emulators that were told about the game (ScummVM).
	Registered []string `json:"registered"`
	// RegisterError is why telling an emulator failed; the game is installed anyway.
	RegisterError string `json:"registerError,omitempty"`
}

// InstallGame downloads a catalog game into the games folder, verifies it
// against the pinned checksum and, where an emulator can be told about it,
// registers it there. Progress is sent as "game:progress" events.
func (a *App) InstallGame(id string) (*GameInstalled, error) {
	g, ok := a.game(id)
	if !ok {
		return nil, errors.New("unknown game")
	}
	a.mu.Lock()
	if a.installing[id] {
		a.mu.Unlock()
		return nil, errors.New("this game is already being installed")
	}
	if a.installing == nil {
		a.installing = map[string]bool{}
	}
	a.installing[id] = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); delete(a.installing, id); a.mu.Unlock() }()

	root := a.gamesRoot()
	var lastSent time.Time
	progress := func(done, total int64) {
		if done == total || time.Since(lastSent) > 150*time.Millisecond {
			lastSent = time.Now()
			runtime.EventsEmit(a.ctx, "game:progress", map[string]any{"id": id, "done": done, "total": total})
		}
	}
	r, err := games.Install(a.ctx, &http.Client{Timeout: 0}, root, g, progress)
	if err != nil {
		return nil, err
	}
	res := &GameInstalled{Receipt: *r, Registered: []string{}}
	a.registerWithEmulators(g, root, res)
	return res, nil
}

// registerWithEmulators tells every installed emulator that plays the game's
// system, and knows how to be told, about the game.
func (a *App) registerWithEmulators(g catalog.Game, root string, res *GameInstalled) {
	if a.cat.Emulation == nil {
		return
	}
	a.mu.Lock()
	installed := a.installed
	a.mu.Unlock()
	roots := emu.DefaultRoots()
	var ids []string
	for id := range a.cat.Emulation.Run {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		spec := a.cat.Emulation.Run[id]
		if len(spec.Register) == 0 || !plays(a.cat.Emulation, id, g.System) {
			continue
		}
		if _, ok := installed[id]; !ok {
			continue
		}
		exe := emu.Locate(spec, roots)
		if exe == "" {
			continue
		}
		if err := emu.Register(exe, emu.Args(spec.Register, "", games.Dir(root, g))); err != nil {
			res.RegisterError = fmt.Sprintf("%s: %v", id, err)
			continue
		}
		res.Registered = append(res.Registered, id)
	}
}

func plays(e *catalog.Emulation, emulator, system string) bool {
	for _, s := range e.Emulators[emulator] {
		if s == system {
			return true
		}
	}
	return false
}

// RemoveGame deletes an installed game's folder. Only folders WinForge made are removed.
func (a *App) RemoveGame(id string) error {
	g, ok := a.game(id)
	if !ok {
		return errors.New("unknown game")
	}
	return games.Remove(a.gamesRoot(), g)
}

// PlayGame starts an installed game with one of the emulators that plays it.
func (a *App) PlayGame(id, emulator string) error {
	g, ok := a.game(id)
	if !ok {
		return errors.New("unknown game")
	}
	if a.cat.Emulation == nil || !plays(a.cat.Emulation, emulator, g.System) {
		return errors.New("that emulator does not play this system")
	}
	spec, ok := a.cat.Emulation.Run[emulator]
	if !ok || len(spec.Args) == 0 {
		return errors.New("WinForge does not know how to start that emulator on a game: open the game from the emulator")
	}
	root := a.gamesRoot()
	file := games.EntryPath(root, g)
	if file == "" {
		return errors.New("this game is opened from its front end, not started by file")
	}
	if st, err := os.Stat(file); err != nil || !st.Mode().IsRegular() {
		return errors.New("the game is not installed")
	}
	exe := emu.Locate(spec, emu.DefaultRoots())
	if exe == "" {
		return errors.New("the emulator's program could not be found on this PC")
	}
	return emu.Start(exe, emu.Args(spec.Args, file, filepath.Dir(file)))
}

// OpenGameFolder shows the folder of an installed game in Explorer.
func (a *App) OpenGameFolder(id string) error {
	g, ok := a.game(id)
	if !ok {
		return errors.New("unknown game")
	}
	dir := games.Dir(a.gamesRoot(), g)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return errors.New("the game is not installed")
	}
	// explorer.exe reports failure even when it opens the folder, so the exit code is ignored.
	return exec.Command("explorer.exe", dir).Start()
}

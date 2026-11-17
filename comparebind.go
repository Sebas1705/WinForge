package main

import (
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/compare"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// CompareApp is an app named in a comparison.
type CompareApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CompareVersion is an app both sides have at different versions: A is this PC / file A, B the other.
type CompareVersion struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	A    string `json:"a"`
	B    string `json:"b"`
}

// CompareUnknown is an id this catalog does not have. In is "a", "b" or "both".
type CompareUnknown struct {
	ID string `json:"id"`
	In string `json:"in"`
}

// CompareSide describes one side of a comparison. No sentence is ever sent: the page words everything.
type CompareSide struct {
	Kind            string           `json:"kind"` // "pc", "file" or "code"
	Name            string           `json:"name"` // the profile's own name; "" for this PC
	File            string           `json:"file"` // the file's name, for a file
	Apps            int              `json:"apps"`
	Versioned       bool             `json:"versioned"`
	UnknownRecipes  []string         `json:"unknownRecipes"`
	UnknownProfiles []string         `json:"unknownProfiles"`
	Profile         *catalog.Profile `json:"profile"` // the file cleaned to what this catalog knows, to save it; null for this PC
}

// CompareResult is what two setups have and lack, read-only.
type CompareResult struct {
	A       CompareSide      `json:"a"`
	B       CompareSide      `json:"b"`
	OnlyA   []CompareApp     `json:"onlyA"`
	OnlyB   []CompareApp     `json:"onlyB"`
	Both    []CompareApp     `json:"both"`
	Differ  []CompareVersion `json:"differ"`
	Unknown []CompareUnknown `json:"unknown"`
}

// pickedProfiles are the files the person chose in the dialog of PickProfileFile. CompareFile and
// CompareFiles read only those, so the page cannot ask Go to read any other file on the PC.
var pickedProfiles = struct {
	sync.Mutex
	paths map[string]bool
}{paths: map[string]bool{}}

func rememberPicked(path string) {
	pickedProfiles.Lock()
	defer pickedProfiles.Unlock()
	if len(pickedProfiles.paths) >= 64 {
		pickedProfiles.paths = map[string]bool{}
	}
	pickedProfiles.paths[filepath.Clean(path)] = true
}

func wasPicked(path string) bool {
	pickedProfiles.Lock()
	defer pickedProfiles.Unlock()
	return pickedProfiles.paths[filepath.Clean(path)]
}

// PickProfileFile asks for a profile file (*.json) and returns its path, or "" if cancelled. It reads
// nothing: CompareFile does, and only for a path chosen here.
func (a *App) PickProfileFile() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "WinForge profile (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return "", err
	}
	rememberPicked(path)
	return path, nil
}

// readPicked reads a chosen profile file, capped in size, and never anything that is not a regular file.
func readPicked(path string) ([]byte, error) {
	if path == "" || !wasPicked(path) {
		return nil, &compare.Error{Code: "not-picked"}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, &compare.Error{Code: "unreadable"}
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, &compare.Error{Code: "unreadable"}
	}
	if st.Size() > compare.MaxFile {
		return nil, &compare.Error{Code: "too-big"}
	}
	b, err := io.ReadAll(io.LimitReader(f, compare.MaxFile+1))
	if err != nil {
		return nil, &compare.Error{Code: "unreadable"}
	}
	return b, nil
}

func (a *App) fileSide(path string) (compare.Side, error) {
	b, err := readPicked(path)
	if err != nil {
		return compare.Side{}, err
	}
	return compare.ParseFile(b, a.cat)
}

func (a *App) pcSide() (compare.Side, error) {
	a.mu.Lock()
	installed := a.installed
	a.mu.Unlock()
	if installed == nil {
		return compare.Side{}, &compare.Error{Code: "no-scan"}
	}
	return compare.PC(versionsOf(installed)), nil
}

// CompareFile sets this PC (catalog apps only, as of the last scan) against a profile file chosen with
// PickProfileFile. It changes nothing.
func (a *App) CompareFile(path string) (CompareResult, error) {
	pc, err := a.pcSide()
	if err != nil {
		return CompareResult{}, err
	}
	other, err := a.fileSide(path)
	if err != nil {
		return CompareResult{}, err
	}
	return a.compareResult(pc, other, "pc", "file", "", filepath.Base(path)), nil
}

// CompareCode sets this PC against a pasted share code. It changes nothing.
func (a *App) CompareCode(code string) (CompareResult, error) {
	pc, err := a.pcSide()
	if err != nil {
		return CompareResult{}, err
	}
	other, err := compare.ParseCode(code, a.cat)
	if err != nil {
		return CompareResult{}, err
	}
	return a.compareResult(pc, other, "pc", "code", "", ""), nil
}

// CompareFiles sets one profile file (A) against another (B), both chosen with PickProfileFile.
func (a *App) CompareFiles(pathA, pathB string) (CompareResult, error) {
	sa, err := a.fileSide(pathA)
	if err != nil {
		return CompareResult{}, err
	}
	sb, err := a.fileSide(pathB)
	if err != nil {
		return CompareResult{}, err
	}
	return a.compareResult(sa, sb, "file", "file", filepath.Base(pathA), filepath.Base(pathB)), nil
}

func (a *App) compareResult(sa, sb compare.Side, kindA, kindB, fileA, fileB string) CompareResult {
	r := compare.Compare(sa, sb)
	out := CompareResult{
		A: describeSide(sa, kindA, fileA), B: describeSide(sb, kindB, fileB),
		OnlyA: a.compareApps(r.OnlyA), OnlyB: a.compareApps(r.OnlyB), Both: a.compareApps(r.Both),
		Differ: []CompareVersion{}, Unknown: []CompareUnknown{},
	}
	for _, d := range r.Differ {
		out.Differ = append(out.Differ, CompareVersion{ID: d.ID, Name: a.nameOf(d.ID), A: d.A, B: d.B})
	}
	for _, u := range r.Unknown {
		out.Unknown = append(out.Unknown, CompareUnknown{ID: u.ID, In: u.In})
	}
	return out
}

func describeSide(s compare.Side, kind, file string) CompareSide {
	return CompareSide{
		Kind: kind, Name: s.Name, File: file, Apps: len(s.Versions), Versioned: s.Versioned,
		UnknownRecipes: nonNil(s.UnknownRecipes), UnknownProfiles: nonNil(s.UnknownProfiles), Profile: s.Profile,
	}
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func (a *App) compareApps(ids []string) []CompareApp {
	out := make([]CompareApp, 0, len(ids))
	for _, id := range ids {
		out = append(out, CompareApp{ID: id, Name: a.nameOf(id)})
	}
	return out
}

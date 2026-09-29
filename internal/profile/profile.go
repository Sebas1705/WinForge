// Package profile stores, exports and imports user profiles. A profile file
// is data only - catalog ids and versions - so importing one from another
// person cannot introduce commands the catalog has not vetted.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

// FormatVersion is the file format version written by Export.
const FormatVersion = 1

// File is the on-disk / shareable representation of a profile.
type File struct {
	WinForge int             `json:"winforge"`
	Profile  catalog.Profile `json:"profile"`
}

// Export serializes a profile for sharing.
func Export(p catalog.Profile) ([]byte, error) {
	sortApps(p.Apps)
	b, err := json.MarshalIndent(File{WinForge: FormatVersion, Profile: p}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Imported is the result of reading a profile file.
type Imported struct {
	Profile catalog.Profile
	// UnknownApps and UnknownRecipes were dropped because this catalog does
	// not contain them (a newer catalog, or a hand-edited file).
	UnknownApps    []string
	UnknownRecipes []string
}

var profileID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// Import parses a profile file and checks it against the catalog. Unknown
// apps and recipes are dropped and reported, never executed or installed.
// Anything that could smuggle behavior in (extends of unknown profiles, bad
// ids) is rejected.
func Import(b []byte, cat *catalog.Catalog) (*Imported, error) {
	var f File
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("not a WinForge profile: %w", err)
	}
	if f.WinForge < 1 {
		return nil, errors.New("not a WinForge profile: missing \"winforge\" version")
	}
	if f.WinForge > FormatVersion {
		return nil, fmt.Errorf("profile format %d is newer than this WinForge understands (%d)", f.WinForge, FormatVersion)
	}
	p := f.Profile
	if !profileID.MatchString(p.ID) {
		return nil, fmt.Errorf("invalid profile id %q", p.ID)
	}
	if strings.TrimSpace(p.Name) == "" {
		return nil, errors.New("profile has no name")
	}
	for _, e := range p.Extends {
		if cat.Profiles[e] == nil {
			return nil, fmt.Errorf("profile extends %q, which is not a catalog profile", e)
		}
	}
	out := &Imported{Profile: p}
	out.Profile.Apps = nil
	seen := map[string]bool{}
	for _, a := range p.Apps {
		switch {
		case cat.Apps[a.ID] == nil:
			out.UnknownApps = append(out.UnknownApps, a.ID)
		case !seen[a.ID]:
			seen[a.ID] = true
			out.Profile.Apps = append(out.Profile.Apps, a)
		}
	}
	out.Profile.Recipes = nil
	for _, r := range p.Recipes {
		if cat.Recipes[r] == nil {
			out.UnknownRecipes = append(out.UnknownRecipes, r)
		} else {
			out.Profile.Recipes = append(out.Profile.Recipes, r)
		}
	}
	return out, nil
}

// FromInstalled builds a profile from what is installed, pinning versions
// only when the caller asks for it.
func FromInstalled(id, name string, installed map[string]string, pinVersions bool) catalog.Profile {
	p := catalog.Profile{ID: id, Name: name, Kind: "custom"}
	for appID, v := range installed {
		pa := catalog.ProfileApp{ID: appID}
		if pinVersions {
			pa.Version = v
		}
		p.Apps = append(p.Apps, pa)
	}
	sortApps(p.Apps)
	return p
}

func sortApps(apps []catalog.ProfileApp) {
	sort.Slice(apps, func(i, j int) bool { return apps[i].ID < apps[j].ID })
}

// WingetImport renders the profile as a file `winget import` accepts, so the
// profile stays usable on a PC that does not have WinForge.
func WingetImport(apps []catalog.ProfileApp, cat *catalog.Catalog) ([]byte, error) {
	type pkg struct {
		PackageIdentifier string `json:"PackageIdentifier"`
		Version           string `json:"Version,omitempty"`
	}
	type source struct {
		Packages      []pkg `json:"Packages"`
		SourceDetails struct {
			Name       string `json:"Name"`
			Identifier string `json:"Identifier"`
			Argument   string `json:"Argument"`
			Type       string `json:"Type"`
		} `json:"SourceDetails"`
	}
	var s source
	s.SourceDetails.Name = "winget"
	s.SourceDetails.Identifier = "Microsoft.Winget.Source_8wekyb3d8bbwe"
	s.SourceDetails.Argument = "https://cdn.winget.microsoft.com/cache"
	s.SourceDetails.Type = "Microsoft.PreIndexed.Package"
	for _, a := range apps {
		app := cat.Apps[a.ID]
		if app == nil {
			return nil, fmt.Errorf("unknown app %q", a.ID)
		}
		s.Packages = append(s.Packages, pkg{PackageIdentifier: app.Winget, Version: a.Version})
	}
	doc := map[string]any{
		"$schema":       "https://aka.ms/winget-packages.schema.2.0.json",
		"CreationDate":  "",
		"Sources":       []source{s},
		"WinGetVersion": "1.9.0",
	}
	return json.MarshalIndent(doc, "", "  ")
}

// Store keeps user profiles as one JSON file each in a directory.
type Store struct{ Dir string }

// DefaultStore is %APPDATA%\WinForge\profiles.
func DefaultStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{Dir: filepath.Join(base, "WinForge", "profiles")}, nil
}

func (s *Store) path(id string) (string, error) {
	if !profileID.MatchString(id) {
		return "", fmt.Errorf("invalid profile id %q", id)
	}
	return filepath.Join(s.Dir, id+".json"), nil
}

// Save writes a profile atomically.
func (s *Store) Save(p catalog.Profile) error {
	path, err := s.path(p.ID)
	if err != nil {
		return err
	}
	b, err := Export(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Delete removes a profile; a missing profile is not an error.
func (s *Store) Delete(id string) error {
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// List returns every stored profile that still parses against the catalog.
func (s *Store) List(cat *catalog.Catalog) ([]catalog.Profile, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []catalog.Profile
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			continue
		}
		imp, err := Import(b, cat)
		if err != nil {
			continue
		}
		out = append(out, imp.Profile)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

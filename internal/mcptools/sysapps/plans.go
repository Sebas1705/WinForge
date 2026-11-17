package sysapps

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sebas1705/WinForge/internal/appconfig"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/profile"
)

func quote(s string) string { return fmt.Sprintf("%q", short(s)) }

// profileFolder is where the person keeps profiles: the folder chosen in Settings when it is a real
// folder, otherwise %AppData%\WinForge\profiles. It only reads; unlike the app it does not probe the
// folder by writing a file in it.
func profileFolder(e *mcpkit.Env) (string, error) {
	base, err := appDataOf(e)
	if err != nil {
		return "", err
	}
	cfg, err := (&appconfig.Store{Path: filepath.Join(base, "config.json")}).Load()
	if err == nil && cfg.ProfilesDir != "" && filepath.IsAbs(cfg.ProfilesDir) {
		if st, serr := os.Stat(cfg.ProfilesDir); serr == nil && st.IsDir() {
			return filepath.Clean(cfg.ProfilesDir), nil
		}
	}
	return filepath.Join(base, "profiles"), nil
}

// userProfiles lists the person's own profiles that still parse against the catalog.
func userProfiles(e *mcpkit.Env, cat *catalog.Catalog) ([]catalog.Profile, error) {
	dir, err := profileFolder(e)
	if err != nil {
		return nil, nil // no data folder: no own profiles
	}
	return (&profile.Store{Dir: dir}).List(cat)
}

// installArgs is the argument of the install tools: a profile or a list of apps, never both.
type installArgs struct {
	ProfileID string   `json:"profileId"`
	AppIDs    []string `json:"appIds"`
}

// validate checks the arguments and, for a list of apps, that every id is in the catalog. It returns the
// cleaned (de-duplicated) app ids.
func (a installArgs) validate(cat *catalog.Catalog) ([]string, error) {
	switch {
	case a.ProfileID != "" && len(a.AppIDs) > 0:
		return nil, errors.New("give profileId or appIds, not both")
	case a.ProfileID == "" && len(a.AppIDs) == 0:
		return nil, errors.New("give profileId or appIds")
	case a.ProfileID != "":
		return nil, checkID("profileId", a.ProfileID)
	}
	ids, err := checkIDs("appIds", a.AppIDs)
	if err != nil {
		return nil, err
	}
	return ids, knownApps(cat, ids)
}

type planOutAndPlan struct {
	plan install.Plan
	out  planOut
}

func toPlanOut(p install.Plan) planOut {
	o := planOut{Steps: []planStepOut{}, AlreadyInstalled: append([]string{}, p.AlreadyInstalled...),
		SkippedRecipes: append([]string{}, p.SkippedRecipes...), NeedsAdmin: p.NeedsAdmin}
	for _, s := range p.Steps {
		o.Steps = append(o.Steps, planStepOut{Kind: string(s.Kind), ID: s.ID, Name: s.Name, Version: s.Version, Admin: s.Admin})
	}
	return o
}

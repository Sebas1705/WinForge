package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/profile"
)

func emptyCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		Apps:     map[string]*catalog.App{"git": {ID: "git", Name: "Git"}},
		Profiles: map[string]*catalog.Profile{"base": {ID: "base", Name: "Base"}},
		Recipes:  map[string]*catalog.Recipe{},
	}
}

func okStores(t *testing.T) (func() (*profile.Store, error), func() (*install.PendingStore, error)) {
	dir := t.TempDir()
	return func() (*profile.Store, error) { return &profile.Store{Dir: filepath.Join(dir, "profiles")}, nil },
		func() (*install.PendingStore, error) {
			return &install.PendingStore{Path: filepath.Join(dir, "pending.json")}, nil
		}
}

func failStore() (*profile.Store, error) { return nil, errors.New("no config dir") }
func failPending() (*install.PendingStore, error) {
	return nil, errors.New("no config dir")
}

func TestNewAppHealthyHasNoStartupError(t *testing.T) {
	s, p := okStores(t)
	a := newApp(emptyCatalog(), s, p)
	if got := a.StartupError(); got != "" {
		t.Fatalf("StartupError() = %q, want empty", got)
	}
	if a.store == nil || a.pending == nil {
		t.Fatal("stores should be set")
	}
}

func TestNewAppSurvivesStoreFailures(t *testing.T) {
	s, p := okStores(t)
	cases := []struct {
		name    string
		store   func() (*profile.Store, error)
		pending func() (*install.PendingStore, error)
		want    []string
	}{
		{"profiles only", failStore, p, []string{"profiles: no config dir"}},
		{"pending only", s, failPending, []string{"unfinished runs: no config dir"}},
		{"both", failStore, failPending, []string{"profiles:", "unfinished runs:"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := newApp(emptyCatalog(), c.store, c.pending) // must not panic
			msg := a.StartupError()
			if msg == "" {
				t.Fatal("expected a startup error to surface to the UI")
			}
			for _, w := range c.want {
				if !strings.Contains(msg, w) {
					t.Errorf("StartupError() = %q, missing %q", msg, w)
				}
			}
		})
	}
}

func TestProfileOperationsWithoutStoreReturnErrors(t *testing.T) {
	a := newApp(emptyCatalog(), failStore, failPending)

	err := a.SaveProfile(catalog.Profile{ID: "mine", Name: "Mine", Apps: []catalog.ProfileApp{{ID: "git"}}})
	if !errors.Is(err, errStorageUnavailable) {
		t.Errorf("SaveProfile error = %v, want errStorageUnavailable", err)
	}
	if err := a.DeleteProfile("mine"); !errors.Is(err, errStorageUnavailable) {
		t.Errorf("DeleteProfile error = %v, want errStorageUnavailable", err)
	}
}

func TestSaveProfileValidationStillRunsFirst(t *testing.T) {
	a := newApp(emptyCatalog(), failStore, failPending)
	if err := a.SaveProfile(catalog.Profile{ID: "base"}); err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Errorf("want built-in id error, got %v", err)
	}
	if err := a.SaveProfile(catalog.Profile{ID: "x", Apps: []catalog.ProfileApp{{ID: "nope"}}}); err == nil || !strings.Contains(err.Error(), "unknown app") {
		t.Errorf("want unknown app error, got %v", err)
	}
}

func TestPendingOperationsWithoutStoreAreNoOps(t *testing.T) {
	a := newApp(emptyCatalog(), failStore, failPending)
	if p := a.loadPending(); p != nil {
		t.Errorf("loadPending() = %v, want nil", p)
	}
	a.savePending(install.Pending{Title: "t", Steps: []install.Step{{Kind: install.StepApp, ID: "git"}}}) // must not panic
	if err := a.DiscardPending(); err != nil {
		t.Errorf("DiscardPending() = %v, want nil", err)
	}
	if plan := a.PlanPending(); len(plan.Steps) != 0 {
		t.Errorf("PlanPending() steps = %v, want none", plan.Steps)
	}
	if err := a.ResumePending(); err == nil || !strings.Contains(err.Error(), "nothing to continue") {
		t.Errorf("ResumePending() = %v, want nothing to continue", err)
	}
}

func TestProfilesRoundTripWhenStoreWorks(t *testing.T) {
	s, p := okStores(t)
	a := newApp(emptyCatalog(), s, p)
	prof := catalog.Profile{ID: "mine", Name: "Mine", Apps: []catalog.ProfileApp{{ID: "git"}}}
	if err := a.SaveProfile(prof); err != nil {
		t.Fatal(err)
	}
	got, err := a.store.List(a.cat)
	if err != nil || len(got) != 1 || got[0].ID != "mine" || got[0].Kind != "custom" {
		t.Fatalf("List = %+v, %v", got, err)
	}
	if err := a.DeleteProfile("mine"); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.store.List(a.cat); len(got) != 0 {
		t.Fatalf("profile not deleted: %+v", got)
	}
}

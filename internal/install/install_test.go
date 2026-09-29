package install_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
)

type fakeExec struct {
	calls []string
	exit  map[string]int // key: substring of the joined command line
}

func (f *fakeExec) Run(_ context.Context, name string, args []string, onLine func(string)) (int, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	for k, code := range f.exit {
		if strings.Contains(line, k) {
			return code, nil
		}
	}
	onLine("ok")
	return 0, nil
}

func plan(t *testing.T, profile string, installed map[string]system.Installed) (*catalog.Catalog, install.Plan) {
	t.Helper()
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cat.Resolve(profile)
	if err != nil {
		t.Fatal(err)
	}
	return cat, install.BuildPlan(cat, res, installed)
}

func TestPlanSkipsInstalledAndOrdersRecipesLast(t *testing.T) {
	_, p := plan(t, "dev-base", map[string]system.Installed{"git": {ID: "git"}, "vscode": {ID: "vscode"}})
	if len(p.AlreadyInstalled) != 2 {
		t.Fatalf("installed: %v", p.AlreadyInstalled)
	}
	seenRecipe := false
	for _, s := range p.Steps {
		if s.Kind == install.StepRecipe {
			seenRecipe = true
		} else if seenRecipe {
			t.Fatal("an app step follows a recipe step")
		}
		if s.ID == "git" || s.ID == "vscode" {
			t.Fatalf("%s is installed and must not be planned", s.ID)
		}
	}
	if !p.NeedsAdmin {
		t.Fatal("windows-long-paths needs admin")
	}
}

func TestRecipeSkippedWhenItsAppIsNeitherInstalledNorPlanned(t *testing.T) {
	cat, _ := plan(t, "dev-base", nil)
	res := &catalog.Resolved{Recipes: []string{"rust-stable"}}
	p := install.BuildPlan(cat, res, nil)
	if len(p.Steps) != 0 || len(p.SkippedRecipes) != 1 {
		t.Fatalf("%+v", p)
	}
}

func TestRunContinuesPastFailuresButSkipsDependents(t *testing.T) {
	cat, p := plan(t, "dev-rust", nil)
	ex := &fakeExec{exit: map[string]int{"Microsoft.VisualStudio.2022.BuildTools": 1603}}
	var events []install.Event
	failed := install.Run(context.Background(), cat, p, ex, func(e install.Event) { events = append(events, e) })

	if len(failed) != 1 || failed[0] != "vs-build-tools-2022" {
		t.Fatalf("failed: %v", failed)
	}
	status := map[string]install.Status{}
	for _, e := range events {
		if e.Status != install.StatusOutput && e.Status != install.StatusStart {
			status[e.Step.ID] = e.Status
		}
	}
	if status["rustup"] != install.StatusSkipped || status["rust-stable"] != install.StatusSkipped {
		t.Fatalf("dependents of a failed app must be skipped: %v", status)
	}
	if status["git"] != install.StatusOK {
		t.Fatalf("independent apps must still install: %v", status)
	}
}

func TestAlreadyInstalledExitCodeIsSuccess(t *testing.T) {
	cat, p := plan(t, "essentials", nil)
	ex := &fakeExec{exit: map[string]int{"7zip.7zip": -1978335135}}
	if failed := install.Run(context.Background(), cat, p, ex, func(install.Event) {}); len(failed) != 0 {
		t.Fatalf("failed: %v", failed)
	}
}

func TestWingetArgsHonourOverrideScopeAndVersion(t *testing.T) {
	cat, _ := catalog.Load(catalogdata.FS)
	args := strings.Join(install.WingetArgs(cat.Apps["vs-build-tools-2022"], "17.9.0"), " ")
	for _, want := range []string{"--exact", "--source winget", "--version 17.9.0", "--override --passive"} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in %s", want, args)
		}
	}
}

func TestPowerShellArgsAreEncoded(t *testing.T) {
	a := install.PowerShellArgs(`Write-Output "it's fine"`)
	if a[len(a)-2] != "-EncodedCommand" || strings.Contains(a[len(a)-1], "fine") {
		t.Fatalf("%v", a)
	}
}

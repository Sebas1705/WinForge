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

func TestUpgradePlanOnlyTouchesCatalogApps(t *testing.T) {
	cat, _ := catalog.Load(catalogdata.FS)
	ups := []system.Upgrade{
		{ID: "Git.Git", Name: "Git", Current: "2.50", Available: "2.55"},
		{ID: "Some.Unrelated", Name: "Unrelated", Current: "1", Available: "2"},
		{ID: "Microsoft.VisualStudioCode", Name: "VS Code", Current: "1", Available: "2"},
	}
	p := install.UpgradePlan(cat, ups, nil)
	if len(p.Steps) != 2 || p.Steps[0].Kind != install.StepUpgrade || p.Steps[0].ID != "git" {
		t.Fatalf("%+v", p.Steps)
	}
	if only := install.UpgradePlan(cat, ups, []string{"vscode"}); len(only.Steps) != 1 || only.Steps[0].ID != "vscode" {
		t.Fatalf("%+v", only.Steps)
	}
	ex := &fakeExec{exit: map[string]int{"Git.Git": -1978335189}}
	if failed := install.Run(context.Background(), cat, p, ex, func(install.Event) {}); len(failed) != 0 {
		t.Fatalf("\"no newer version\" is not a failure: %v", failed)
	}
	if !strings.Contains(ex.calls[0], "upgrade --id Git.Git") {
		t.Fatal(ex.calls[0])
	}
}

func TestScriptIsReadableAndSafelyQuoted(t *testing.T) {
	cat, p := plan(t, "dev-base", nil)
	s := install.Script(cat, p, "Developer base")
	for _, want := range []string{"winget install --id Git.Git --exact", "function Invoke-Recipe", "Invoke-Recipe '", "@'\n", "elevated PowerShell"} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %q", want)
		}
	}
	vs := install.Script(cat, install.Plan{Steps: []install.Step{{Kind: install.StepApp, ID: "vs-build-tools-2022", Name: "It's VS"}}}, "x")
	if !strings.Contains(vs, "--override '--passive --wait --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended'") || !strings.Contains(vs, "Write-Host '== It''s VS'") {
		t.Fatalf("quoting: %s", vs)
	}
}

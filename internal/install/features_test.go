package install_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
)

var featureFeatures = []string{"wsl", "vmp", "hyperv", "sandbox", "netfx3"}

func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestFeatureRecipesExistAndAreAdminWithACheck(t *testing.T) {
	cat := loadCatalog(t)
	for _, f := range featureFeatures {
		for _, dir := range []string{"on", "off"} {
			id := "feature-" + f + "-" + dir
			r := cat.Recipes[id]
			if r == nil {
				t.Fatalf("%s: recipe is missing", id)
			}
			if !r.Admin || strings.TrimSpace(r.Check) == "" || strings.TrimSpace(r.PowerShell) == "" || r.Description == "" {
				t.Errorf("%s: must be admin, with a check, a script and a description", id)
			}
			if !strings.Contains(r.PowerShell, "exit 3010") {
				t.Errorf("%s: must exit 3010 when Windows asks for a restart", id)
			}
			if !strings.Contains(r.PowerShell, "/norestart") {
				t.Errorf("%s: must never restart the PC by itself", id)
			}
			if strings.Count(r.Description, ". ") != 0 {
				t.Errorf("%s: description should be one sentence: %q", id, r.Description)
			}
		}
	}
}

func TestFeatureRecipesNeverDeleteUserData(t *testing.T) {
	cat := loadCatalog(t)
	banned := []string{"remove-item", "del ", "rmdir", "--unregister", "--import", "--export", ".vhdx", "remove-vm", "/remove", "format-"}
	for id, r := range cat.Recipes {
		if !strings.HasPrefix(id, "feature-") {
			continue
		}
		low := strings.ToLower(r.PowerShell + "\n" + r.Check)
		for _, b := range banned {
			if strings.Contains(low, b) {
				t.Errorf("%s: must not contain %q", id, b)
			}
		}
	}
}

func TestWSLOnEnablesBothFeaturesAndOffTouchesOnlyWSL(t *testing.T) {
	cat := loadCatalog(t)
	on := cat.Recipes["feature-wsl-on"].PowerShell
	for _, want := range []string{"Microsoft-Windows-Subsystem-Linux", "VirtualMachinePlatform", "--set-default-version 2"} {
		if !strings.Contains(on, want) {
			t.Errorf("feature-wsl-on should mention %q", want)
		}
	}
	off := cat.Recipes["feature-wsl-off"].PowerShell
	if strings.Contains(off, "VirtualMachinePlatform") && strings.Contains(off, "/disable-feature /featurename:VirtualMachinePlatform") {
		t.Error("feature-wsl-off must never disable VirtualMachinePlatform")
	}
	if strings.Count(off, "/disable-feature") != 1 || !strings.Contains(off, "Microsoft-Windows-Subsystem-Linux") {
		t.Errorf("feature-wsl-off disables only the WSL feature:\n%s", off)
	}
}

func TestRepairPlanAcceptsFeatureRecipesWithARestorePointFirst(t *testing.T) {
	cat := loadCatalog(t)
	for _, f := range featureFeatures {
		for _, dir := range []string{"on", "off"} {
			id := "feature-" + f + "-" + dir
			p, err := install.RepairPlan(cat, []string{id})
			if err != nil {
				t.Fatalf("%s: %v", id, err)
			}
			if len(p.Steps) != 2 || p.Steps[0].ID != "repair-restore-point" || p.Steps[1].ID != id || !p.NeedsAdmin || !p.Steps[1].Admin || p.Steps[1].Kind != install.StepRecipe {
				t.Errorf("%s: restore point first, then the admin recipe: %+v", id, p)
			}
		}
	}
	p, err := install.RepairPlan(cat, []string{"feature-wsl-on", "feature-vmp-on", "feature-wsl-on", "repair-restore-point"})
	if err != nil || len(p.Steps) != 3 || p.Steps[0].ID != "repair-restore-point" || p.Steps[1].ID != "feature-wsl-on" || p.Steps[2].ID != "feature-vmp-on" {
		t.Fatalf("one restore point, duplicates dropped, order kept: %+v %v", p.Steps, err)
	}
	mixed, err := install.RepairPlan(cat, []string{"repair-firewall", "feature-hyperv-off"})
	if err != nil || len(mixed.Steps) != 3 || mixed.Steps[0].ID != "repair-restore-point" {
		t.Fatalf("a feature in a mixed plan still brings the restore point: %+v %v", mixed.Steps, err)
	}
}

func TestRepairPlanRefusesUnknownFeatureIdsAndProfileRecipes(t *testing.T) {
	cat := loadCatalog(t)
	for _, id := range []string{"feature-nope", "feature-", "feature", "Feature-wsl-on", "feature-wsl-on ", "../feature-wsl-on", "git-defaults"} {
		if _, err := install.RepairPlan(cat, []string{id}); err == nil {
			t.Errorf("%q must be refused", id)
		}
	}
	if _, err := install.RepairPlan(cat, []string{"feature-wsl-on", "feature-nope"}); err == nil {
		t.Error("one unknown feature id refuses the whole plan")
	}
	// A recipe that is not a maintenance recipe is refused even when it exists
	// in the catalog, and a profile's recipes can never reach RepairPlan.
	for pid := range cat.Profiles {
		res, err := cat.Resolve(pid)
		if err != nil {
			continue
		}
		for _, r := range res.Recipes {
			if strings.HasPrefix(r, "repair-") || strings.HasPrefix(r, "optimize-") || strings.HasPrefix(r, "feature-") {
				continue
			}
			if _, err := install.RepairPlan(cat, []string{r}); err == nil {
				t.Errorf("profile %s: recipe %q must not run through RepairPlan", pid, r)
			}
		}
	}
}

func TestFeatureRestartExitCodeReachesTheStepAsReboot(t *testing.T) {
	cat := loadCatalog(t)
	p, err := install.RepairPlan(cat, []string{"feature-wsl-on"})
	if err != nil {
		t.Fatal(err)
	}
	ex := &lineExec{lines: []string{"Restart Windows to finish turning on WSL."}, code: 3010}
	var rebootFor []string
	failed := install.Run(context.Background(), cat, p, ex, func(e install.Event) {
		if e.Reason == install.ReasonReboot {
			rebootFor = append(rebootFor, e.Step.ID)
		}
	})
	if len(failed) != 0 {
		t.Fatalf("3010 is success, not failure: %v", failed)
	}
	if len(rebootFor) != 2 || rebootFor[1] != "feature-wsl-on" {
		t.Fatalf("the feature step reports reason %q: %v", install.ReasonReboot, rebootFor)
	}
}

package install_test

import (
	"testing"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
)

func TestRepairPlanAddsARestorePointOnlyWhenDriversOrSystemFilesChange(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	ids := func(p install.Plan) []string {
		var out []string
		for _, s := range p.Steps {
			out = append(out, s.ID)
		}
		return out
	}
	p, err := install.RepairPlan(cat, []string{"repair-driver-updates", "repair-firewall", "repair-driver-updates"})
	if err != nil {
		t.Fatal(err)
	}
	got := ids(p)
	if len(got) != 3 || got[0] != "repair-restore-point" || got[1] != "repair-driver-updates" || got[2] != "repair-firewall" {
		t.Fatalf("restore point first, duplicates dropped: %v", got)
	}
	if !p.NeedsAdmin || p.Steps[1].Kind != install.StepRecipe || !p.Steps[1].Admin {
		t.Fatalf("%+v", p)
	}
	safe, err := install.RepairPlan(cat, []string{"repair-firewall", "repair-defender-signatures"})
	if err != nil || len(safe.Steps) != 2 || safe.Steps[0].ID == "repair-restore-point" {
		t.Fatalf("settings-only repairs need no restore point: %v %v", ids(safe), err)
	}
	just, _ := install.RepairPlan(cat, []string{"repair-restore-point", "repair-windows-files"})
	if g := ids(just); len(g) != 2 || g[0] != "repair-restore-point" || g[1] != "repair-windows-files" {
		t.Fatalf("an explicit restore point is not doubled: %v", g)
	}
}

func TestRepairPlanRefusesAnythingThatIsNotARepair(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"git-defaults", "rust-stable", "repair-nope", "", "../repair-firewall"} {
		if _, err := install.RepairPlan(cat, []string{id}); err == nil {
			t.Errorf("%q must be refused", id)
		}
	}
	if _, err := install.RepairPlan(cat, []string{"repair-firewall", "git-defaults"}); err == nil {
		t.Error("one bad id refuses the whole plan")
	}
	empty, err := install.RepairPlan(cat, nil)
	if err != nil || len(empty.Steps) != 0 {
		t.Fatalf("no repairs, no steps: %+v %v", empty, err)
	}
}

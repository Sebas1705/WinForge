package actions

import (
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/services"
	"github.com/Sebas1705/WinForge/internal/startup"
)

// ---- startup ----

func TestStartupToggleSummaryRunAndUndo(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	p := h.prepare("winforge_prepare_startup_toggle", `{"id":"hkcu-run|Chat","enabled":false}`)
	if p.Kind != "startup-toggle" || p.NeedsAdmin {
		t.Fatalf("%+v", p)
	}
	sum := p.Summary.(startupSummary)
	if sum.Name != "Chat" || sum.From != "starts with Windows" || sum.To != "does not start with Windows" || !strings.Contains(sum.Undo, "enabled=true") {
		t.Fatalf("%+v", sum)
	}
	if b.start.writeCount() != 0 {
		t.Fatal("prepare wrote")
	}
	res := h.ran(p.PlanID)
	if r := res.Structured.(simpleResult); !r.Changed || res.IsError || !strings.Contains(r.Undo, "enabled=true") {
		t.Fatalf("%+v", r)
	}
	if b.start.writeCount() != 1 {
		t.Errorf("writes %v", b.start.writes)
	}
	// It is off now, so asking for off again is refused, and on is allowed (the undo).
	h.refused("winforge_prepare_startup_toggle", `{"id":"hkcu-run|Chat","enabled":false}`)
	h.prepare("winforge_prepare_startup_toggle", `{"id":"hkcu-run|Chat","enabled":true}`)
}

func TestStartupToggleValidation(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	cases := map[string]string{
		`{"id":"hkcu-run|Nope","enabled":true}`:  "not in the startup list",
		`{"id":"../../x","enabled":true}`:        "not in the startup list",
		`{"id":"hkcu-run|Chat","enabled":true}`:  "already",
		`{"id":"hkcu-run|Chat"}`:                 "required",
		`{"enabled":true}`:                       "required",
		`{"id":"hkcu-run|Chat","enabled":"yes"}`: "",
	}
	for args, want := range cases {
		if got := h.refused("winforge_prepare_startup_toggle", args); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", args, got, want)
		}
	}
	// A machine item needs administrator rights.
	*b.admin = false
	if got := h.refused("winforge_prepare_startup_toggle", `{"id":"hklm-run|Audio","enabled":false}`); !strings.Contains(got, "administrator") {
		t.Errorf("%s", got)
	}
	h.prepare("winforge_prepare_startup_toggle", `{"id":"hkcu-run|Chat","enabled":false}`)
	if b.start.writeCount() != 0 {
		t.Fatal("a refusal must not write")
	}
}

func TestStartupMachineItemWithAdminAndWarnings(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	p := h.prepare("winforge_prepare_startup_toggle", `{"id":"hklm-run|Audio","enabled":false}`)
	if !p.NeedsAdmin {
		t.Error("a machine item needs admin")
	}
	// A missing program is called out.
	p = h.prepare("winforge_prepare_startup_toggle", `{"id":"hkcu-run|Gone","enabled":false}`)
	if w := p.Summary.(startupSummary).Warnings; len(w) != 1 || !strings.Contains(w[0], "not there") {
		t.Errorf("%v", w)
	}
}

func TestStartupRunRefusesWhenTheItemChanged(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	p := h.prepare("winforge_prepare_startup_toggle", `{"id":"hkcu-run|Chat","enabled":false}`)
	b.start.setCommand(startup.UserRun, "Chat", `"C:\Other\evil.exe"`)
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "changed or is gone") || b.start.writeCount() != 0 {
		t.Errorf("%s", got)
	}
	// Lost admin between prepare and run.
	p = h.prepare("winforge_prepare_startup_toggle", `{"id":"hklm-run|Audio","enabled":false}`)
	*b.admin = false
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "administrator") || b.start.writeCount() != 0 {
		t.Errorf("%s", got)
	}
}

// ---- services ----

func svcItem(t *testing.T, b *bundle, id string) services.Item {
	t.Helper()
	items, err := b.Services.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("no service %s in %v", id, items)
	return services.Item{}
}

func TestServiceModeListsDependentsAndRunsWithConfirmation(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	// Parent has a dependent (Child): lowering it needs confirmation, which the approved plan stands for.
	p := h.prepare("winforge_prepare_service_mode", `{"id":"Parent","mode":"disabled"}`)
	if p.Kind != "service-mode" || !p.NeedsAdmin {
		t.Fatalf("%+v", p)
	}
	sum := p.Summary.(serviceSummary)
	if len(sum.Dependents) != 1 || sum.Dependents[0] != "Vendor Child" || len(sum.Warnings) != 1 || sum.From != "Automatic" || sum.To != "Disabled" {
		t.Fatalf("%+v", sum)
	}
	if b.svc.WriteCount() != 0 {
		t.Fatal("prepare wrote")
	}
	res := h.ran(p.PlanID)
	r := res.Structured.(serviceResult)
	if !r.Changed || res.IsError || r.To != "Disabled" || !strings.Contains(r.Undo, "service_restore") {
		t.Fatalf("%+v", r)
	}
	if b.svc.WriteCount() != 1 || svcItem(t, b, "Parent").StartMode != services.Disabled {
		t.Errorf("writes %d mode %v", b.svc.WriteCount(), svcItem(t, b, "Parent").StartMode)
	}

	// Restore original: planned from what WinForge remembers, run through the same confirmation rule.
	rp := h.prepare("winforge_prepare_service_restore", `{"id":"Parent"}`)
	if rs := rp.Summary.(serviceSummary); rs.To != "Automatic" || rs.From != "Disabled" {
		t.Fatalf("%+v", rs)
	}
	h.ran(rp.PlanID)
	if svcItem(t, b, "Parent").StartMode != services.Auto || svcItem(t, b, "Parent").Changed {
		t.Errorf("%+v", svcItem(t, b, "Parent"))
	}
	// Nothing left to restore.
	h.refused("winforge_prepare_service_restore", `{"id":"Parent"}`)
}

func TestServiceModeWithoutDependentsNeedsNoConfirmationAndSecurityIsWarned(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	p := h.prepare("winforge_prepare_service_mode", `{"id":"Solo","mode":"manual"}`)
	if len(p.Summary.(serviceSummary).Warnings) != 0 {
		t.Errorf("%+v", p.Summary)
	}
	h.ran(p.PlanID)
	// Kaspersky looks like a security product: the plan says so, and the approved plan confirms.
	p = h.prepare("winforge_prepare_service_mode", `{"id":"AVP21","mode":"disabled"}`)
	sum := p.Summary.(serviceSummary)
	if !sum.Security || len(sum.Warnings) == 0 || !strings.Contains(sum.Warnings[0], "security") {
		t.Fatalf("%+v", sum)
	}
	if res := h.ran(p.PlanID); res.IsError || svcItem(t, b, "AVP21").StartMode != services.Disabled {
		t.Errorf("%s", res.Text)
	}
}

func TestServiceModeRaisingTheModeNeedsNoDependentWarning(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	// Child (manual) -> auto raises; Parent has dependents but is already auto. Steam manual -> auto has none.
	p := h.prepare("winforge_prepare_service_mode", `{"id":"Child","mode":"auto"}`)
	if len(p.Summary.(serviceSummary).Warnings) != 0 {
		t.Errorf("%+v", p.Summary)
	}
}

func TestServiceModeValidation(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	cases := map[string]string{
		`{"id":"WinDefend","mode":"disabled"}`:             "not in the services list", // protected: never listed
		`{"id":"windefend","mode":"disabled"}`:             "not in the services list",
		`{"id":"Dhcp","mode":"disabled"}`:                  "not in the services list",
		`{"id":"Nope","mode":"disabled"}`:                  "not in the services list",
		`{"id":"Parent","mode":"auto"}`:                    "already",
		`{"id":"Parent","mode":"turbo"}`:                   "mode must be",
		`{"id":"","mode":"manual"}`:                        "id is required",
		`{"id":"Parent"}`:                                  "mode must be",
		`{"id":"Parent","mode":"manual","confirmed":true}`: "",
	}
	for args, want := range cases {
		if got := h.refused("winforge_prepare_service_mode", args); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", args, got, want)
		}
	}
	h.refused("winforge_prepare_service_restore", `{"id":"Solo"}`) // never changed by WinForge
	*b.admin = false
	if got := h.refused("winforge_prepare_service_mode", `{"id":"Solo","mode":"manual"}`); !strings.Contains(got, "administrator") {
		t.Errorf("%s", got)
	}
	if b.svc.WriteCount() != 0 {
		t.Fatal("a refusal must not write")
	}
}

func TestServiceModeRefusesWhenTheWorldChanged(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	p := h.prepare("winforge_prepare_service_mode", `{"id":"Solo","mode":"disabled"}`)
	// A new service now depends on Solo: the plan did not list it, so it must not run.
	b.svc.Services = append(b.svc.Services, svcRaw("Late", "Vendor Late", 2, `"C:\Program Files\Vendor\late.exe"`, "Solo"))
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "changed or is gone") || b.svc.WriteCount() != 0 {
		t.Errorf("%s", got)
	}
	// The start mode changed behind the plan's back.
	p = h.prepare("winforge_prepare_service_mode", `{"id":"Steam Client Service","mode":"disabled"}`)
	for i := range b.svc.Services {
		if b.svc.Services[i].Name == "Steam Client Service" {
			b.svc.Services[i].Start = 2
		}
	}
	h.ranRefused(p.PlanID)
	// Lost admin.
	p = h.prepare("winforge_prepare_service_mode", `{"id":"Solo","mode":"manual"}`)
	*b.admin = false
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "administrator") || b.svc.WriteCount() != 0 {
		t.Errorf("%s", got)
	}
}

// ---- scheduled tasks ----

func TestTaskToggleRunAndRestore(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	*b.admin = false // the user's own limited task needs no administrator rights
	p := h.prepare("winforge_prepare_task_toggle", `{"id":"\\MyBackup","enabled":false}`)
	if p.Kind != "task-toggle" || p.NeedsAdmin {
		t.Fatalf("%+v", p)
	}
	sum := p.Summary.(taskSummary)
	if sum.From != "enabled" || sum.To != "disabled" || sum.Name != "MyBackup" || len(sum.Triggers) == 0 {
		t.Fatalf("%+v", sum)
	}
	if b.tsk.WriteCount() != 0 {
		t.Fatal("prepare wrote")
	}
	res := h.ran(p.PlanID)
	if r := res.Structured.(simpleResult); !r.Changed || r.Now != "disabled" || res.IsError {
		t.Fatalf("%+v", r)
	}
	if b.tsk.WriteCount() != 1 {
		t.Errorf("writes %d", b.tsk.WriteCount())
	}
	rp := h.prepare("winforge_prepare_task_restore", `{"id":"\\MyBackup"}`)
	if rs := rp.Summary.(taskSummary); rs.To != "enabled" {
		t.Fatalf("%+v", rs)
	}
	h.ran(rp.PlanID)
	h.refused("winforge_prepare_task_restore", `{"id":"\\MyBackup"}`) // nothing left to restore
	h.refused("winforge_prepare_task_toggle", `{"id":"\\MyBackup","enabled":true}`)
}

func TestTaskToggleRules(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	// Disabling a security product's task is warned about, and the approved plan confirms it.
	p := h.prepare("winforge_prepare_task_toggle", `{"id":"\\Avast Update","enabled":false}`)
	sum := p.Summary.(taskSummary)
	if !sum.Security || len(sum.Warnings) != 1 || !p.NeedsAdmin {
		t.Fatalf("%+v", sum)
	}
	if res := h.ran(p.PlanID); res.IsError {
		t.Fatalf("%s", res.Text)
	}
	cases := map[string]string{
		`{"id":"\\Microsoft\\Windows\\Defrag\\ScheduledDefrag","enabled":false}`: "not in the scheduled tasks list",
		`{"id":"\\Nope","enabled":false}`:                                        "not in the scheduled tasks list",
		`{"id":"\\GoogleUpdateTaskMachineCore","enabled":true}`:                  "already",
		`{"id":"\\MyBackup"}`:                                                    "required",
		`{"enabled":true}`:                                                       "required",
	}
	for args, want := range cases {
		if got := h.refused("winforge_prepare_task_toggle", args); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", args, got, want)
		}
	}
	// A machine task needs administrator rights.
	*b.admin = false
	if got := h.refused("winforge_prepare_task_toggle", `{"id":"\\GoogleUpdateTaskMachineCore","enabled":false}`); !strings.Contains(got, "administrator") {
		t.Errorf("%s", got)
	}
}

func TestTaskRunRefusesWhenTheWorldChanged(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	p := h.prepare("winforge_prepare_task_toggle", `{"id":"\\GoogleUpdateTaskMachineCore","enabled":false}`)
	b.tsk.Tasks[0].State = 1 // somebody disabled it already
	writes := b.tsk.WriteCount()
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "changed or is gone") || b.tsk.WriteCount() != writes {
		t.Errorf("%s", got)
	}
	p = h.prepare("winforge_prepare_task_toggle", `{"id":"\\Avast Update","enabled":false}`)
	*b.admin = false
	if got := h.ranRefused(p.PlanID); !strings.Contains(got, "administrator") || b.tsk.WriteCount() != writes {
		t.Errorf("%s", got)
	}
}

func TestTaskActionPathIsRedacted(t *testing.T) {
	b := newDeps(t)
	b.tsk.Tasks[1].XML = taskXML("me", `<LogonTrigger/>`, `PC\sebas`, `C:\Users\`+planted+`\Backup\b.exe`)
	h := b.env(t, nil)
	*b.admin = false
	p := h.prepare("winforge_prepare_task_toggle", `{"id":"\\MyBackup","enabled":false}`)
	if got := p.Summary.(taskSummary).Program; got == "" || strings.Contains(got, planted) {
		t.Errorf("program %q", got)
	}
}

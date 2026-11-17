package actions

import (
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/privacy"
)

// The privacy tests run the real engine over an in-memory registry, so the summary can be compared with
// what the registry holds after Run.

const (
	cdmKey      = `Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager`
	explorerKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`
)

func privacyHarness(t *testing.T) (*bundle, *harness) {
	b := newDeps(t)
	b.reg.putDWord(advKey, "Enabled", 1)               // advertising-id: on, apply turns it off
	b.reg.putDWord(explorerKey, "Start_TrackProgs", 0) // start-most-used: already private
	return b, b.env(t, nil)
}

func dwordOf(v privacy.Value) string { return privacy.Display(v) }

func TestPrivacyApplySummaryEqualsWhatIsWritten(t *testing.T) {
	b, h := privacyHarness(t)
	p := h.prepare("winforge_prepare_privacy", `{"apply":["advertising-id","start-most-used","settings-suggestions"]}`)
	if p.Kind != "privacy" || p.NeedsAdmin {
		t.Fatalf("%+v", p)
	}
	sum := p.Summary.(privacySummary)
	if b.reg.writeCount() != 0 {
		t.Fatal("prepare wrote to the registry")
	}
	// advertising-id (1 value) + settings-suggestions (3 values, absent) change; start-most-used is already private.
	if len(sum.Changes) != 4 || len(sum.AlreadyAsAsked) != 1 || sum.AlreadyAsAsked[0] != "start-most-used" {
		t.Fatalf("%+v", sum)
	}
	for _, c := range sum.Changes {
		if !strings.HasPrefix(c.Path, `HKEY_CURRENT_USER\`) || c.Type != "REG_DWORD" || c.New != "0" || c.Action != "apply" {
			t.Errorf("%+v", c)
		}
	}
	if sum.Changes[0].Old != "1" || sum.Changes[1].Old != "(does not exist)" {
		t.Errorf("old values: %+v", sum.Changes)
	}

	res := h.ran(p.PlanID)
	if res.IsError {
		t.Fatalf("%s", res.Text)
	}
	r := res.Structured.(privacyResult)
	if r.Applied != 2 || len(r.BackupIDs) != 1 || len(r.Failed) != 0 {
		t.Fatalf("%+v", r)
	}
	// Exactly what the summary listed is now in the registry, and nothing else was written.
	for _, c := range sum.Changes {
		got := b.reg.get(strings.TrimPrefix(c.Path, `HKEY_CURRENT_USER\`), c.Name)
		if !got.Exists || dwordOf(got) != c.New {
			t.Errorf("%s\\%s = %+v, summary said %s", c.Path, c.Name, got, c.New)
		}
	}
	if b.reg.writeCount() != 4 {
		t.Errorf("writes = %d, want the 4 listed", b.reg.writeCount())
	}
}

func TestPrivacyResetSummaryAndRun(t *testing.T) {
	b, h := privacyHarness(t)
	b.reg.putDWord(cdmKey, "SoftLandingEnabled", 0) // tips-welcome: one value at the private setting; resetting removes it
	p := h.prepare("winforge_prepare_privacy", `{"reset":["tips-welcome","advertising-id"]}`)
	sum := p.Summary.(privacySummary)
	// advertising-id is at Windows' own setting (1), so a reset has nothing to do for it.
	if len(sum.Changes) != 1 || sum.Changes[0].Name != "SoftLandingEnabled" || sum.Changes[0].New != "(value removed)" || sum.Changes[0].Old != "0" || sum.Changes[0].Action != "reset" {
		t.Fatalf("%+v", sum)
	}
	if len(sum.AlreadyAsAsked) != 1 || sum.AlreadyAsAsked[0] != "advertising-id" || b.reg.writeCount() != 0 {
		t.Fatalf("%+v writes %d", sum, b.reg.writeCount())
	}
	h.ran(p.PlanID)
	if b.reg.get(cdmKey, "SoftLandingEnabled").Exists || !b.reg.get(advKey, "Enabled").Exists {
		t.Error("reset removed the wrong values")
	}
	if b.reg.writeCount() != 1 {
		t.Errorf("writes %d", b.reg.writeCount())
	}
}

func TestPrivacyValidation(t *testing.T) {
	b, h := privacyHarness(t)
	cases := map[string]string{
		`{}`:                       "no setting",
		`{"apply":[],"reset":[]}`:  "no setting",
		`{"apply":["nope"]}`:       "not a privacy setting",
		`{"reset":["../../evil"]}`: "not a privacy setting",
		`{"apply":["advertising-id"],"reset":["advertising-id"]}`: "both",
		`{"apply":["start-most-used"]}`:                           "nothing to change",
		`{"apply":"advertising-id"}`:                              "",
	}
	for args, want := range cases {
		if got := h.refused("winforge_prepare_privacy", args); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", args, got, want)
		}
	}
	// One unknown id refuses the whole request even next to a good one.
	h.refused("winforge_prepare_privacy", `{"apply":["advertising-id","nope"]}`)
	if b.reg.writeCount() != 0 {
		t.Fatal("a refusal must not write")
	}
}

func TestPrivacyRunRefusesWhenTheRegistryChangedSincePrepare(t *testing.T) {
	b, h := privacyHarness(t)
	p := h.prepare("winforge_prepare_privacy", `{"apply":["advertising-id"]}`)
	b.reg.putDWord(advKey, "Enabled", 7) // someone edited the value
	got := h.ranRefused(p.PlanID)
	if !strings.Contains(got, "no longer matches") || b.reg.writeCount() != 0 {
		t.Errorf("%s writes %d", got, b.reg.writeCount())
	}
}

func TestPrivacyRestoreUndoesExactlyAndOnlyOnce(t *testing.T) {
	b, h := privacyHarness(t)
	p := h.prepare("winforge_prepare_privacy", `{"apply":["advertising-id","tips-welcome"]}`)
	r := h.ran(p.PlanID).Structured.(privacyResult)
	if len(r.BackupIDs) != 1 {
		t.Fatalf("%+v", r)
	}
	id := r.BackupIDs[0]
	if b.reg.get(advKey, "Enabled").Data[0] != 0 {
		t.Fatal("apply did not write")
	}
	writesAfterApply := b.reg.writeCount()

	rp := h.prepare("winforge_prepare_privacy_restore", `{"backupId":"`+id+`"}`)
	if rp.Kind != "privacy-restore" || b.reg.writeCount() != writesAfterApply {
		t.Fatalf("prepare must not write: %+v", rp)
	}
	sum := rp.Summary.(restoreSummary)
	if sum.BackupID != id || len(sum.Items) != 2 || sum.Action != "apply" {
		t.Errorf("%+v", sum)
	}
	res := h.ran(rp.PlanID)
	rr := res.Structured.(privacyRestoreResult)
	if len(rr.Restored) != 2 || len(rr.ChangedSince) != 0 || rr.Error != "" {
		t.Fatalf("%+v", rr)
	}
	// The registry is exactly as it was: the value is back to 1, and the ones that did not exist are gone.
	if v := b.reg.get(advKey, "Enabled"); dwordOf(v) != "1" {
		t.Errorf("advertising value %+v", v)
	}
	if b.reg.get(cdmKey, "SoftLandingEnabled").Exists {
		t.Error("a value that did not exist must be removed again")
	}
	// The same backup cannot be restored twice.
	if got := h.refused("winforge_prepare_privacy_restore", `{"backupId":"`+id+`"}`); !strings.Contains(got, "already restored") {
		t.Errorf("%s", got)
	}
}

func TestPrivacyRestoreValidation(t *testing.T) {
	_, h := privacyHarness(t)
	for _, args := range []string{`{}`, `{"backupId":""}`, `{"backupId":"../../x"}`, `{"backupId":"20260101T000000Z"}`, `{"backupId":"x","path":"C:\\"}`} {
		h.refused("winforge_prepare_privacy_restore", args)
	}
}

func TestPrivacyRestoreRefusesAtRunWhenTheBackupIsGone(t *testing.T) {
	b, h := privacyHarness(t)
	r := h.ran(h.prepare("winforge_prepare_privacy", `{"apply":["advertising-id"]}`).PlanID).Structured.(privacyResult)
	rp := h.prepare("winforge_prepare_privacy_restore", `{"backupId":"`+r.BackupIDs[0]+`"}`)
	// The backup is restored behind the plan's back (the GUI, another run).
	if _, err := (&privacy.Manager{Sys: b.reg}).Restore(r.BackupIDs[0]); err != nil {
		t.Fatal(err)
	}
	writes := b.reg.writeCount()
	got := h.ranRefused(rp.PlanID)
	if !strings.Contains(got, "already restored") || b.reg.writeCount() != writes {
		t.Errorf("%s", got)
	}
}

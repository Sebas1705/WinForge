package actions

import (
	"strings"
	"testing"
)

const (
	idGone    = `path-missing|user|C:\gone`
	idHome    = `home-missing|user|JAVA_HOME`
	idMachine = `path-missing|machine|C:\machine-gone`
	idUserBin = `path-missing|user|C:\Users\` + planted + `\gone-bin`
)

func TestEnvFixSummaryAndRun(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	p := h.prepare("winforge_prepare_env_fix", `{"findingIds":["`+strings.ReplaceAll(idGone, `\`, `\\`)+`","`+idHome+`"]}`)
	if p.Kind != "env-fix" || p.NeedsAdmin {
		t.Fatalf("%+v", p)
	}
	sum := p.Summary.(envFixSummary)
	if len(sum.Findings) != 2 || !strings.Contains(sum.Findings[0].Fix, "Remove the entry") || !strings.Contains(sum.Findings[1].Fix, "Delete the user variable JAVA_HOME") {
		t.Fatalf("%+v", sum)
	}
	if b.envsys.writeCount() != 0 {
		t.Fatal("prepare wrote")
	}
	res := h.ran(p.PlanID)
	r := res.Structured.(envFixResult)
	if r.Applied != 2 || r.BackupID == "" || len(r.Failed) != 0 || res.IsError {
		t.Fatalf("%+v", r)
	}
	// Exactly the two listed things changed: C:\gone left the user PATH, JAVA_HOME is gone, the rest stays as written.
	if got := b.envsys.value("Path"); got != `C:\keep;C:\KEEP\;rel;C:\Users\`+planted+`\gone-bin` {
		t.Errorf("Path = %q", got)
	}
	if b.envsys.value("JAVA_HOME") != "" {
		t.Error("JAVA_HOME should be deleted")
	}
	if b.envsys.value("Path") == "" || len(b.envsys.writes) != 2 {
		t.Errorf("writes %v", b.envsys.writes)
	}
	if !strings.Contains(r.Undo, "winforge_prepare_env_restore") {
		t.Error("undo must name the restore tool")
	}
}

func TestEnvFixValidation(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	cases := map[string]string{
		`{"findingIds":[]}`:                          "",
		`{"findingIds":["nope"]}`:                    "not a finding",
		`{"findingIds":["` + idMachine + `"]}`:       "machine-wide",
		`{"findingIds":["path-duplicate|user|rel"]}`: "not a finding",
		`{}`: "",
	}
	for args, want := range cases {
		args = strings.ReplaceAll(args, `\`, `\\`)
		if got := h.refused("winforge_prepare_env_fix", args); !strings.Contains(got, want) {
			t.Errorf("%s: %q lacks %q", args, got, want)
		}
	}
	// A finding that is reported but has no automatic fix: the tool-shadowed kind has none.
	rep, _ := b.Env.Scan()
	for _, f := range rep.Findings {
		if !f.Fixable && f.Scope == "user" {
			h.refused("winforge_prepare_env_fix", `{"findingIds":["`+strings.ReplaceAll(f.ID, `\`, `\\`)+`"]}`)
		}
	}
	if b.envsys.writeCount() != 0 {
		t.Fatal("a refusal must not write")
	}
}

func TestEnvFixAcceptsTheRedactedIdAModelRead(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	redacted := strings.ReplaceAll(strings.ReplaceAll(idUserBin, planted, "<user>"), `\`, `\\`)
	p := h.prepare("winforge_prepare_env_fix", `{"findingIds":["`+redacted+`"]}`)
	sum := p.Summary.(envFixSummary)
	if len(sum.Findings) != 1 || strings.Contains(asJSON(t, sum), planted) {
		t.Fatalf("%+v", sum)
	}
	h.ran(p.PlanID)
	if strings.Contains(b.envsys.value("Path"), "gone-bin") {
		t.Error("the entry should be removed")
	}
}

func TestEnvFixRefusesWhenTheEnvironmentChanged(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	p := h.prepare("winforge_prepare_env_fix", `{"findingIds":["`+strings.ReplaceAll(idGone, `\`, `\\`)+`"]}`)
	b.envsys.set("Path", `C:\keep;C:\other-gone`) // C:\gone is no longer there
	got := h.ranRefused(p.PlanID)
	if !strings.Contains(got, "changed since") || b.envsys.writeCount() != 0 {
		t.Errorf("%s %v", got, b.envsys.writes)
	}
}

func TestEnvRestoreRoundTrip(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	before := b.envsys.value("Path")
	r := h.ran(h.prepare("winforge_prepare_env_fix", `{"findingIds":["`+strings.ReplaceAll(idGone, `\`, `\\`)+`"]}`).PlanID).Structured.(envFixResult)
	writes := b.envsys.writeCount()
	rp := h.prepare("winforge_prepare_env_restore", `{"backupId":"`+r.BackupID+`"}`)
	if b.envsys.writeCount() != writes {
		t.Fatal("prepare wrote")
	}
	sum := rp.Summary.(restoreSummary)
	if len(sum.Items) != 1 || sum.Items[0] != "Path" || sum.BackupID != r.BackupID {
		t.Fatalf("%+v", sum)
	}
	rr := h.ran(rp.PlanID).Structured.(envRestoreResult)
	if len(rr.Restored) != 1 || len(rr.ChangedSince) != 0 || b.envsys.value("Path") != before {
		t.Errorf("%+v path %q", rr, b.envsys.value("Path"))
	}
}

func TestEnvRestoreValidationAndGoneBackup(t *testing.T) {
	b := newDeps(t)
	h := b.env(t, nil)
	for _, args := range []string{`{}`, `{"backupId":"../x"}`, `{"backupId":"20260101T000000Z"}`, `{"backupId":"20260101T000000Z","dir":"C:\\"}`} {
		h.refused("winforge_prepare_env_restore", args)
	}
	r := h.ran(h.prepare("winforge_prepare_env_fix", `{"findingIds":["`+idHome+`"]}`).PlanID).Structured.(envFixResult)
	rp := h.prepare("winforge_prepare_env_restore", `{"backupId":"`+r.BackupID+`"}`)
	// The backup file disappears before the plan runs.
	if len(mustBackups(t, b)) != 1 {
		t.Fatal("the backup should be listed before it disappears")
	}
	b.envsys.dir = t.TempDir() // another folder: the backup is not there any more
	writes := b.envsys.writeCount()
	if got := h.ranRefused(rp.PlanID); !strings.Contains(got, "no longer in the list") || b.envsys.writeCount() != writes {
		t.Errorf("%s", got)
	}
}

func mustBackups(t *testing.T, b *bundle) []string {
	t.Helper()
	list, err := b.Env.Backups()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, x := range list {
		ids = append(ids, x.ID)
	}
	return ids
}

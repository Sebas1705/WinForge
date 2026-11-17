package privacy

import "testing"

// Preview is the plan Apply and Reset make, read without writing: it must list exactly what they then change.
func TestPreviewListsWhatApplyAndResetThenChange(t *testing.T) {
	f := newFake(t)
	f.putDWord(keyAdv, "Enabled", 1)
	f.putDWord(keyExplorer, "Start_TrackProgs", 0) // already private
	m := &Manager{Sys: f}

	got, res, err := m.Preview([]string{"advertising-id", "start-most-used"}, ActionApply)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 0 {
		t.Fatalf("Preview wrote: %v", f.writes)
	}
	if len(got) != 1 || got[0].Tweak != "advertising-id" || got[0].Old != "1" || got[0].New != "0" || got[0].Remove || got[0].NewType != "REG_DWORD" ||
		got[0].Path != `HKEY_CURRENT_USER\`+keyAdv {
		t.Fatalf("%+v", got)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "start-most-used" {
		t.Fatalf("%+v", res)
	}
	if r, err := m.Apply([]string{"advertising-id", "start-most-used"}); err != nil || r.Applied != 1 || len(f.writes) != 1 {
		t.Fatalf("%+v %v writes %v", r, err, f.writes)
	}

	// Reset: the value is now at the private setting, which is not Windows' own, so a reset removes it.
	got, _, err = m.Preview([]string{"advertising-id"}, ActionReset)
	if err != nil || len(got) != 1 || !got[0].Remove || got[0].Old != "0" || got[0].NewType != "" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestPreviewRefusesUnknownIDsAndActions(t *testing.T) {
	m := &Manager{Sys: newFake(t)}
	if _, _, err := m.Preview([]string{"nope"}, ActionApply); err != ErrUnknownTweak {
		t.Errorf("%v", err)
	}
	if _, _, err := m.Preview([]string{"advertising-id"}, "delete-everything"); err == nil {
		t.Error("an unknown action must be refused")
	}
}

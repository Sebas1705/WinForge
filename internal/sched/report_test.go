package sched

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTheLogKeepsTheLast30Runs(t *testing.T) {
	s := LogStore{Dir: t.TempDir()}
	for i := 0; i < KeepRuns+7; i++ {
		r := Report{Started: day0.Add(time.Duration(i) * time.Hour), Mode: Notify, Items: []Item{{ID: "git", Name: "Git", Result: ResultFound}}}
		if err := s.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 30 {
		t.Fatalf("%d runs kept", len(runs))
	}
	if !runs[0].Started.Equal(day0.Add(36*time.Hour)) || !runs[29].Started.Equal(day0.Add(7*time.Hour)) {
		t.Errorf("newest first, the oldest 7 dropped: %v ... %v", runs[0].Started, runs[29].Started)
	}
	files, _ := os.ReadDir(filepath.Join(s.Dir, "runs"))
	if len(files) != 30 {
		t.Errorf("files on disk: %d", len(files))
	}
}

func TestTheLogSurvivesADamagedFileAndAMissingFolder(t *testing.T) {
	s := LogStore{Dir: filepath.Join(t.TempDir(), "nothing-here")}
	if runs, err := s.List(); err != nil || len(runs) != 0 {
		t.Fatalf("a log that does not exist yet is empty: %v %v", runs, err)
	}
	s.Save(Report{Started: day0, Items: nil})
	os.WriteFile(filepath.Join(s.Dir, "runs", "29990101T000000Z.json"), []byte("{broken"), 0o644)
	os.WriteFile(filepath.Join(s.Dir, "runs", "note.txt"), []byte("not a run"), 0o644)
	runs, err := s.List()
	if err != nil || len(runs) != 1 || runs[0].Items == nil {
		t.Fatalf("a damaged entry must not hide the others: %+v %v", runs, err)
	}
}

func TestRunsKeepTheirVersionsForTheLog(t *testing.T) {
	s := LogStore{Dir: t.TempDir()}
	in := Report{Started: day0, Finished: day0.Add(time.Minute), Mode: Apply, Scheduled: true, Items: []Item{
		{ID: "git", Name: "Git", From: "1.0", To: "2.0", Result: ResultUpdated, Reboot: true},
		{ID: "vlc", Name: "VLC", From: "3.0", To: "3.1", Result: ResultFailed, Reason: "network"},
	}}
	if err := s.Save(in); err != nil {
		t.Fatal(err)
	}
	out, _ := s.List()
	if len(out) != 1 || out[0].Items[0] != in.Items[0] || out[0].Items[1] != in.Items[1] || out[0].ID != "20261116T100000Z" {
		t.Errorf("%+v", out)
	}
}

func TestStatus(t *testing.T) {
	it := func(rs ...string) []Item {
		var out []Item
		for i, r := range rs {
			out = append(out, Item{ID: fmt.Sprint(i), Result: r})
		}
		return out
	}
	for name, c := range map[string]struct {
		r    Report
		want string
	}{
		"nothing":            {Report{Mode: Apply}, "ok"},
		"updated":            {Report{Mode: Apply, Items: it(ResultUpdated, ResultIgnored)}, "ok"},
		"partial":            {Report{Mode: Apply, Items: it(ResultUpdated, ResultFailed)}, "partial"},
		"failed":             {Report{Mode: Apply, Items: it(ResultFailed)}, "failed"},
		"waiting":            {Report{Mode: Apply, Items: it(ResultNeedsAdmin)}, "attention"},
		"found by a check":   {Report{Mode: Notify, Items: it(ResultFound)}, "attention"},
		"error beats all":    {Report{Mode: Apply, Error: ErrCodeCheck, Items: it(ResultUpdated)}, "error"},
		"ignored is no news": {Report{Mode: Notify, Items: it(ResultIgnored)}, "ok"},
	} {
		if got := c.r.Status(); got != c.want {
			t.Errorf("%s: %s", name, got)
		}
	}
}

func TestLockIsExclusiveAndReleasable(t *testing.T) {
	l := Lock{Path: filepath.Join(t.TempDir(), "x", "run.lock")}
	release, err := l.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Acquire(); !errors.Is(err, ErrBusy) {
		t.Fatalf("%v", err)
	}
	release()
	release2, err := l.Acquire()
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	release2()
}

func TestALockLeftByACrashedRunIsTakenOver(t *testing.T) {
	l := Lock{Path: filepath.Join(t.TempDir(), "run.lock")}
	if _, err := l.Acquire(); err != nil {
		t.Fatal(err)
	}
	// a fresh lock holds
	if _, err := l.Acquire(); !errors.Is(err, ErrBusy) {
		t.Fatal("a fresh lock is a running run")
	}
	// more than the longest possible run later, it is a leftover
	l.Now = func() time.Time { return time.Now().Add(RunTimeout + 20*time.Minute) }
	release, err := l.Acquire()
	if err != nil {
		t.Fatalf("stale lock: %v", err)
	}
	release()
}

func TestLanguageIsKeptForTheUnattendedRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "auto")
	fallback := func() string { return "es-MX" }
	if got := Language(dir, fallback); got != "es" {
		t.Errorf("nothing stored: the Windows language, %q", got)
	}
	if got := Language(dir, nil); got != "en" {
		t.Errorf("nothing at all: English, %q", got)
	}
	// Starting the app must not create files for someone who never turned the schedule on.
	if err := SaveLanguage(dir, "en", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("onlyIfKept created a folder")
	}
	if err := SaveLanguage(dir, "en", false); err != nil {
		t.Fatal(err)
	}
	if got := Language(dir, fallback); got != "en" {
		t.Errorf("stored language wins: %q", got)
	}
	if err := SaveLanguage(dir, "es", true); err != nil {
		t.Fatal(err)
	}
	if got := Language(dir, fallback); got != "es" {
		t.Errorf("changing the language later: %q", got)
	}
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{oops"), 0o644)
	if got := Language(dir, func() string { return "en-GB" }); got != "en" {
		t.Errorf("a damaged file falls back: %q", got)
	}
	for in, want := range map[string]string{"es": "es", "ES-es": "es", "es_AR": "es", "en-US": "en", "fr": "en", "": "en", "espanol": "en"} {
		if got := NormalizeLanguage(in); got != want {
			t.Errorf("%q -> %q", in, got)
		}
	}
}

func TestToastTextEnglishAndSpanish(t *testing.T) {
	r := func(mode Mode, items ...Item) Report { return Report{Mode: mode, Items: items} }
	it := func(name, result string) Item { return Item{ID: strings.ToLower(name), Name: name, Result: result} }
	cases := []struct {
		name        string
		rep         Report
		lang        string
		title, body string
		ok          bool
	}{
		{"one found", r(Notify, it("Git", ResultFound)), "en", "1 update available", "Git", true},
		{"one found es", r(Notify, it("Git", ResultFound)), "es", "1 actualización disponible", "Git", true},
		{"three found es", r(Notify, it("Git", ResultFound), it("VLC", ResultFound), it("7-Zip", ResultFound)), "es", "3 actualizaciones disponibles", "Git, VLC, 7-Zip", true},
		{"five found, admin counts", r(Notify, it("A", ResultFound), it("B", ResultFound), it("C", ResultFound), it("D", ResultFound), it("E", ResultNeedsAdmin)), "en", "5 updates available", "A, B, C and 2 more", true},
		{"five found es", r(Notify, it("A", ResultFound), it("B", ResultFound), it("C", ResultFound), it("D", ResultFound), it("E", ResultFound)), "es", "5 actualizaciones disponibles", "A, B, C y 2 más", true},
		{"ignored only", r(Notify, it("A", ResultIgnored)), "en", "", "", false},
		{"nothing", r(Notify), "en", "", "", false},
		{"updated two, one needs you", r(Apply, it("Git", ResultUpdated), it("VLC", ResultUpdated), it("7-Zip", ResultNeedsAdmin)), "en", "Updated 2 apps, 1 needs you", "7-Zip", true},
		{"updated two, one needs you es", r(Apply, it("Git", ResultUpdated), it("VLC", ResultUpdated), it("7-Zip", ResultNeedsAdmin)), "es", "Actualizadas 2 aplicaciones, 1 te necesita", "7-Zip", true},
		{"updated one", r(Apply, it("Git", ResultUpdated)), "en", "Updated 1 app", "Git", true},
		{"updated one es", r(Apply, it("Git", ResultUpdated)), "es", "Actualizada 1 aplicación", "Git", true},
		{"two need you", r(Apply, it("A", ResultNeedsAdmin), it("B", ResultFailed)), "en", "2 apps need you", "A, B", true},
		{"two need you es", r(Apply, it("A", ResultNeedsAdmin), it("B", ResultFailed)), "es", "2 aplicaciones te necesitan", "A, B", true},
		{"updated and failed", r(Apply, it("A", ResultUpdated), it("B", ResultFailed), it("C", ResultFailed)), "en", "Updated 1 app, 2 need you", "B, C", true},
		{"only ignored and skipped", r(Apply, it("A", ResultIgnored), it("B", ResultSkipped)), "en", "", "", false},
		{"unknown language is English", r(Notify, it("Git", ResultFound)), "fr", "1 update available", "Git", true},
	}
	for _, c := range cases {
		got, ok := Compose(c.rep, c.lang)
		if ok != c.ok || got.Title != c.title || got.Body != c.body {
			t.Errorf("%s: %q / %q (%v)", c.name, got.Title, got.Body, ok)
		}
		if ok && got.Launch != LaunchUpdates {
			t.Errorf("%s: a click opens WinForge on Updates", c.name)
		}
	}
	// a restart is told, in the language
	rep := Report{Mode: Apply, Items: []Item{{Name: "Git", Result: ResultUpdated, Reboot: true}}}
	if got, _ := Compose(rep, "es"); got.Body != "Git Reinicia Windows para terminar." {
		t.Errorf("%q", got.Body)
	}
	if got, _ := Compose(rep, "en"); got.Body != "Git Restart Windows to finish." {
		t.Errorf("%q", got.Body)
	}
}

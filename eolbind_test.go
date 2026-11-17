package main

import (
	"errors"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/eol"
	"github.com/Sebas1705/WinForge/internal/system"
)

func withEOLData(t *testing.T, d *eol.Data, err error) {
	t.Helper()
	old := eolLoad
	eolLoad = func() (*eol.Data, error) { return d, err }
	t.Cleanup(func() { eolLoad = old })
}

func eolFixture() *eol.Data {
	return &eol.Data{AsOf: "2025-06-01", Products: []eol.Product{{
		ID: "git", Name: "Git product", Source: "https://example.com", Apps: []string{"git", "vlc"},
		Cycles: []eol.Cycle{{Cycle: "3", EOL: "2030-01-01"}, {Cycle: "2", EOL: "2026-01-01"}},
	}}}
}

func TestEOLReportUsesTheLastScanAndTheClock(t *testing.T) {
	withEOLData(t, eolFixture(), nil)
	a := testApp(t)
	a.now = func() time.Time { return time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC) }
	a.installed = map[string]system.Installed{
		"git":  {ID: "git", Version: "2.1.0"},
		"vlc":  {ID: "vlc", Version: "3.0.1"},
		"7zip": {ID: "7zip", Version: "24.0"}, // not in the table: left out
	}
	a.detect = func() map[string]system.Installed { t.Fatal("must not scan when a scan exists"); return nil }
	r, err := a.EOLReport()
	if err != nil {
		t.Fatal(err)
	}
	if r.AsOf != "2025-06-01" || r.AgeDays != 534 || !r.Stale {
		t.Errorf("age: %+v", r)
	}
	if len(r.Findings) != 2 || r.Findings[0].AppID != "git" || r.Findings[0].Status != eol.Ended || r.Findings[1].AppID != "vlc" || r.Findings[1].Status != eol.OK {
		t.Errorf("%+v", r.Findings)
	}
}

func TestEOLReportScansOnceWhenThereIsNoScan(t *testing.T) {
	withEOLData(t, eolFixture(), nil)
	a := testApp(t)
	a.now = func() time.Time { return time.Date(2025, 6, 2, 9, 0, 0, 0, time.UTC) }
	calls := 0
	a.detect = func() map[string]system.Installed {
		calls++
		return map[string]system.Installed{"git": {ID: "git", Version: "3.2"}}
	}
	r, err := a.EOLReport()
	if err != nil || calls != 1 || len(r.Findings) != 1 || r.Findings[0].Status != eol.OK || r.Stale {
		t.Fatalf("%v calls=%d %+v", err, calls, r)
	}
	if a.installed != nil {
		t.Error("a read-only report must not replace the app's last scan")
	}
}

func TestEOLReportEmptyIsAListAndErrorsPass(t *testing.T) {
	withEOLData(t, eolFixture(), nil)
	a := testApp(t)
	a.installed = map[string]system.Installed{}
	r, err := a.EOLReport()
	if err != nil || r.Findings == nil || len(r.Findings) != 0 {
		t.Fatalf("%v %+v", err, r)
	}
	withEOLData(t, nil, errors.New("bad table"))
	if _, err := a.EOLReport(); err == nil {
		t.Error("a table that cannot be read must be an error")
	}
}

func TestEmbeddedEOLTableLoadsIntoTheApp(t *testing.T) {
	a := testApp(t)
	a.installed = map[string]system.Installed{}
	r, err := a.EOLReport()
	if err != nil || r.AsOf == "" {
		t.Fatalf("%v %+v", err, r)
	}
}

package eol_test

import (
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/eol"
)

// The committed catalogdata/eol.yml must parse, validate and only name catalog apps that exist.
func TestEmbeddedDataIsValid(t *testing.T) {
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	d, err := eol.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range d.Validate(func(id string) bool { return cat.Apps[id] != nil }) {
		t.Error(e)
	}
	if len(d.Products) < 10 {
		t.Errorf("only %d products", len(d.Products))
	}
	asOf, err := eol.ParseDay(d.AsOf)
	if err != nil {
		t.Fatal(err)
	}
	if asOf.After(time.Now().Add(48 * time.Hour)) {
		t.Errorf("asOf %s is in the future", d.AsOf)
	}
}

// Every dated cycle is a real calendar date, and a line the source calls long-term never lacks one.
func TestEmbeddedDataHasNoUndatedLongTermLine(t *testing.T) {
	d, err := eol.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range d.Products {
		for _, c := range p.Cycles {
			if c.LTS && c.EOL == "" {
				t.Errorf("%s %s: long-term line with no end date", p.ID, c.Cycle)
			}
		}
	}
}

// A few lines whose dates were checked against the vendors' own pages when the file was written
// (Node's schedule.json, Microsoft's .NET release index, PostgreSQL's versioning policy).
func TestEmbeddedDataKnownDates(t *testing.T) {
	d, err := eol.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]map[string]string{
		"nodejs":     {"20": "2026-04-30", "22": "2027-04-30", "24": "2028-04-30"},
		"dotnet":     {"6": "2024-11-12", "8": "2026-11-10", "9": "2026-11-10", "10": "2028-11-14"},
		"postgresql": {"15": "2027-11-11", "16": "2028-11-09", "17": "2029-11-08"},
	}
	for _, p := range d.Products {
		for cycle, eolDay := range want[p.ID] {
			found := false
			for _, c := range p.Cycles {
				if c.Cycle == cycle {
					found = true
					if c.EOL != eolDay {
						t.Errorf("%s %s: eol %s, want %s", p.ID, cycle, c.EOL, eolDay)
					}
				}
			}
			if !found {
				t.Errorf("%s %s is missing", p.ID, cycle)
			}
		}
	}
}

// The real data end to end: Python 3.10 installed on the day this was written is out of support.
func TestEmbeddedDataEndToEnd(t *testing.T) {
	d, err := eol.Load(catalogdata.FS)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	r := d.Build([]eol.Installed{
		{AppID: "python-3-10", Version: "3.10.11"},
		{AppID: "python-3-13", Version: "3.13.9"},
		{AppID: "temurin-21", Version: "21.0.9.10"},
		{AppID: "powershell", Version: "7.4.6.0"},
		{AppID: "nodejs-lts", Version: "24.11.0"},
		{AppID: "go", Version: "1.26.0"},
	}, now)
	got := map[string]eol.Finding{}
	for _, f := range r.Findings {
		got[f.AppID] = f
	}
	if f := got["python-3-10"]; f.Status != eol.Ended || len(f.Apps) == 0 {
		t.Errorf("python 3.10: %+v", f)
	}
	if f := got["python-3-13"]; f.Status != eol.OK {
		t.Errorf("python 3.13: %+v", f)
	}
	if f := got["temurin-21"]; f.Status != eol.OK || f.Cycle != "21" {
		t.Errorf("temurin 21: %+v", f)
	}
	if f := got["powershell"]; f.Status != eol.Ended || !f.SelfUpdate || len(f.Replace) == 0 {
		t.Errorf("powershell 7.4 (ended 2026-11-10): %+v", f)
	}
	if f := got["nodejs-lts"]; f.Status != eol.OK {
		t.Errorf("node 24: %+v", f)
	}
	if f := got["go"]; f.Status != eol.OK || f.End != "" {
		t.Errorf("go 1.26: %+v", f)
	}
}

func TestSelfUpdate(t *testing.T) {
	d := &eol.Data{AsOf: "2026-11-17", Products: []eol.Product{{
		ID: "ps", Name: "PS", Apps: []string{"ps"}, Follows: map[string]string{"ps": eol.TrackLatest},
		Cycles: []eol.Cycle{{Cycle: "7.6"}, {Cycle: "7.4", EOL: "2026-01-01"}},
	}, {
		ID: "other", Name: "O", Apps: []string{"o"},
		Cycles: []eol.Cycle{{Cycle: "2"}, {Cycle: "1", EOL: "2026-01-01"}},
	}}}
	r := d.Build([]eol.Installed{{AppID: "ps", Version: "7.4.1"}, {AppID: "o", Version: "1.0"}}, time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC))
	for _, f := range r.Findings {
		if f.AppID == "ps" && (!f.SelfUpdate || len(f.Apps) != 0) {
			t.Errorf("%+v", f)
		}
		if f.AppID == "o" && f.SelfUpdate {
			t.Errorf("%+v", f)
		}
	}
}

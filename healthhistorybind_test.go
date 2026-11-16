package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/battery"
	"github.com/Sebas1705/WinForge/internal/health"
	"github.com/Sebas1705/WinForge/internal/healthhistory"
)

func hhApp(now *time.Time) (*App, *healthhistory.MemStore) {
	m := &healthhistory.MemStore{}
	a := &App{hhist: hhistState{svc: &healthhistory.Service{Store: m}}}
	a.now = func() time.Time { return *now }
	return a, m
}

func scanReport(freeGB float64) *health.Report {
	return &health.Report{
		Disks:   []health.Disk{{Name: "NVMe", Media: "SSD", Health: "Healthy"}},
		Volumes: []health.Volume{{Letter: "C", SizeGB: 500, FreeGB: freeGB}},
	}
}

func TestRecordHealthStoresASnapshotAndTrendsAppear(t *testing.T) {
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	a, _ := hhApp(&now)
	wear := 4
	a.recordHealth(scanReport(200), healthhistory.Extras{Disks: []healthhistory.DiskStat{{Name: "NVMe", WearPct: &wear}}})
	now = now.Add(10 * 24 * time.Hour)
	wear = 6
	a.recordHealth(scanReport(150), healthhistory.Extras{Disks: []healthhistory.DiskStat{{Name: "NVMe", WearPct: &wear}}})
	v, err := a.HealthHistory()
	if err != nil || len(v.Snapshots) != 2 || !v.Enough {
		t.Fatalf("%v %+v", err, v)
	}
	got := map[string]bool{}
	for _, s := range v.Statements {
		got[s.Code] = true
	}
	if !got[healthhistory.StmtFreeFell] || !got[healthhistory.StmtWearRose] {
		t.Fatalf("%+v", v.Statements)
	}
	if err := a.HealthHistoryClear(); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.HealthHistory(); len(v.Snapshots) != 0 || v.Enough {
		t.Fatalf("%+v", v)
	}
}

func TestRecordHealthNeverBreaksAScan(t *testing.T) {
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	a, m := hhApp(&now)
	m.SaveErr = errors.New("disk full")
	a.recordHealth(scanReport(200), healthhistory.Extras{}) // must not panic or fail
	a.recordHealth(nil, healthhistory.Extras{})
	(&App{}).recordHealth(scanReport(1), healthhistory.Extras{}) // no history store at all
	if v, err := (&App{}).HealthHistory(); err != nil || v.Snapshots == nil || v.Statements == nil {
		t.Fatalf("no store gives an empty view: %v %+v", err, v)
	}
	if err := (&App{}).HealthHistoryClear(); err != nil {
		t.Fatal(err)
	}
}

func TestStartHealthExtrasUsesTheFakes(t *testing.T) {
	now := time.Now()
	a, _ := hhApp(&now)
	wear := 3
	a.hhist.disks = func(context.Context) ([]healthhistory.DiskStat, error) {
		return []healthhistory.DiskStat{{Name: "NVMe", WearPct: &wear}}, nil
	}
	a.hhist.battery = func(context.Context) (battery.Report, error) {
		return battery.Report{Present: true, DesignMWh: 50000, FullMWh: 40000}, nil
	}
	x := a.startHealthExtras()()
	if len(x.Disks) != 1 || x.Battery == nil {
		t.Fatalf("%+v", x)
	}
	a.recordHealth(scanReport(100), x)
	v, _ := a.HealthHistory()
	if len(v.Snapshots) != 1 || v.Snapshots[0].Battery == nil || *v.Snapshots[0].Disks[0].WearPct != 3 {
		t.Fatalf("%+v", v.Snapshots)
	}
}

func TestBatteryBinding(t *testing.T) {
	now := time.Now()
	a, _ := hhApp(&now)
	a.hhist.battery = func(context.Context) (battery.Report, error) {
		return battery.Parse([]byte(`<BatteryReport><Batteries></Batteries></BatteryReport>`))
	}
	r, err := a.Battery()
	if err != nil || r.Present || r.Batteries == nil {
		t.Fatalf("a desktop: %v %+v", err, r)
	}
	a.hhist.battery = func(context.Context) (battery.Report, error) { return battery.Report{}, errors.New("powercfg failed") }
	r, err = a.Battery()
	if err == nil || r.Present || r.Batteries == nil || r.History == nil {
		t.Fatalf("a failed read keeps the JSON shape: %v %+v", err, r)
	}
}

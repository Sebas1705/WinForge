package main

import (
	"context"
	"log"

	"github.com/Sebas1705/WinForge/internal/battery"
	"github.com/Sebas1705/WinForge/internal/health"
	"github.com/Sebas1705/WinForge/internal/healthhistory"
)

// hhistState is the PC health history and the readers behind it. svc is nil when the settings
// folder is unavailable; disks and battery are replaced by tests (nil means this PC).
type hhistState struct {
	svc     *healthhistory.Service
	disks   func(context.Context) ([]healthhistory.DiskStat, error)
	battery func(context.Context) (battery.Report, error)
}

func (a *App) openHealthHistory() {
	s, err := healthhistory.DefaultStore()
	if err != nil {
		log.Printf("winforge: health history: %v", err)
		return
	}
	a.hhist.svc = &healthhistory.Service{Store: s}
}

func (a *App) readBattery(ctx context.Context) (battery.Report, error) {
	if a.hhist.battery != nil {
		return a.hhist.battery(ctx)
	}
	return battery.Read(ctx)
}

// startHealthExtras begins reading drive counters and the battery while the scan runs (they take a
// second or two; a scan takes much longer), and returns a function that waits for them.
func (a *App) startHealthExtras() func() healthhistory.Extras {
	if a.hhist.svc == nil {
		return func() healthhistory.Extras { return healthhistory.Extras{} }
	}
	disks := a.hhist.disks
	if disks == nil {
		disks = healthhistory.ReadDisks
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return healthhistory.Collect(ctx, disks, a.readBattery)
}

// recordHealth stores a snapshot of a finished scan. It never fails the scan: a history that
// cannot be written is logged and skipped. The score ignores the Windows Update search, so it does
// not jump depending on whether that search was run.
func (a *App) recordHealth(r *health.Report, extras healthhistory.Extras) {
	if a.hhist.svc == nil || r == nil {
		return
	}
	now := a.clock()
	snap := healthhistory.FromScan(r, health.Analyze(r, nil, now), extras, now)
	if err := a.hhist.svc.Record(snap); err != nil {
		log.Printf("winforge: %v", err)
	}
}

// HealthHistory returns the stored scan snapshots (oldest first) and the plain statements about
// how the PC changed. It reads a small file and nothing else.
func (a *App) HealthHistory() (healthhistory.View, error) {
	if a.hhist.svc == nil {
		return healthhistory.BuildView(healthhistory.File{}), nil
	}
	return a.hhist.svc.View()
}

// HealthHistoryClear deletes the whole history. The interface asks first.
func (a *App) HealthHistoryClear() error {
	if a.hhist.svc == nil {
		return nil
	}
	return a.hhist.svc.Clear()
}

// Battery reads the laptop battery's condition. A PC without a battery answers present=false.
// It only reads: powercfg writes its report to a temporary file that is deleted at once.
func (a *App) Battery() (battery.Report, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	r, err := a.readBattery(ctx)
	if err != nil {
		return battery.Report{Batteries: []battery.Battery{}, History: []battery.Point{}, Verdict: battery.VerdictUnknown}, err
	}
	return r, nil
}

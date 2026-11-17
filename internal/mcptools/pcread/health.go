package pcread

import (
	"context"
	"time"

	"github.com/Sebas1705/WinForge/internal/health"
	"github.com/Sebas1705/WinForge/internal/healthhistory"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

type finding struct {
	Key      string            `json:"key"`
	Group    string            `json:"group"`
	Severity string            `json:"severity"`
	Params   map[string]string `json:"params,omitempty"`
}

type machine struct {
	Manufacturer string  `json:"manufacturer"`
	Model        string  `json:"model"`
	OS           string  `json:"os"`
	CPU          string  `json:"cpu"`
	RAMGB        float64 `json:"ramGB"`
}

type healthScanOut struct {
	Score                 map[string]int `json:"score"`
	Findings              []finding      `json:"findings"`
	Truncated             bool           `json:"truncated"`
	Machine               machine        `json:"machine"`
	WindowsUpdateSearched bool           `json:"windowsUpdateSearched"`
	Errors                []string       `json:"sectionsNotRead"`
}

func (g *group) healthTools() []mcpkit.Tool {
	scan := g.read("winforge_health_scan", "PC health scan",
		"Runs WinForge's PC health scan (a read-only PowerShell/CIM look at firmware, drivers, storage, security and Windows) and returns the score {ok,total}, the findings [{key,group,severity,params}] and a machine summary (manufacturer, model, Windows edition, CPU name, RAM in GB). "+
			"The result describes this PC's hardware and settings and is seen by the model; text coming from the PC (device and driver names) has the user name and user folder removed. No serial numbers and no hardware ids beyond what the findings already carry. "+
			"Set withWindowsUpdate to also search Windows Update for pending drivers and updates (slower; it never downloads or installs anything). This tool NEVER changes anything on the PC and takes a while: it reports progress.",
		mcpkit.Schema(`{"type":"object","properties":{"withWindowsUpdate":{"type":"boolean","default":false,"description":"Also search Windows Update for pending drivers, firmware and updates (read-only, slower)."},`+limitProp+`},"additionalProperties":false}`),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			var a struct {
				WithWindowsUpdate bool `json:"withWindowsUpdate"`
				Limit             *int `json:"limit"`
			}
			if err := mcpkit.Decode(x.c.Args, &a); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			limit, err := limitArg(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			steps := 0
			x.progress(0, 0, "starting the health scan")
			r, err := g.d.HealthCollect(ctx, func(s string) {
				steps++
				x.progress(float64(steps), 0, s)
			})
			if err != nil {
				return nil, err
			}
			if r == nil {
				return mcpkit.Fail("the health scan returned nothing"), nil
			}
			var upd *health.UpdateScan
			if a.WithWindowsUpdate {
				x.progress(float64(steps), 0, "searching Windows Update")
				if upd, err = g.d.HealthUpdates(ctx); err != nil {
					return nil, err
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			fs := health.Analyze(r, upd, x.now())
			ok, total := health.Tally(fs)
			out := healthScanOut{
				Score:                 map[string]int{"ok": ok, "total": total},
				WindowsUpdateSearched: upd != nil,
				Machine: machine{
					Manufacturer: x.redact(r.Machine.Manufacturer), Model: x.redact(r.Machine.Model),
					OS: x.redact(r.OS.Caption), CPU: x.redact(r.CPU.Name), RAMGB: r.Memory.TotalGB,
				},
				Errors: []string{},
			}
			for _, e := range r.Errors {
				out.Errors = append(out.Errors, x.redact(short(e, maxText)))
			}
			list := make([]finding, 0, len(fs))
			for _, f := range fs {
				p := map[string]string{}
				for k, v := range f.Params {
					if v != "" {
						p[k] = x.redact(v)
					}
				}
				if len(p) == 0 {
					p = nil
				}
				list = append(list, finding{Key: f.Key, Group: string(f.Group), Severity: string(f.Severity), Params: p})
			}
			out.Findings, out.Truncated = capped(list, limit)
			return mcpkit.JSON(out), nil
		})

	history := g.read("winforge_health_history", "PC health history",
		"Returns the summary of WinForge's stored PC health snapshots (how many, first and last date, the latest score, problems per area, free space per drive, drive health and wear, battery capacity), a compact score timeline and the trend statements (a code plus parameters, for example free space dropped). "+
			"It reads WinForge's own small history file and nothing else; it does not run a scan. It describes this PC's state over time and is seen by the model; names are redacted. It NEVER changes or clears the history.",
		mcpkit.Schema(`{"type":"object","properties":{`+limitProp+`},"additionalProperties":false}`),
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			var a struct {
				Limit *int `json:"limit"`
			}
			if err := mcpkit.Decode(x.c.Args, &a); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			limit, err := limitArg(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			v, err := runCtx(ctx, func() (hv, error) { return g.d.HealthHistory(ctx) })
			if err != nil {
				return nil, err
			}
			return mcpkit.JSON(buildHistory(v, limit, x.redact)), nil
		})

	bat := g.read("winforge_battery", "Battery health",
		"Returns the laptop battery report: whether a battery is present, health as a percent of the design capacity, charge cycles when the battery reports them, a verdict (good, worn, replace or unknown), capacities in mWh and the recent weekly readings. "+
			"It reads Windows' own battery report (powercfg writes it to a temporary file that is deleted at once); no serial number or battery id is read. It describes this PC's hardware and is seen by the model. It NEVER changes anything.",
		mcpkit.NoArgs,
		func(ctx context.Context, x *tc) (*mcpkit.Result, error) {
			if err := mcpkit.Decode(x.c.Args, &struct{}{}); err != nil {
				return mcpkit.Fail("%s", err), nil
			}
			r, err := g.d.Battery(ctx)
			if err != nil {
				return nil, err
			}
			type bat struct {
				Chemistry string  `json:"chemistry,omitempty"`
				DesignMWh int64   `json:"designMWh"`
				FullMWh   int64   `json:"fullMWh"`
				HealthPct float64 `json:"healthPct"`
				Cycles    *int    `json:"cycles"`
				Verdict   string  `json:"verdict"`
			}
			type point struct {
				Date      string  `json:"date"`
				FullMWh   int64   `json:"fullMWh"`
				HealthPct float64 `json:"healthPct"`
				Cycles    *int    `json:"cycles"`
			}
			out := struct {
				Present    bool               `json:"present"`
				HealthPct  float64            `json:"healthPct"`
				Cycles     *int               `json:"cycles"`
				Verdict    string             `json:"verdict"`
				DesignMWh  int64              `json:"designMWh"`
				FullMWh    int64              `json:"fullMWh"`
				Batteries  []bat              `json:"batteries"`
				History    []point            `json:"history"`
				Thresholds map[string]float64 `json:"thresholds"`
			}{
				Present: r.Present, HealthPct: r.HealthPct, Cycles: r.Cycles, Verdict: r.Verdict,
				DesignMWh: r.DesignMWh, FullMWh: r.FullMWh, Batteries: []bat{}, History: []point{},
				Thresholds: map[string]float64{"goodMinPct": r.GoodMinPct, "replaceBelowPct": r.ReplaceBelowPct},
			}
			for _, b := range r.Batteries {
				out.Batteries = append(out.Batteries, bat{x.redact(b.Chemistry), b.DesignMWh, b.FullMWh, b.HealthPct, b.Cycles, b.Verdict})
			}
			for _, p := range r.History {
				out.History = append(out.History, point{p.Date, p.FullMWh, p.HealthPct, p.Cycles})
			}
			return mcpkit.JSON(out), nil
		})
	return []mcpkit.Tool{scan, history, bat}
}

type timePoint struct {
	At    time.Time `json:"at"`
	Score int       `json:"score"`
	Total int       `json:"total"`
}

func buildHistory(v hv, limit int, redact func(string) string) map[string]any {
	n := len(v.Snapshots)
	out := map[string]any{"enough": v.Enough}
	summary := map[string]any{"count": n}
	var latest any
	if n > 0 {
		first, last := v.Snapshots[0], v.Snapshots[n-1]
		summary["first"], summary["last"] = first.At, last.At
		type vol struct {
			Letter     string `json:"letter"`
			FreeBytes  int64  `json:"freeBytes"`
			TotalBytes int64  `json:"totalBytes"`
		}
		type disk struct {
			Name         string `json:"name"`
			Media        string `json:"media,omitempty"`
			Health       string `json:"health,omitempty"`
			WearPct      *int   `json:"wearPct"`
			TempC        *int   `json:"tempC"`
			PowerOnHours *int   `json:"powerOnHours"`
		}
		vols, disks := []vol{}, []disk{}
		for _, x := range last.Volumes {
			vols = append(vols, vol{redact(x.Letter), x.FreeBytes, x.TotalBytes})
		}
		for _, d := range last.Disks {
			disks = append(disks, disk{redact(d.Name), redact(d.Media), redact(d.Health), d.WearPct, d.TempC, d.PowerOnHours})
		}
		areas := map[string]int{}
		for k, c := range last.Areas {
			areas[k] = c
		}
		latest = map[string]any{"at": last.At, "score": last.Score, "total": last.Total, "problemsPerArea": areas, "volumes": vols, "disks": disks, "battery": last.Battery}
	}
	out["snapshots"], out["latest"] = summary, latest
	// The timeline keeps the newest points when it has to cut.
	tl := make([]timePoint, 0, n)
	for _, s := range v.Snapshots {
		tl = append(tl, timePoint{s.At, s.Score, s.Total})
	}
	truncated := false
	if len(tl) > limit {
		tl, truncated = tl[len(tl)-limit:], true
	}
	out["timeline"] = tl
	type st struct {
		Code   string            `json:"code"`
		Params map[string]string `json:"params"`
	}
	sts := make([]st, 0, len(v.Statements))
	for _, s := range v.Statements {
		p := map[string]string{}
		for k, val := range s.Params {
			p[k] = redact(val)
		}
		sts = append(sts, st{s.Code, p})
	}
	var cut bool
	sts, cut = capped(sts, limit)
	out["statements"] = sts
	out["truncated"] = truncated || cut
	return out
}

// hv is the stored history as the page sees it.
type hv = healthhistory.View

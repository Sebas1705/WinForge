package main

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/cliout"
	"github.com/Sebas1705/WinForge/internal/exitcode"
	"github.com/Sebas1705/WinForge/internal/health"
)

// healthOut is what a health check produced, before it is printed.
type healthOut struct {
	Machine  machineJSON
	Findings []health.Finding
}

// machineJSON is the machine member of the `health` document: manufacturer, model, os (the
// Windows caption), cpu and ramGB.
type machineJSON struct {
	Manufacturer string  `json:"manufacturer"`
	Model        string  `json:"model"`
	OS           string  `json:"os"`
	CPU          string  `json:"cpu"`
	RAMGB        float64 `json:"ramGB"`
}

func collectHealth(ctx context.Context, withUpdates bool) (*healthOut, error) {
	r, err := health.Collect(ctx)
	if err != nil {
		return nil, err
	}
	var scan *health.UpdateScan
	if withUpdates {
		if scan, err = health.ScanUpdates(ctx); err != nil {
			return nil, err
		}
	}
	return &healthOut{
		Machine:  machineJSON{r.Machine.Manufacturer, r.Machine.Model, r.OS.Caption, r.CPU.Name, r.Memory.TotalGB},
		Findings: health.Analyze(r, scan, time.Now()),
	}, nil
}

// healthDoc is the --json document of `health`: the envelope and
//
//	machine     {manufacturer, model, os, cpu, ramGB}
//	findings[]  {key, group, severity, params?, links?, winget?}: keys and parameters, not sentences
//	okCount, total   how many of the checks are fine
type healthDoc struct {
	cliout.Envelope
	Machine  machineJSON      `json:"machine"`
	Findings []health.Finding `json:"findings"`
	OKCount  int              `json:"okCount"`
	Total    int              `json:"total"`
}

// healthCmd prints the PC health checks. Findings are shown as key and parameters: the desktop app
// words them in the user's language, and this command is for diagnostics and scripting.
func (c *cmdCtx) healthCmd(args []string) int {
	if len(args) > 1 || (len(args) == 1 && args[0] != "updates") {
		return c.usageErr("")
	}
	h, err := c.d.health(c.ctx, len(args) == 1)
	if err != nil {
		if c.timedOut() || errors.Is(err, context.DeadlineExceeded) {
			return c.fail(exitcode.Timeout, "timeout", "the time limit was reached during the health check")
		}
		return c.fail(exitcode.Failures, "", err.Error())
	}
	c.printf("%s %s | %s | %s (%.0f GB RAM)\n", h.Machine.Manufacturer, h.Machine.Model, h.Machine.OS, h.Machine.CPU, h.Machine.RAMGB)
	for _, f := range h.Findings {
		keys := make([]string, 0, len(f.Params))
		for k := range f.Params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var ps []string
		for _, k := range keys {
			if f.Params[k] != "" {
				ps = append(ps, k+"="+f.Params[k])
			}
		}
		c.printf("[%-7s] %-16s %s\n", f.Severity, f.Key, strings.Join(ps, "  "))
	}
	ok, total := health.Tally(h.Findings)
	c.printf("\n%d of %d checks fine\n", ok, total)
	doc := &healthDoc{Machine: h.Machine, Findings: h.Findings, OKCount: ok, Total: total}
	if doc.Findings == nil {
		doc.Findings = []health.Finding{}
	}
	return c.done(exitcode.OK, "", "", &doc.Envelope, doc)
}

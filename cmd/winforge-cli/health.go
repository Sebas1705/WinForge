package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/health"
)

// runHealth prints the PC health checks. Findings are shown as key and
// parameters: the desktop app words them in the user's language, and this
// command is for diagnostics and scripting.
func runHealth(ctx context.Context, withUpdates bool) {
	r, err := health.Collect(ctx)
	if err != nil {
		fatal(err)
	}
	var scan *health.UpdateScan
	if withUpdates {
		if scan, err = health.ScanUpdates(ctx); err != nil {
			fatal(err)
		}
	}
	findings := health.Analyze(r, scan, time.Now())
	fmt.Fprintf(os.Stdout, "%s %s | %s | %s (%.0f GB RAM)\n", r.Machine.Manufacturer, r.Machine.Model, r.OS.Caption, r.CPU.Name, r.Memory.TotalGB)
	for _, f := range findings {
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
		fmt.Printf("[%-7s] %-16s %s\n", f.Severity, f.Key, strings.Join(ps, "  "))
	}
	ok, total := health.Tally(findings)
	fmt.Printf("\n%d of %d checks fine\n", ok, total)
}

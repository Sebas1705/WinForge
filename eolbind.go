package main

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/eol"
	"github.com/Sebas1705/WinForge/internal/system"
)

// The end-of-support table is embedded in the program (catalogdata/eol.yml) and only read. It is
// parsed once. Tests replace eolLoad.
var (
	eolOnce sync.Once
	eolData *eol.Data
	eolErr  error
)

var eolLoad = func() (*eol.Data, error) {
	eolOnce.Do(func() { eolData, eolErr = eol.Load(catalogdata.FS) })
	return eolData, eolErr
}

// EOLReport says, for every installed catalog app whose product has a release line that ends,
// whether that line is past its end of support, ends within 180 days, is fine, or could not be
// matched. It reads the last scan of the PC (scanning once if there is none) and the embedded
// table; nothing is downloaded and nothing about the PC is sent anywhere. The report carries the
// date of the table (asOf, ageDays, stale) so the page can say how old the answer may be.
func (a *App) EOLReport() (eol.Report, error) {
	d, err := eolLoad()
	if err != nil {
		return eol.Report{}, err
	}
	return d.Build(a.eolInstalled(), a.clock()), nil
}

// eolInstalled lists the installed catalog apps and their versions from the last scan. With no
// scan yet it takes one, the way the history does, without keeping the result.
func (a *App) eolInstalled() []eol.Installed {
	a.mu.Lock()
	found := a.installed
	a.mu.Unlock()
	if found == nil {
		if a.detect != nil {
			found = a.detect()
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			inv, _ := system.Snapshot(ctx) // without winget the registry and PATH still say something
			found = system.Detect(a.cat, inv)
		}
	}
	out := make([]eol.Installed, 0, len(found))
	for id, i := range found {
		out = append(out, eol.Installed{AppID: id, Version: i.Version})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AppID < out[j].AppID })
	return out
}

// Command winforge-verify checks every catalog app against the winget-pkgs
// repository: the package exists, its publisher is the one the catalog
// expects, and every installer is downloaded over HTTPS. With -lock it also
// writes catalog.lock.json, a reviewable record of what was verified.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/wingetpkgs"
)

// LockEntry is what the lock file records per app.
type LockEntry struct {
	Version   string   `json:"version"`
	Publisher string   `json:"publisher"`
	Hosts     []string `json:"installerHosts"`
	SHA256    []string `json:"installerSha256"`
}

func verify(ctx context.Context, c *wingetpkgs.Client, a *catalog.App) (*LockEntry, error) {
	p, err := c.Package(ctx, a.Winget)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(p.Publisher), strings.TrimSpace(a.Publisher)) {
		return nil, fmt.Errorf("publisher is %q, catalog expects %q", p.Publisher, a.Publisher)
	}
	hosts, hashes := map[string]bool{}, map[string]bool{}
	for _, i := range p.Installers {
		u, err := url.Parse(i.URL)
		if err != nil || u.Scheme != "https" {
			return nil, fmt.Errorf("installer %q is not an https URL", i.URL)
		}
		hosts[u.Host] = true
		if i.SHA256 != "" {
			hashes[strings.ToLower(i.SHA256)] = true
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("manifest %s lists no installers", p.Version)
	}
	return &LockEntry{Version: p.Version, Publisher: p.Publisher, Hosts: keys(hosts), SHA256: keys(hashes)}, nil
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// outcome is how one app's verification ended.
type outcome int

const (
	verified outcome = iota
	// unverified: GitHub kept rate limiting the lookups. Nothing is known
	// about the app, so it is reported but does not fail the run.
	unverified
	failed
)

func classify(err error) outcome {
	switch {
	case err == nil:
		return verified
	case errors.Is(err, wingetpkgs.ErrRateLimited):
		return unverified
	default:
		return failed
	}
}

func readLock(path string) map[string]*LockEntry {
	entries := map[string]*LockEntry{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &entries)
	}
	return entries
}

// mergeLock builds the new lock: verified apps get fresh data, unverified
// apps keep what the previous lock recorded for them.
func mergeLock(prev map[string]*LockEntry, ids []string, entries []*LockEntry, outcomes []outcome) map[string]*LockEntry {
	out := map[string]*LockEntry{}
	for i, id := range ids {
		switch outcomes[i] {
		case verified:
			out[id] = entries[i]
		case unverified:
			if e, ok := prev[id]; ok {
				out[id] = e
			}
		}
	}
	return out
}

// summary appends a line to the GitHub Actions job summary, if there is one.
func summary(line string) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

func main() { os.Exit(run()) }

func run() int {
	lock := flag.String("lock", "", "write the lock file to this path")
	only := flag.String("only", "", "verify only this app id")
	source := flag.String("source", "git", "where manifests come from: git (one shallow sparse clone) or http (GitHub API + raw files)")
	mirrorDir := flag.String("mirror", "", "with -source git, keep the winget-pkgs clone in this folder and reuse it (default: a temporary folder)")
	workers := flag.Int("workers", 5, "concurrent lookups")
	flag.Parse()

	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ids := make([]string, 0, len(cat.Apps))
	for id := range cat.Apps {
		if *only == "" || *only == id {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	ctx := context.Background()
	start := time.Now()
	c := wingetpkgs.New()
	if *source == "git" {
		dir := *mirrorDir
		if dir == "" {
			tmp, err := os.MkdirTemp("", "winget-pkgs-")
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
			defer os.RemoveAll(tmp)
			dir = filepath.Join(tmp, "winget-pkgs")
		}
		m, err := wingetpkgs.OpenMirror(ctx, dir, "", c.Token)
		if err == nil {
			wingetIDs := make([]string, len(ids))
			for i, id := range ids {
				wingetIDs[i] = cat.Apps[id].Winget
			}
			err = m.Prefetch(ctx, wingetIDs)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: winget-pkgs clone failed, falling back to HTTP: %v\n", err)
		} else {
			c.Mirror = m
			fmt.Printf("winget-pkgs clone ready in %s\n", time.Since(start).Round(time.Second))
		}
	} else if *source != "http" {
		fmt.Fprintln(os.Stderr, "-source must be git or http")
		return 2
	}
	if c.Mirror == nil && c.Token == "" {
		fmt.Fprintln(os.Stderr, "warning: no GITHUB_TOKEN or gh login; the GitHub API allows only 60 requests per hour")
	}

	entries := make([]*LockEntry, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(*workers, 1))
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			entries[i], errs[i] = verify(ctx, c, cat.Apps[id])
		}()
	}
	wg.Wait()

	actions := os.Getenv("GITHUB_ACTIONS") == "true"
	outcomes := make([]outcome, len(ids))
	counts := [3]int{}
	for i, err := range errs {
		outcomes[i] = classify(err)
		counts[outcomes[i]]++
		switch outcomes[i] {
		case unverified:
			fmt.Printf("SKIP %-32s %v\n", ids[i], err)
			if actions {
				fmt.Printf("::warning::%s unverified (rate limited): %v\n", ids[i], err)
			}
		case failed:
			fmt.Printf("FAIL %-32s %v\n", ids[i], err)
		}
	}
	line := fmt.Sprintf("%d verified, %d unverified (rate limited), %d failed", counts[verified], counts[unverified], counts[failed])
	fmt.Printf("\n%s in %s\n", line, time.Since(start).Round(time.Second))
	summary("**Catalog verification:** " + line)
	if counts[unverified] > 0 && actions {
		fmt.Printf("::warning::%d catalog apps could not be verified because GitHub rate limited the lookups; their previous lock data is kept\n", counts[unverified])
	}

	if *lock != "" && counts[failed] == 0 {
		merged := mergeLock(readLock(*lock), ids, entries, outcomes)
		if *only != "" {
			// A single-app run updates that entry and keeps the rest.
			all := readLock(*lock)
			for id, e := range merged {
				all[id] = e
			}
			merged = all
		}
		b, _ := json.MarshalIndent(merged, "", "  ")
		if err := os.WriteFile(*lock, append(b, '\n'), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		fmt.Println("wrote", *lock)
	}
	if counts[failed] > 0 {
		return 1
	}
	return 0
}

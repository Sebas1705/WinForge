// Command winforge-verify checks every catalog app against the winget-pkgs
// repository: the package exists, its publisher is the one the catalog
// expects, and every installer is downloaded over HTTPS. With -lock it also
// writes catalog.lock.json, a reviewable record of what was verified.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"

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

func main() {
	lock := flag.String("lock", "", "write the lock file to this path")
	only := flag.String("only", "", "verify only this app id")
	flag.Parse()

	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	c := wingetpkgs.New()
	if c.Token == "" {
		fmt.Fprintln(os.Stderr, "warning: no GITHUB_TOKEN or gh login; the GitHub API allows only 60 requests per hour")
	}

	ids := make([]string, 0, len(cat.Apps))
	for id := range cat.Apps {
		if *only == "" || *only == id {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	type result struct {
		entry *LockEntry
		err   error
	}
	results := make([]result, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			e, err := verify(context.Background(), c, cat.Apps[id])
			results[i] = result{e, err}
		}()
	}
	wg.Wait()

	failed := 0
	entries := map[string]*LockEntry{}
	for i, r := range results {
		if r.err != nil {
			failed++
			fmt.Printf("FAIL %-32s %v\n", ids[i], r.err)
			continue
		}
		entries[ids[i]] = r.entry
	}
	fmt.Printf("\n%d verified, %d failed\n", len(entries), failed)
	if *lock != "" && failed == 0 {
		b, _ := json.MarshalIndent(entries, "", "  ")
		if err := os.WriteFile(*lock, append(b, '\n'), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println("wrote", *lock)
	}
	if failed > 0 {
		os.Exit(1)
	}
}

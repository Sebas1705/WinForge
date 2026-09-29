// Command winforge-import grows the catalog from winget-pkgs manifests.
//
//	winforge-import -list tools/catalog-lists/x.txt -out catalogdata/apps/x.yml
//	winforge-import -detect
//
// A list file has one entry per line: category|Winget.Id[|catalog-id].
// Everything factual (name, publisher, homepage, license, description) comes
// from the manifest, so entries are as reliable as winget-pkgs itself; ids that
// do not exist, have no https homepage or no https installer are reported and
// skipped. -detect regenerates catalogdata/detect/detect.yml: registry rules
// built from each package's Apps & features name, which is how apps installed
// by hand (not through winget) are recognised.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/wingetpkgs"
)

type item struct{ category, winget, id, name, home string }

func main() {
	list := flag.String("list", "", "list file: category|Winget.Id[|catalog-id] per line")
	out := flag.String("out", "", "YAML file to write new apps to")
	detect := flag.Bool("detect", false, "regenerate catalogdata/detect/detect.yml")
	deny := flag.String("deny", "tools/catalog-lists/deny.txt", "file of winget ids never to import")
	flag.Parse()

	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		fatal(err)
	}
	c := wingetpkgs.New()
	c.CacheDir = ".winget-cache" // git-ignored; manifests are re-read after 12h
	switch {
	case *detect:
		fatalIf(genDetect(c, cat, "catalogdata/detect/detect.yml"))
	case *list != "" && *out != "":
		fatalIf(genApps(c, cat, *list, *out, *deny))
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func readList(path string) ([]item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var items []item
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 || len(parts) > 5 {
			return nil, fmt.Errorf("%s:%d: want category|Winget.Id[|id[|Name[|https-homepage]]]", path, n)
		}
		it := item{category: strings.TrimSpace(parts[0]), winget: strings.TrimSpace(parts[1])}
		if len(parts) >= 3 {
			it.id = strings.TrimSpace(parts[2])
		}
		if len(parts) >= 4 {
			it.name = strings.TrimSpace(parts[3])
		}
		if len(parts) == 5 {
			it.home = strings.TrimSpace(parts[4])
			if !strings.HasPrefix(it.home, "https://") {
				return nil, fmt.Errorf("%s:%d: homepage must be https", path, n)
			}
		}
		items = append(items, it)
	}
	return items, sc.Err()
}

func fetchAll[T any](items []T, key func(T) string, c *wingetpkgs.Client) ([]*wingetpkgs.Package, []error) {
	pkgs := make([]*wingetpkgs.Package, len(items))
	errs := make([]error, len(items))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	for i, it := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pkgs[i], errs[i] = c.Package(context.Background(), key(it))
		}()
	}
	wg.Wait()
	return pkgs, errs
}

var (
	channelID   = regexp.MustCompile(`(?i)\.(beta|nightly|canary|insiders?|preview|dev|rc\d*|alpha|unstable|edge)$`)
	channelName = regexp.MustCompile(`(?i)\b(beta|nightly|canary|insiders?|preview|pre-release|unstable)\b`)
)

var noiseSuffix = regexp.MustCompile(`(?i)\s+(stable|setup)$|\s*\((64-bit|32-bit)\)$`)

// cleanName drops marketing subtitles and packaging noise:
// "Fork - a fast git client" -> "Fork", "Opera Stable" -> "Opera". Sysinternals
// tools get their family name so "Handle" and "Strings" are findable.
func cleanName(id, n string) string {
	if i := strings.Index(n, " - "); i > 0 {
		n = n[:i]
	}
	n = strings.TrimSpace(noiseSuffix.ReplaceAllString(strings.TrimSpace(n), ""))
	if strings.HasPrefix(id, "Microsoft.Sysinternals.") && !strings.Contains(strings.ToLower(n), "sysinternals") {
		n = "Sysinternals " + n
	}
	return n
}

// readSet reads one id per line ('#' starts a comment). Ids are folded to
// lower case for the deny list, where a human typed them; the not-found cache
// keeps exact case because manifest paths in winget-pkgs are case-sensitive.
func readSet(path string, fold bool) map[string]bool {
	out := map[string]bool{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, l := range strings.Split(string(b), "\n") {
		if i := strings.Index(l, "#"); i >= 0 {
			l = l[:i]
		}
		if l = strings.TrimSpace(l); l != "" {
			if fold {
				l = strings.ToLower(l)
			}
			out[l] = true
		}
	}
	return out
}

func genApps(c *wingetpkgs.Client, cat *catalog.Catalog, listPath, outPath, denyPath string) error {
	items, err := readList(listPath)
	if err != nil {
		return err
	}
	denied := readSet(denyPath, true)
	// Ids that did not exist on an earlier run are not looked up again: each
	// lookup costs a GitHub API request and the hourly budget is 5000.
	const notFoundPath = "tools/catalog-lists/.notfound"
	known404 := readSet(notFoundPath, false)
	haveWinget := map[string]bool{}
	haveID := map[string]bool{}
	for id, a := range cat.Apps {
		haveWinget[strings.ToLower(a.Winget)] = true
		haveID[id] = true
	}
	var todo []item
	seen := map[string]bool{}
	for _, it := range items {
		k := strings.ToLower(it.winget)
		if haveWinget[k] || seen[k] {
			continue
		}
		if known404[it.winget] {
			continue
		}
		if denied[k] {
			fmt.Printf("deny %s\n", it.winget)
			continue
		}
		if channelID.MatchString(it.winget) {
			fmt.Printf("skip %-40s pre-release channel\n", it.winget)
			continue
		}
		seen[k] = true
		todo = append(todo, it)
	}
	pkgs, errs := fetchAll(todo, func(i item) string { return i.winget }, c)

	var b strings.Builder
	var newly404 []string
	added, skipped := 0, 0
	for i, it := range todo {
		p := pkgs[i]
		if errs[i] != nil {
			skipped++
			fmt.Printf("skip %-40s %v\n", it.winget, errs[i])
			if strings.Contains(errs[i].Error(), "not found in winget-pkgs") {
				newly404 = append(newly404, it.winget)
			}
			continue
		}
		p.Name = cleanName(p.ID, p.Name)
		if it.name != "" {
			p.Name = it.name
		}
		if channelName.MatchString(p.Name) {
			skipped++
			fmt.Printf("skip %-40s pre-release channel (%s)\n", it.winget, p.Name)
			continue
		}
		home := firstHTTPS(p.PackageURL, p.PublisherURL, it.home)
		if why := incomplete(p, home); why != "" {
			skipped++
			fmt.Printf("skip %-40s %s\n", it.winget, why)
			continue
		}
		id := it.id
		if id == "" {
			id = wingetpkgs.Slug(p.Name)
		}
		if id == "" || haveID[id] {
			id = wingetpkgs.Slug(p.Publisher + " " + p.Name)
		}
		if id == "" || haveID[id] {
			skipped++
			fmt.Printf("skip %-40s id %q already taken\n", it.winget, id)
			continue
		}
		haveID[id] = true
		desc := wingetpkgs.OneLine(p, 160)
		if desc == "" {
			desc = p.Name + " by " + p.Publisher + "."
		}
		fmt.Fprintf(&b, "- id: %s\n  name: %s\n  category: %s\n  description: %s\n  homepage: %s\n",
			id, q(p.Name), it.category, q(desc), home)
		if p.License != "" {
			fmt.Fprintf(&b, "  license: %s\n", q(p.License))
		}
		fmt.Fprintf(&b, "  winget: %s\n  publisher: %s\n\n", p.ID, q(p.Publisher))
		added++
	}
	if added > 0 {
		f, err := os.OpenFile(outPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString(b.String()); err != nil {
			return err
		}
	}
	if len(newly404) > 0 {
		if f, err := os.OpenFile(notFoundPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			f.WriteString(strings.Join(newly404, "\n") + "\n")
			f.Close()
		}
	}
	fmt.Printf("\n%d added to %s, %d skipped, %d already in the catalog\n", added, outPath, skipped, len(items)-len(todo))
	return nil
}

func genDetect(c *wingetpkgs.Client, cat *catalog.Catalog, outPath string) error {
	ids := make([]string, 0, len(cat.Apps))
	for id := range cat.Apps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	pkgs, errs := fetchAll(ids, func(id string) string { return cat.Apps[id].Winget }, c)
	var b strings.Builder
	n := 0
	for i, id := range ids {
		if errs[i] != nil {
			fmt.Printf("warn %-32s %v\n", id, errs[i])
			continue
		}
		pats := wingetpkgs.RegistryPatterns(pkgs[i])
		if len(pats) == 0 {
			continue
		}
		fmt.Fprintf(&b, "- id: %s\n  registry:\n", id)
		for _, p := range pats {
			fmt.Fprintf(&b, "    - %s\n", q(p))
		}
		n++
	}
	body := b.String()
	if body == "" {
		body = "[]\n"
	}
	header := "# Generated by `winforge-import -detect` from winget manifests. Do not edit by hand.\n"
	if err := os.WriteFile(outPath, []byte(header+body), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote detection rules for %d of %d apps to %s\n", n, len(ids), outPath)
	return nil
}

func firstHTTPS(urls ...string) string {
	for _, u := range urls {
		if strings.HasPrefix(u, "https://") {
			return u
		}
	}
	return ""
}

// incomplete says why a manifest cannot become a catalog entry, or "".
func incomplete(p *wingetpkgs.Package, home string) string {
	switch {
	case p.Name == "":
		return "manifest has no name"
	case p.Publisher == "":
		return "manifest has no publisher"
	case home == "":
		return "manifest has no https homepage (add one as the 5th list field)"
	case len(p.Installers) == 0:
		return "manifest lists no installers"
	}
	for _, i := range p.Installers {
		if u, err := url.Parse(i.URL); err != nil || u.Scheme != "https" {
			return "installer is not downloaded over https: " + i.URL
		}
	}
	return ""
}

// q quotes a string for YAML. JSON strings are valid YAML double-quoted
// scalars, and json.Marshal handles every escape.
func q(s string) string {
	b, _ := json.Marshal(s)
	return strings.ReplaceAll(string(b), `&`, "&")
}

func fatalIf(err error) {
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

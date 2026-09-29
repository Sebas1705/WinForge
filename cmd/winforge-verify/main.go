// Command winforge-verify checks every catalog app against the winget-pkgs
// repository: the package exists, its publisher is the one the catalog
// expects, and every installer is downloaded over HTTPS. With -lock it also
// writes catalog.lock.json, a reviewable record of what was verified.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
)

type installerManifest struct {
	Installers []struct {
		Architecture    string `yaml:"Architecture"`
		InstallerURL    string `yaml:"InstallerUrl"`
		InstallerSha256 string `yaml:"InstallerSha256"`
	} `yaml:"Installers"`
	InstallerURL    string `yaml:"InstallerUrl"`
	InstallerSha256 string `yaml:"InstallerSha256"`
}

type localeManifest struct {
	Publisher   string `yaml:"Publisher"`
	PackageName string `yaml:"PackageName"`
}

// LockEntry is what the lock file records per app.
type LockEntry struct {
	Version   string   `json:"version"`
	Publisher string   `json:"publisher"`
	Hosts     []string `json:"installerHosts"`
	SHA256    []string `json:"installerSha256"`
}

type client struct {
	http  *http.Client
	token string
}

func (c *client) get(u string, accept string) ([]byte, int, error) {
	req, _ := http.NewRequest("GET", u, nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if c.token != "" && strings.HasPrefix(u, "https://api.github.com/") {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

// manifestDir is manifests/<first letter>/<Id segments...>.
func manifestDir(id string) string {
	return "manifests/" + strings.ToLower(id[:1]) + "/" + strings.ReplaceAll(id, ".", "/")
}

// latestVersion lists the package's version folders and returns the newest.
func (c *client) latestVersion(id string) (string, error) {
	u := "https://api.github.com/repos/microsoft/winget-pkgs/contents/" + manifestDir(id)
	b, code, err := c.get(u, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	if code == 404 {
		return "", fmt.Errorf("package %s not found in winget-pkgs", id)
	}
	if code != 200 {
		return "", fmt.Errorf("GitHub API %d for %s: %s", code, id, strings.TrimSpace(string(b)))
	}
	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &entries); err != nil {
		return "", err
	}
	var versions []string
	for _, e := range entries {
		// Sub-folders that do not start like a version (Beta, zh-TW, ...) are
		// other packages nested under this identifier.
		if e.Type == "dir" && looksLikeVersion(e.Name) {
			versions = append(versions, e.Name)
		}
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("package %s has no versions", id)
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) < 0 })
	return versions[len(versions)-1], nil
}

func looksLikeVersion(name string) bool {
	name = strings.TrimPrefix(name, "v")
	return name != "" && name[0] >= '0' && name[0] <= '9'
}

// compareVersions orders dotted versions numerically where possible, which is
// how winget itself compares them.
func compareVersions(a, b string) int {
	as, bs := splitVersion(a), splitVersion(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		xn, xe := strconv.Atoi(x)
		yn, ye := strconv.Atoi(y)
		switch {
		case xe == nil && ye == nil:
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	return 0
}

func splitVersion(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' || r == '+' })
}

func (c *client) fetchYAML(id, version, suffix string, out any) error {
	u := "https://raw.githubusercontent.com/microsoft/winget-pkgs/master/" + manifestDir(id) + "/" + url.PathEscape(version) + "/" + id + suffix
	b, code, err := c.get(u, "")
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("%s: HTTP %d", u, code)
	}
	return yaml.Unmarshal(b, out)
}

func (c *client) verify(a *catalog.App) (*LockEntry, error) {
	v, err := c.latestVersion(a.Winget)
	if err != nil {
		return nil, err
	}
	var inst installerManifest
	if err := c.fetchYAML(a.Winget, v, ".installer.yaml", &inst); err != nil {
		return nil, err
	}
	var loc localeManifest
	if err := c.fetchYAML(a.Winget, v, ".locale.en-US.yaml", &loc); err != nil {
		return nil, fmt.Errorf("%w (default locale manifest)", err)
	}
	if !strings.EqualFold(strings.TrimSpace(loc.Publisher), strings.TrimSpace(a.Publisher)) {
		return nil, fmt.Errorf("publisher is %q, catalog expects %q", loc.Publisher, a.Publisher)
	}
	entry := &LockEntry{Version: v, Publisher: loc.Publisher}
	hosts := map[string]bool{}
	hashes := map[string]bool{}
	add := func(rawURL, sum string) error {
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme != "https" {
			return fmt.Errorf("installer %q is not an https URL", rawURL)
		}
		hosts[u.Host] = true
		if sum != "" {
			hashes[strings.ToLower(sum)] = true
		}
		return nil
	}
	if inst.InstallerURL != "" {
		if err := add(inst.InstallerURL, inst.InstallerSha256); err != nil {
			return nil, err
		}
	}
	for _, i := range inst.Installers {
		if err := add(i.InstallerURL, i.InstallerSha256); err != nil {
			return nil, err
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("manifest %s lists no installers", v)
	}
	entry.Hosts = keys(hosts)
	entry.SHA256 = keys(hashes)
	return entry, nil
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	if out, err := exec.Command("gh", "auth", "token").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
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
	c := &client{http: &http.Client{Timeout: 30 * time.Second}, token: githubToken()}
	if c.token == "" {
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
		id    string
		entry *LockEntry
		err   error
	}
	results := make([]result, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			e, err := c.verify(cat.Apps[id])
			results[i] = result{id, e, err}
		}()
	}
	wg.Wait()

	failed := 0
	entries := map[string]*LockEntry{}
	for _, r := range results {
		if r.err != nil {
			failed++
			fmt.Printf("FAIL %-28s %v\n", r.id, r.err)
			continue
		}
		entries[r.id] = r.entry
		fmt.Printf("ok   %-28s %s  (%s)\n", r.id, r.entry.Version, strings.Join(r.entry.Hosts, ", "))
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

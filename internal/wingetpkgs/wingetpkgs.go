// Package wingetpkgs reads package manifests from the microsoft/winget-pkgs
// repository. It is used by the catalog verifier and importer, never by the
// app at run time.
package wingetpkgs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Client fetches manifests. Token (optional) raises the GitHub API rate limit
// from 60 to 5000 requests an hour.
type Client struct {
	HTTP  *http.Client
	Token string
	// APIURL and RawURL are overridable for tests.
	APIURL string
	RawURL string
	// CacheDir, when set, stores fetched packages as JSON for CacheTTL so
	// repeated bulk runs do not spend the GitHub API budget again.
	CacheDir string
}

// CacheTTL is how long a cached package is trusted.
const CacheTTL = 12 * time.Hour

// New returns a client using GITHUB_TOKEN, GH_TOKEN or `gh auth token`.
func New() *Client {
	return &Client{
		HTTP:   &http.Client{Timeout: 30 * time.Second},
		Token:  token(),
		APIURL: "https://api.github.com",
		RawURL: "https://raw.githubusercontent.com",
	}
}

func token() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if t := os.Getenv(k); t != "" {
			return t
		}
	}
	if out, err := exec.Command("gh", "auth", "token").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

// ARPEntry is an "Apps & features" entry the installer creates.
type ARPEntry struct {
	DisplayName string `yaml:"DisplayName"`
	Publisher   string `yaml:"Publisher"`
}

// Installer is one downloadable installer of a package.
type Installer struct {
	URL    string
	SHA256 string
	Type   string
}

// Package is the merged view of a package's newest version.
type Package struct {
	ID         string
	Version    string
	Installers []Installer
	ARP        []ARPEntry

	Publisher        string
	Name             string
	PackageURL       string
	PublisherURL     string
	License          string
	ShortDescription string
	Description      string
	Tags             []string
}

type installerYAML struct {
	InstallerType string     `yaml:"InstallerType"`
	InstallerURL  string     `yaml:"InstallerUrl"`
	Sha256        string     `yaml:"InstallerSha256"`
	ARP           []ARPEntry `yaml:"AppsAndFeaturesEntries"`
	Installers    []struct {
		InstallerType string     `yaml:"InstallerType"`
		InstallerURL  string     `yaml:"InstallerUrl"`
		Sha256        string     `yaml:"InstallerSha256"`
		ARP           []ARPEntry `yaml:"AppsAndFeaturesEntries"`
	} `yaml:"Installers"`
}

type versionYAML struct {
	DefaultLocale string `yaml:"DefaultLocale"`
}

type localeYAML struct {
	Publisher        string   `yaml:"Publisher"`
	PublisherURL     string   `yaml:"PublisherUrl"`
	PackageName      string   `yaml:"PackageName"`
	PackageURL       string   `yaml:"PackageUrl"`
	License          string   `yaml:"License"`
	ShortDescription string   `yaml:"ShortDescription"`
	Description      string   `yaml:"Description"`
	Tags             []string `yaml:"Tags"`
}

// ManifestDir is manifests/<first letter>/<Id segments...>.
func ManifestDir(id string) string {
	return "manifests/" + strings.ToLower(id[:1]) + "/" + strings.ReplaceAll(id, ".", "/")
}

// get retries rate limits (429) and server errors with exponential backoff,
// honouring Retry-After, so bulk runs survive GitHub's throttling.
func (c *Client) get(ctx context.Context, u, accept string) ([]byte, int, error) {
	delay := time.Second
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			return nil, 0, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if c.Token != "" && strings.HasPrefix(u, c.APIURL) {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, 0, err
		}
		b, rerr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		// GitHub's primary rate limit answers 403 with Remaining: 0; wait for the
		// reset (bounded) instead of failing hundreds of lookups in a bulk run.
		limited := resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0"
		retry := limited || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if !retry || attempt >= 5 {
			return b, resp.StatusCode, rerr
		}
		wait := delay
		if n, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && n > 0 && n < 120 {
			wait = time.Duration(n) * time.Second
		}
		if limited {
			if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				if d := time.Until(time.Unix(reset, 0)) + 2*time.Second; d > 0 && d < 70*time.Minute {
					fmt.Fprintf(os.Stderr, "GitHub rate limit reached; waiting %s\n", d.Round(time.Second))
					wait = d
					attempt-- // waiting out the limit is not a failed attempt
				}
			}
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
		delay *= 2
	}
}

// Versions lists the package's candidate version folders, newest first. Some
// entries are really nested packages (Canonical.Ubuntu has a folder 2404), so
// callers try candidates in order until one holds this package's manifests.
func (c *Client) Versions(ctx context.Context, id string) ([]string, error) {
	b, code, err := c.get(ctx, c.APIURL+"/repos/microsoft/winget-pkgs/contents/"+ManifestDir(id), "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, fmt.Errorf("package %s not found in winget-pkgs", id)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("GitHub API %d for %s: %s", code, id, strings.TrimSpace(string(b)))
	}
	var entries []struct{ Name, Type string }
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, err
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
		return nil, fmt.Errorf("package %s has no versions", id)
	}
	sort.Slice(versions, func(i, j int) bool { return CompareVersions(versions[i], versions[j]) > 0 })
	return versions, nil
}

func looksLikeVersion(name string) bool {
	name = strings.TrimPrefix(name, "v")
	return name != "" && name[0] >= '0' && name[0] <= '9'
}

// CompareVersions orders dotted versions numerically where possible.
func CompareVersions(a, b string) int {
	as, bs := split(a), split(b)
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
		if xe == nil && ye == nil {
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
			continue
		}
		if c := strings.Compare(x, y); c != 0 {
			return c
		}
	}
	return 0
}

func split(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' || r == '+' })
}

func (c *Client) fetchYAML(ctx context.Context, id, version, suffix string, out any) (bool, error) {
	u := c.RawURL + "/microsoft/winget-pkgs/master/" + ManifestDir(id) + "/" + url.PathEscape(version) + "/" + id + suffix
	b, code, err := c.get(ctx, u, "")
	if err != nil {
		return false, err
	}
	if code == http.StatusNotFound {
		return false, nil
	}
	if code != http.StatusOK {
		return false, fmt.Errorf("%s: HTTP %d", u, code)
	}
	return true, yaml.Unmarshal(b, out)
}

// Package loads the newest version's installer and default-locale manifests.
func (c *Client) Package(ctx context.Context, id string) (*Package, error) {
	if p := c.cached(id); p != nil {
		return p, nil
	}
	p, err := c.fetchPackage(ctx, id)
	if err == nil {
		c.store(p)
	}
	return p, err
}

func (c *Client) cachePath(id string) string {
	return filepath.Join(c.CacheDir, strings.ReplaceAll(id, "/", "_")+".json")
}

func (c *Client) cached(id string) *Package {
	if c.CacheDir == "" {
		return nil
	}
	path := c.cachePath(id)
	st, err := os.Stat(path)
	if err != nil || time.Since(st.ModTime()) > CacheTTL {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var p Package
	if json.Unmarshal(b, &p) != nil || p.ID != id {
		return nil
	}
	return &p
}

func (c *Client) store(p *Package) {
	if c.CacheDir == "" {
		return
	}
	if os.MkdirAll(c.CacheDir, 0o755) != nil {
		return
	}
	if b, err := json.Marshal(p); err == nil {
		_ = os.WriteFile(c.cachePath(p.ID), b, 0o644)
	}
}

func (c *Client) fetchPackage(ctx context.Context, id string) (*Package, error) {
	candidates, err := c.Versions(ctx, id)
	if err != nil {
		return nil, err
	}
	var v string
	var inst installerYAML
	for i, cand := range candidates {
		if i == 6 {
			break
		}
		inst = installerYAML{}
		ok, err := c.fetchYAML(ctx, id, cand, ".installer.yaml", &inst)
		if err != nil {
			return nil, err
		}
		if ok {
			v = cand
			break
		}
	}
	if v == "" {
		return nil, fmt.Errorf("%s: no version folder with an installer manifest", id)
	}
	p := &Package{ID: id, Version: v}
	if inst.InstallerURL != "" {
		p.Installers = append(p.Installers, Installer{inst.InstallerURL, inst.Sha256, inst.InstallerType})
	}
	p.ARP = append(p.ARP, inst.ARP...)
	for _, i := range inst.Installers {
		t := i.InstallerType
		if t == "" {
			t = inst.InstallerType
		}
		p.Installers = append(p.Installers, Installer{i.InstallerURL, i.Sha256, t})
		p.ARP = append(p.ARP, i.ARP...)
	}

	locale := "en-US"
	var ver versionYAML
	if ok, _ := c.fetchYAML(ctx, id, v, ".yaml", &ver); ok && ver.DefaultLocale != "" {
		locale = ver.DefaultLocale
	}
	var loc localeYAML
	if ok, err := c.fetchYAML(ctx, id, v, ".locale."+locale+".yaml", &loc); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("%s %s: no %s locale manifest", id, v, locale)
	}
	p.Publisher, p.PublisherURL, p.Name, p.PackageURL = loc.Publisher, loc.PublisherURL, loc.PackageName, loc.PackageURL
	p.License, p.ShortDescription, p.Description, p.Tags = loc.License, loc.ShortDescription, loc.Description, loc.Tags
	return p, nil
}

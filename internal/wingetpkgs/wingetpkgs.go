// Package wingetpkgs reads package manifests from the microsoft/winget-pkgs
// repository. It is used by the catalog verifier and importer, never by the
// app at run time.
package wingetpkgs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
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
	// Mirror, when set, answers every lookup from a local clone of
	// winget-pkgs instead of HTTP (see OpenMirror).
	Mirror *Mirror
	// BaseDelay is the first retry backoff and MaxWait the total time one
	// request may spend waiting on rate limits (zero means the defaults).
	BaseDelay time.Duration
	MaxWait   time.Duration
}

// CacheTTL is how long a cached package is trusted.
const CacheTTL = 12 * time.Hour

// Retry defaults: the first backoff, the most one request may spend waiting
// in total before it gives up, and the most attempts it makes.
const (
	DefaultBaseDelay = time.Second
	DefaultMaxWait   = 2 * time.Minute
	maxAttempts      = 8
)

// New returns a client using GITHUB_TOKEN, GH_TOKEN or `gh auth token`.
func New() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		Token:     token(),
		APIURL:    "https://api.github.com",
		RawURL:    "https://raw.githubusercontent.com",
		BaseDelay: DefaultBaseDelay,
		MaxWait:   DefaultMaxWait,
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
	// Icons are the manifest's own icon URLs (newer manifests only).
	Icons []string
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
	Icons            []struct {
		URL string `yaml:"IconUrl"`
	} `yaml:"Icons"`
}

// ManifestDir is manifests/<first letter>/<Id segments...>.
func ManifestDir(id string) string {
	return "manifests/" + strings.ToLower(id[:1]) + "/" + strings.ReplaceAll(id, ".", "/")
}

// ErrRateLimited marks a request GitHub kept throttling after every retry.
// It says nothing about the package itself, so callers can report the lookup
// as unverified instead of as a failure.
var ErrRateLimited = errors.New("rate limited by GitHub")

// RateLimitError is what get returns, wrapping ErrRateLimited, when a rate
// limit outlasts the retries.
type RateLimitError struct {
	URL    string
	Status int
	Body   string
}

func (e *RateLimitError) Error() string {
	msg := fmt.Sprintf("%s: HTTP %d, still rate limited after retries", e.URL, e.Status)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// rateLimited reports whether a response is GitHub throttling (primary or
// secondary rate limit) rather than a real answer about the resource.
func rateLimited(code int, h http.Header, body []byte) bool {
	if code == http.StatusTooManyRequests {
		return true
	}
	if code != http.StatusForbidden {
		return false
	}
	if h.Get("X-RateLimit-Remaining") == "0" || h.Get("Retry-After") != "" {
		return true
	}
	b := strings.ToLower(string(body))
	return strings.Contains(b, "rate limit") || strings.Contains(b, "abuse")
}

// serverWait is how long the server asked us to wait, if it said so.
func serverWait(h http.Header, now time.Time) (time.Duration, bool) {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second, true
		}
		if t, err := http.ParseTime(v); err == nil {
			return max(t.Sub(now), 0), true
		}
	}
	if h.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return max(time.Unix(reset, 0).Sub(now)+time.Second, 0), true
		}
	}
	return 0, false
}

// get retries rate limits (429, 403 rate-limit answers) and server errors
// with exponential backoff and full jitter, honouring Retry-After and
// X-RateLimit-Reset. One request never waits more than MaxWait in total: a
// rate limit that outlasts it is returned as a *RateLimitError, a server
// error as its last response.
func (c *Client) get(ctx context.Context, u, accept string) ([]byte, int, error) {
	base, budget := c.BaseDelay, c.MaxWait
	if base <= 0 {
		base = DefaultBaseDelay
	}
	if budget <= 0 {
		budget = DefaultMaxWait
	}
	var waited time.Duration
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
		if err != nil {
			return nil, 0, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		// raw.githubusercontent.com accepts the token too; authenticated
		// requests are throttled per token instead of per (shared CI) IP.
		if c.Token != "" && (strings.HasPrefix(u, c.APIURL) || strings.HasPrefix(u, c.RawURL)) {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, 0, err
		}
		b, rerr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		limited := rateLimited(resp.StatusCode, resp.Header, b)
		if !limited && resp.StatusCode < 500 {
			return b, resp.StatusCode, rerr
		}

		wait := time.Duration(rand.Int64N(int64(base<<min(attempt, 10)) + 1))
		if d, ok := serverWait(resp.Header, time.Now()); ok {
			wait = d + time.Duration(rand.Int64N(int64(base)/4+1))
		}
		if attempt+1 >= maxAttempts || waited+wait > budget {
			if limited {
				return nil, resp.StatusCode, &RateLimitError{URL: u, Status: resp.StatusCode, Body: firstLine(b)}
			}
			return b, resp.StatusCode, rerr
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
		waited += wait
	}
}

func firstLine(b []byte) string {
	s, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

// Versions lists the package's candidate version folders, newest first. Some
// entries are really nested packages (Canonical.Ubuntu has a folder 2404), so
// callers try candidates in order until one holds this package's manifests.
func (c *Client) Versions(ctx context.Context, id string) ([]string, error) {
	var names []string
	if c.Mirror != nil {
		dirs, ok := c.Mirror.subdirs(ManifestDir(id))
		if !ok {
			return nil, fmt.Errorf("package %s not found in winget-pkgs", id)
		}
		names = dirs
	} else {
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
		for _, e := range entries {
			if e.Type == "dir" {
				names = append(names, e.Name)
			}
		}
	}
	var versions []string
	for _, n := range names {
		// Sub-folders that do not start like a version (Beta, zh-TW, ...) are
		// other packages nested under this identifier.
		if looksLikeVersion(n) {
			versions = append(versions, n)
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
	if c.Mirror != nil {
		b, ok, err := c.Mirror.read(ctx, ManifestDir(id)+"/"+version+"/"+id+suffix)
		if err != nil || !ok {
			return false, err
		}
		return true, yaml.Unmarshal(b, out)
	}
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
	for _, i := range loc.Icons {
		if i.URL != "" {
			p.Icons = append(p.Icons, i.URL)
		}
	}
	return p, nil
}

// Package updater finds newer WinForge releases on GitHub and downloads their
// installer, refusing to hand back anything whose SHA-256 does not match the
// release's SHA256SUMS file.
package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Release is the part of a GitHub release the updater needs.
type Release struct {
	Tag    string
	URL    string // release page
	Notes  string
	Assets map[string]string // asset name -> download URL
}

// Updater talks to one repository's releases.
type Updater struct {
	Repo   string // owner/name
	APIURL string // https://api.github.com by default
	Client *http.Client
	// InstallerSuffix identifies the installer asset.
	InstallerSuffix string
}

// New returns an updater for the WinForge repository.
func New() *Updater {
	return &Updater{Repo: "Sebas1705/WinForge", APIURL: "https://api.github.com",
		Client: http.DefaultClient, InstallerSuffix: "-windows-installer.exe"}
}

// Latest returns the newest published (non-draft, non-prerelease) release.
func (u *Updater) Latest(ctx context.Context) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u.APIURL+"/repos/"+u.Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no release has been published yet")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var body struct {
		Tag    string `json:"tag_name"`
		URL    string `json:"html_url"`
		Body   string `json:"body"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, err
	}
	rel := &Release{Tag: body.Tag, URL: body.URL, Notes: body.Body, Assets: map[string]string{}}
	for _, a := range body.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

// Newer reports whether latest is a higher version than current. Development
// builds ("dev", "", anything unparsable) are never offered an update, and a
// pre-release sorts below its release (1.2.0-rc1 < 1.2.0).
func Newer(current, latest string) bool {
	c, okC := parse(current)
	l, okL := parse(latest)
	if !okC || !okL {
		return false
	}
	for i := 0; i < 3; i++ {
		if l.n[i] != c.n[i] {
			return l.n[i] > c.n[i]
		}
	}
	return c.pre != "" && l.pre == ""
}

type version struct {
	n   [3]int
	pre string
}

func parse(v string) (version, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var out version
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		if v[i] == '-' {
			out.pre = v[i+1:]
		}
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if v == "" || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out.n[i] = n
	}
	return out, true
}

// Progress is called with bytes downloaded and the total (0 if unknown).
type Progress func(done, total int64)

// Download fetches the release's installer into dir and verifies it against
// SHA256SUMS from the same release. On any mismatch the file is deleted and
// an error returned; the caller never sees an unverified installer.
func (u *Updater) Download(ctx context.Context, rel *Release, dir string, progress Progress) (string, error) {
	var name, url string
	for n, a := range rel.Assets {
		if strings.HasSuffix(n, u.InstallerSuffix) {
			name, url = n, a
		}
	}
	if name == "" {
		return "", errors.New("the release has no Windows installer")
	}
	sumsURL, ok := rel.Assets["SHA256SUMS"]
	if !ok {
		return "", errors.New("the release has no SHA256SUMS; refusing to install an unverifiable file")
	}
	prefix := "https://github.com/" + u.Repo + "/releases/download/"
	if u.APIURL != "https://api.github.com" { // tests point at a local server
		prefix = u.APIURL
	}
	for _, s := range []string{url, sumsURL} {
		if !strings.HasPrefix(s, prefix) {
			return "", fmt.Errorf("unexpected download location %q", s)
		}
	}

	sums, err := u.fetch(ctx, sumsURL, 1<<20)
	if err != nil {
		return "", err
	}
	want, err := findSum(string(sums), name)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	got, err := u.downloadTo(ctx, url, path, progress)
	if err != nil {
		os.Remove(path)
		return "", err
	}
	if got != want {
		os.Remove(path)
		return "", fmt.Errorf("checksum mismatch for %s: expected %s, got %s", name, want, got)
	}
	return path, nil
}

func (u *Updater) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func (u *Updater) downloadTo(ctx context.Context, url, path string, progress Progress) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: %s", resp.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	var done int64
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return "", err
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(done, resp.ContentLength)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// findSum reads "<hex>  <name>" lines (sha256sum format).
func findSum(sums, name string) (string, error) {
	sc := bufio.NewScanner(strings.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name && len(f[0]) == 64 {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS has no entry for %s", name)
}

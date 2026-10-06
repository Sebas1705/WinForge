// Command winforge-icons downloads an icon for every catalog app into
// frontend/public/icons, plus index.json (app id -> file). The files ship inside
// the app, so it never contacts a vendor to draw its own interface.
//
//	winforge-icons            fetch icons for apps that do not have one yet
//	winforge-icons -force     refetch everything
//	winforge-icons -only git  one app
//
// Sources, in order: the GitHub owner avatar when the homepage is a GitHub
// repository, the winget manifest's own icon, the icons the homepage declares,
// then /favicon.ico. Content is identified by its bytes, never by what the
// server claims, and SVGs with scripts are refused.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/icons"
	"github.com/Sebas1705/WinForge/internal/wingetpkgs"
)

const outDir = "frontend/public/icons"

func main() {
	force := flag.Bool("force", false, "refetch icons that already exist")
	only := flag.String("only", "", "fetch a single app id")
	github := flag.Bool("github", false, "refetch apps whose homepage is on GitHub (after changing avatar rules)")
	flag.Parse()

	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal(err)
	}
	index := readIndex()

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to %s", req.URL.Scheme)
			}
			return nil
		},
	}
	wp := wingetpkgs.New()
	wp.CacheDir = ".winget-cache"
	owners := &ownerTypes{client: client, token: wp.Token, seen: map[string]string{}}

	var ids []string
	for id := range cat.Apps {
		_, onGitHub := icons.GitHubOwner(cat.Apps[id].Homepage)
		if (*only == "" || *only == id) && (*force || *only != "" || index[id] == "" || (*github && onGitHub)) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if *github {
		// Start clean: an app that no longer qualifies for an icon must lose the
		// old one, not keep it because nothing replaced it.
		for _, id := range ids {
			old, _ := filepath.Glob(filepath.Join(outDir, id+".*"))
			for _, o := range old {
				os.Remove(o)
			}
			delete(index, id)
		}
	}

	type result struct {
		file, via string
	}
	results := make([]result, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			file, via := fetchApp(context.Background(), client, wp, owners, cat.Apps[id])
			results[i] = result{file, via}
		}()
	}
	wg.Wait()

	got, missing := 0, []string{}
	for i, id := range ids {
		if results[i].file == "" {
			missing = append(missing, id)
			continue
		}
		index[id] = results[i].file
		got++
	}
	// Drop index entries whose file vanished or whose app left the catalog.
	for id, f := range index {
		if cat.Apps[id] == nil {
			delete(index, id)
			continue
		}
		if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
			delete(index, id)
		}
	}
	generic := dropGeneric(cat, index)
	for _, id := range generic {
		delete(index, id)
	}
	writeIndex(index)
	if len(generic) > 0 {
		fmt.Printf("dropped %d generic icons shared across publishers: %s\n", len(generic), strings.Join(generic, " "))
	}
	fmt.Printf("%d fetched, %d without an icon, %d/%d apps have one now\n", got, len(missing), len(index), len(cat.Apps))
	if len(missing) > 0 && len(missing) <= 40 {
		fmt.Println("no icon:", strings.Join(missing, " "))
	}
}

func fetchApp(ctx context.Context, c *http.Client, wp *wingetpkgs.Client, owners *ownerTypes, a *catalog.App) (file, via string) {
	var cands []string
	if owner, ok := icons.GitHubOwner(a.Homepage); ok && icons.UseAvatar(owners.typeOf(ctx, owner)) {
		cands = append(cands, icons.AvatarURL(owner))
	}
	if p, err := wp.Package(ctx, a.Winget); err == nil {
		cands = append(cands, p.Icons...)
	}
	if page, final := get(ctx, c, a.Homepage, 1<<20); page != nil {
		if base, err := url.Parse(final); err == nil {
			for _, cd := range icons.FromHTML(page, base) {
				cands = append(cands, cd.URL)
			}
		}
	}
	if u, err := url.Parse(a.Homepage); err == nil && u.Scheme == "https" {
		cands = append(cands, (&url.URL{Scheme: "https", Host: u.Host, Path: "/favicon.ico"}).String())
	}
	seen := map[string]bool{}
	for _, u := range cands {
		if seen[u] || !strings.HasPrefix(u, "https://") || icons.Generic(u) {
			continue
		}
		seen[u] = true
		body, _ := get(ctx, c, u, icons.MaxBytes+1)
		ext, ok := icons.Sniff(body)
		if !ok {
			continue
		}
		name := a.ID + "." + ext
		// Replace any older icon of a different type.
		old, _ := filepath.Glob(filepath.Join(outDir, a.ID+".*"))
		for _, o := range old {
			os.Remove(o)
		}
		if err := os.WriteFile(filepath.Join(outDir, name), body, 0o644); err != nil {
			return "", ""
		}
		return name, u
	}
	return "", ""
}

// ownerTypes looks up whether a GitHub account is an Organization or a User,
// once per owner. Personal avatars are photos of people, so only organizations'
// are used; a failed lookup counts as "not an organization".
type ownerTypes struct {
	client *http.Client
	token  string
	mu     sync.Mutex
	seen   map[string]string
}

func (o *ownerTypes) typeOf(ctx context.Context, owner string) string {
	o.mu.Lock()
	if t, ok := o.seen[owner]; ok {
		o.mu.Unlock()
		return t
	}
	o.mu.Unlock()
	t := ""
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/users/"+url.PathEscape(owner), nil)
	if err == nil {
		req.Header.Set("Accept", "application/vnd.github+json")
		if o.token != "" {
			req.Header.Set("Authorization", "Bearer "+o.token)
		}
		if resp, err := o.client.Do(req); err == nil {
			var body struct {
				Type string `json:"type"`
			}
			if resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) == nil {
				t = body.Type
			}
			resp.Body.Close()
		}
	}
	o.mu.Lock()
	o.seen[owner] = t
	o.mu.Unlock()
	return t
}

// get fetches a URL, returning at most limit bytes and the final URL. Failures
// return nil: a missing icon is normal, not an error.
func get(ctx context.Context, c *http.Client, u string, limit int64) ([]byte, string) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, ""
	}
	req.Header.Set("User-Agent", "WinForge-icons/1.0 (+https://github.com/Sebas1705/WinForge)")
	req.Header.Set("Accept", "image/*,text/html;q=0.8")
	resp, err := c.Do(req)
	if err != nil {
		return nil, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ""
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, ""
	}
	return b, resp.Request.URL.String()
}

// brand reduces a publisher to its first word, so "JetBrains s.r.o." and
// "JetBrains" count as one brand.
func brand(publisher string) string {
	f := strings.FieldsFunc(strings.ToLower(publisher), func(r rune) bool { return r == ' ' || r == ',' || r == '.' })
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// dropGeneric removes icons that are byte-identical across apps from three or
// more different brands: a host's default icon, not the project's own.
// The same brand on several of its own products (JetBrains, Python versions)
// is kept.
func dropGeneric(cat *catalog.Catalog, index map[string]string) []string {
	byHash := map[[32]byte][]string{}
	for id, f := range index {
		b, err := os.ReadFile(filepath.Join(outDir, f))
		if err != nil {
			continue
		}
		h := sha256.Sum256(b)
		byHash[h] = append(byHash[h], id)
	}
	var drop []string
	for _, ids := range byHash {
		pubs := map[string]bool{}
		for _, id := range ids {
			pubs[brand(cat.Apps[id].Publisher)] = true
		}
		if len(ids) >= 3 && len(pubs) >= 3 {
			for _, id := range ids {
				os.Remove(filepath.Join(outDir, index[id]))
			}
			drop = append(drop, ids...)
		}
	}
	sort.Strings(drop)
	return drop
}

func readIndex() map[string]string {
	m := map[string]string{}
	if b, err := os.ReadFile(filepath.Join(outDir, "index.json")); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func writeIndex(m map[string]string) {
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(outDir, "index.json"), append(b, '\n'), 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

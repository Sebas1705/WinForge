// Package icons finds and validates app icons for the catalog. It is used by
// the winforge-icons build tool; the app itself only ships the resulting files.
//
// Sources, in order of trust: the winget manifest's own icon URL, the icon the
// project's homepage declares, and the owner avatar for projects whose
// homepage is a GitHub repository. Anything that is not a small, plain image
// is rejected.
package icons

import (
	"bytes"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// MaxBytes is the largest icon file accepted.
const MaxBytes = 64 * 1024

// Candidate is an icon URL found on a page, with a rough quality score.
type Candidate struct {
	URL   string
	Score int
}

var sizeRe = regexp.MustCompile(`(\d+)x(\d+)`)

// FromHTML returns the icon candidates a page declares, best first. base is the
// page's final URL, used to resolve relative links. Only https links are kept.
func FromHTML(doc []byte, base *url.URL) []Candidate {
	root, err := html.Parse(bytes.NewReader(doc))
	if err != nil {
		return nil
	}
	var out []Candidate
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, href, sizes, typ string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "rel":
					rel = strings.ToLower(a.Val)
				case "href":
					href = strings.TrimSpace(a.Val)
				case "sizes":
					sizes = a.Val
				case "type":
					typ = strings.ToLower(a.Val)
				}
			}
			if strings.Contains(rel, "icon") && !strings.Contains(rel, "mask-icon") && href != "" {
				if u, err := base.Parse(href); err == nil && u.Scheme == "https" {
					out = append(out, Candidate{URL: u.String(), Score: score(rel, sizes, typ, u.Path)})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func score(rel, sizes, typ, path string) int {
	s := 10
	if strings.Contains(rel, "apple-touch-icon") {
		s += 40
	}
	if m := sizeRe.FindStringSubmatch(sizes); m != nil {
		w, _ := strconv.Atoi(m[1])
		switch {
		case w >= 64 && w <= 256:
			s += 30
		case w > 256:
			s += 10 // large files, rarely worth it
		default:
			s += w / 8
		}
	}
	if strings.Contains(typ, "svg") || strings.HasSuffix(strings.ToLower(path), ".svg") {
		s += 35 // scales cleanly
	}
	if strings.HasSuffix(strings.ToLower(path), ".ico") {
		s -= 5
	}
	return s
}

// GitHubOwner returns the owner of a github.com/<owner>[/<repo>] homepage.
func GitHubOwner(homepage string) (string, bool) {
	u, err := url.Parse(homepage)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") {
		return "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", false
	}
	return parts[0], true
}

// AvatarURL is the GitHub owner's avatar at icon size.
func AvatarURL(owner string) string {
	return "https://github.com/" + url.PathEscape(owner) + ".png?size=96"
}

// genericHosts serve one icon for everything they host (a code host's own logo
// rather than the project's), so an icon from them identifies nothing.
var genericHosts = []string{
	"github.githubassets.com", "githubassets.com", "sourceforge.net", "a.fsdn.com", "gitlab.com",
	"codeberg.org", "bitbucket.org", "wikimedia.org",
}

// Generic reports whether the icon URL belongs to a host whose icons are not
// specific to the project.
func Generic(iconURL string) bool {
	u, err := url.Parse(iconURL)
	if err != nil {
		return true
	}
	h := strings.ToLower(u.Hostname())
	for _, g := range genericHosts {
		if h == g || strings.HasSuffix(h, "."+g) {
			return true
		}
	}
	return false
}

// Sniff identifies an image by content (never by the server's claim) and
// returns the file extension to store it under. SVG must be plain: no scripts,
// event handlers or embedded foreign content.
func Sniff(b []byte) (ext string, ok bool) {
	if len(b) == 0 || len(b) > MaxBytes {
		return "", false
	}
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "png", true
	case bytes.HasPrefix(b, []byte{0, 0, 1, 0}):
		return "ico", true
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "jpg", true
	case bytes.HasPrefix(b, []byte("GIF8")):
		return "gif", true
	case len(b) > 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "webp", true
	}
	head := strings.ToLower(string(b[:min(len(b), 512)]))
	if strings.Contains(head, "<svg") && safeSVG(strings.ToLower(string(b))) {
		return "svg", true
	}
	return "", false
}

func safeSVG(s string) bool {
	for _, bad := range []string{"<script", "javascript:", "<foreignobject", "<iframe", "<!entity", "<image", "onload=", "onerror=", "onclick=", "@import"} {
		if strings.Contains(s, bad) {
			return false
		}
	}
	return true
}

// UseAvatar reports whether a GitHub account's avatar may stand in for a
// project icon. Only organizations qualify: a personal account's avatar is a
// person's photo, which has no business in an app list. When the type is
// unknown (the lookup failed) the answer is no.
func UseAvatar(accountType string) bool {
	return strings.EqualFold(accountType, "Organization")
}

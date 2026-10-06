package icons_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/icons"
)

func TestFromHTMLPrefersTouchIconsAndSvgOverTinyFavicons(t *testing.T) {
	page := `<html><head>
	<link rel="shortcut icon" href="/favicon.ico">
	<link rel="icon" sizes="16x16" href="/16.png">
	<link rel="apple-touch-icon" sizes="180x180" href="/touch.png">
	<link rel="icon" type="image/svg+xml" href="logo.svg">
	<link rel="mask-icon" href="/mask.svg">
	<link rel="icon" href="http://insecure.example/x.png">
	<link rel="stylesheet" href="/a.css">
	</head><body></body></html>`
	base, _ := url.Parse("https://example.com/products/")
	got := icons.FromHTML([]byte(page), base)
	var urls []string
	for _, c := range got {
		urls = append(urls, c.URL)
	}
	want := []string{"https://example.com/touch.png", "https://example.com/products/logo.svg"}
	if len(urls) != 4 || urls[0] != want[0] || urls[1] != want[1] {
		t.Fatalf("order/filter wrong: %v", urls)
	}
	for _, u := range urls {
		if strings.HasPrefix(u, "http://") || strings.Contains(u, "mask") {
			t.Fatalf("must keep https icons only: %v", urls)
		}
	}
}

func TestGitHubOwnerAndGenericHosts(t *testing.T) {
	if o, ok := icons.GitHubOwner("https://github.com/BurntSushi/ripgrep"); !ok || o != "BurntSushi" {
		t.Fatal(o, ok)
	}
	if _, ok := icons.GitHubOwner("https://notgithub.com/x/y"); ok {
		t.Fatal("only github.com")
	}
	if _, ok := icons.GitHubOwner("https://github.com/"); ok {
		t.Fatal("no owner")
	}
	if !icons.Generic("https://github.githubassets.com/favicons/favicon.svg") || !icons.Generic("https://a.fsdn.com/x.png") || icons.Generic("https://www.mozilla.org/favicon.ico") {
		t.Fatal("generic host detection")
	}
}

func TestSniffIdentifiesByContentAndRejectsDangerousFiles(t *testing.T) {
	ok := map[string]string{
		"png":  "\x89PNG\r\n\x1a\n....",
		"ico":  "\x00\x00\x01\x00\x01\x00",
		"jpg":  "\xff\xd8\xff\xe0....",
		"gif":  "GIF89a....",
		"webp": "RIFF\x00\x00\x00\x00WEBPVP8 ",
		"svg":  `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>`,
	}
	for ext, body := range ok {
		if got, good := icons.Sniff([]byte(body)); !good || got != ext {
			t.Errorf("%s: got %q %v", ext, got, good)
		}
	}
	bad := map[string]string{
		"html":         "<!doctype html><html><body>Not found</body></html>",
		"svg script":   `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"svg handler":  `<svg onload="x()"></svg>`,
		"svg js link":  `<svg><a href="javascript:x()"><rect/></a></svg>`,
		"svg embedded": `<svg><foreignObject><div/></foreignObject></svg>`,
		"empty":        "",
		"too big":      "\x89PNG\r\n\x1a\n" + strings.Repeat("x", icons.MaxBytes),
		"text":         "hello",
	}
	for name, body := range bad {
		if _, good := icons.Sniff([]byte(body)); good {
			t.Errorf("%s must be rejected", name)
		}
	}
}

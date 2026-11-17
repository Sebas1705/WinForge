// Command winforge-eol refreshes catalogdata/eol.yml, the end-of-support table the PC health page
// reads. It is run by a maintainer, never by the app: WinForge itself makes no network request for
// this feature and sends nothing about the PC.
//
// It asks https://endoflife.date/api/<product>.json for each product in products.go (one public
// request per product, with no information about anyone's PC), validates the result against the
// catalog and rewrites the file deterministically. The committed file is the source of truth; read
// the diff before committing.
//
//	go run ./cmd/winforge-eol                 rewrite catalogdata/eol.yml
//	go run ./cmd/winforge-eol -check          fetch, compare with the committed file, change nothing
//	go run ./cmd/winforge-eol -dir ./json     read <product>.json files from a folder instead
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/eol"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("winforge-eol", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", filepath.Join("catalogdata", eol.File), "the file to write")
	asOf := fs.String("asof", time.Now().UTC().Format(eol.DayLayout), "the as-of day to write (YYYY-MM-DD)")
	dir := fs.String("dir", "", "read <product>.json files from this folder instead of the network")
	base := fs.String("base", "https://endoflife.date/api", "base URL of the endoflife.date API")
	check := fs.Bool("check", false, "compare with the existing file and print the differences; write nothing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if _, err := eol.ParseDay(*asOf); err != nil {
		fmt.Fprintf(stderr, "-asof %q is not a YYYY-MM-DD date\n", *asOf)
		return 2
	}
	cat, err := catalog.Load(catalogdata.FS)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	fetch := fetchHTTP(*base)
	if *dir != "" {
		fetch = func(slug string) ([]byte, error) { return os.ReadFile(filepath.Join(*dir, slug+".json")) }
	}
	d, warns, err := generate(specs, fetch, *asOf)
	for _, w := range warns {
		fmt.Fprintln(stderr, "warning:", w)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if errs := d.Validate(func(id string) bool { return cat.Apps[id] != nil }); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(stderr, "invalid:", e)
		}
		return 1
	}
	body := render(d, vendorsOf(specs))

	if *check {
		old, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		// The as-of line always differs; ignore it so a clean check really means "same dates".
		diff := diffLines(dropAsOf(string(old)), dropAsOf(string(body)))
		if len(diff) == 0 {
			fmt.Fprintln(stdout, "eol.yml matches the source (apart from asOf).")
			return 0
		}
		fmt.Fprintln(stdout, "eol.yml differs from the source (- committed, + source):")
		for _, l := range diff {
			fmt.Fprintln(stdout, l)
		}
		return 1
	}
	if err := os.WriteFile(*out, body, 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprintf(stdout, "wrote %s: %d products as of %s\n", *out, len(d.Products), d.AsOf)
	return 0
}

func dropAsOf(s string) string {
	lines := strings.Split(s, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if !strings.HasPrefix(l, "asOf:") {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

// fetchHTTP reads one product from the API. It sends nothing but the product name.
func fetchHTTP(base string) func(slug string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	return func(slug string) ([]byte, error) {
		req, err := http.NewRequest(http.MethodGet, strings.TrimRight(base, "/")+"/"+slug+".json", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "winforge-eol (maintainer tool)")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	}
}

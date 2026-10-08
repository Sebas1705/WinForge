// Package games downloads the freely redistributable games of the catalog into
// a games folder and removes them again. Every download is checked against the
// SHA-256 pinned in the catalog before a byte of it is used, extraction cannot
// leave the game's folder, and nothing is written outside the games folder.
package games

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

const receiptName = "winforge-game.json"

const (
	maxFiles      = 20000
	maxExpandedBy = 4 // a zip may expand to this many times its declared size...
	minExpanded   = 64 << 20
	maxExpanded   = int64(4) << 30
)

// Receipt records what was installed, so the game can be recognised and removed.
type Receipt struct {
	ID          string    `json:"id"`
	SHA256      string    `json:"sha256"`
	URL         string    `json:"url"`
	Files       int       `json:"files"`
	InstalledAt time.Time `json:"installedAt"`
}

// Dir is where a game lives: <root>/<system>/<id>.
func Dir(root string, g catalog.Game) string {
	return filepath.Join(root, g.System, g.ID)
}

// EntryPath is the file an emulator is handed to play the game, or "".
func EntryPath(root string, g catalog.Game) string {
	if g.Entry == "" {
		return ""
	}
	return filepath.Join(Dir(root, g), filepath.FromSlash(g.Entry))
}

// Progress is called with the bytes downloaded so far and the expected total.
type Progress func(done, total int64)

// Install downloads, verifies and unpacks a game. It is safe to run again: a
// game that is already installed with the same checksum is left as it is. On
// any failure nothing is left behind.
func Install(ctx context.Context, client *http.Client, root string, g catalog.Game, progress Progress) (*Receipt, error) {
	if !strings.HasPrefix(g.URL, "https://") {
		return nil, errors.New("downloads must use https")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	dir := Dir(root, g)
	if r := readReceipt(dir); r != nil && r.ID == g.ID && r.SHA256 == g.SHA256 {
		return r, nil
	}
	if _, err := os.Lstat(dir); err == nil {
		return nil, fmt.Errorf("the folder %s already exists and was not installed by WinForge: move it or choose another games folder", dir)
	}

	tmp, err := os.CreateTemp(root, "winforge-download-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if err := download(ctx, client, g, tmp, progress); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	part := dir + ".part"
	_ = os.RemoveAll(part)
	installed := false
	defer func() {
		if !installed { // a failed install leaves no folder behind, not even an empty system folder
			_ = os.RemoveAll(part)
			_ = os.Remove(filepath.Dir(part))
		}
	}()
	if err := os.MkdirAll(part, 0o755); err != nil {
		return nil, err
	}
	n := 1
	switch g.Kind {
	case "file":
		if err := copyFile(tmp.Name(), filepath.Join(part, g.File)); err != nil {
			_ = os.RemoveAll(part)
			return nil, err
		}
	case "zip":
		if n, err = unzip(tmp.Name(), part, g.Size); err != nil {
			_ = os.RemoveAll(part)
			return nil, err
		}
	default:
		_ = os.RemoveAll(part)
		return nil, fmt.Errorf("unknown kind %q", g.Kind)
	}
	if g.Entry != "" {
		if st, err := os.Stat(filepath.Join(part, filepath.FromSlash(g.Entry))); err != nil || !st.Mode().IsRegular() {
			_ = os.RemoveAll(part)
			return nil, fmt.Errorf("the download does not contain %s", g.Entry)
		}
	}
	r := &Receipt{ID: g.ID, SHA256: g.SHA256, URL: g.URL, Files: n, InstalledAt: time.Now().UTC()}
	b, _ := json.MarshalIndent(r, "", "  ")
	if err := os.WriteFile(filepath.Join(part, receiptName), b, 0o644); err != nil {
		_ = os.RemoveAll(part)
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		_ = os.RemoveAll(part)
		return nil, err
	}
	if err := os.Rename(part, dir); err != nil {
		return nil, err
	}
	installed = true
	return r, nil
}

// download fetches g.URL into f, refusing anything that is not the pinned file.
func download(ctx context.Context, client *http.Client, g catalog.Game, f *os.File, progress Progress) error {
	c := *client
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing a redirect to %s", req.URL.Scheme)
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "WinForge/1 (+https://github.com/Sebas1705/WinForge)")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("could not download %s: %w", g.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("could not download %s: the server answered %s", g.Name, resp.Status)
	}
	if resp.ContentLength > g.Size {
		return fmt.Errorf("%s is larger than expected (%d bytes, not %d)", g.Name, resp.ContentLength, g.Size)
	}
	h := sha256.New()
	var done int64
	buf := make([]byte, 128<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			done += int64(n)
			if done > g.Size {
				return fmt.Errorf("%s is larger than expected", g.Name)
			}
			h.Write(buf[:n])
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			if progress != nil {
				progress(done, g.Size)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("the download of %s was interrupted: %w", g.Name, rerr)
		}
	}
	if done != g.Size {
		return fmt.Errorf("%s is smaller than expected (%d bytes, not %d)", g.Name, done, g.Size)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != g.SHA256 {
		return fmt.Errorf("%s does not match its published checksum, so it was discarded", g.Name)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// unzip extracts a zip into dir and returns how many files it wrote. Entries
// that are not plain files inside dir are an error, not skipped silently.
func unzip(zipPath, dir string, declared int64) (int, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, errors.New("the download is not a valid zip file")
	}
	defer zr.Close()
	if len(zr.File) > maxFiles {
		return 0, errors.New("the zip has too many files")
	}
	budget := declared * maxExpandedBy
	if budget < minExpanded {
		budget = minExpanded
	}
	if budget > maxExpanded {
		budget = maxExpanded
	}
	count := 0
	for _, f := range zr.File {
		name := strings.TrimSuffix(f.Name, "/")
		if name == "" {
			continue
		}
		if !safeEntry(name) {
			return 0, fmt.Errorf("the zip contains an unsafe path (%q)", f.Name)
		}
		if f.Mode()&os.ModeSymlink != 0 || (!f.Mode().IsRegular() && !f.FileInfo().IsDir()) {
			return 0, fmt.Errorf("the zip contains something that is not a plain file (%q)", f.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return 0, err
			}
			continue
		}
		if int64(f.UncompressedSize64) > budget {
			return 0, errors.New("the zip expands to more than expected")
		}
		budget -= int64(f.UncompressedSize64)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return 0, err
		}
		rc, err := f.Open()
		if err != nil {
			return 0, err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			rc.Close()
			return 0, fmt.Errorf("could not write %s: %w", name, err)
		}
		// A lying header cannot write more than it declared.
		n, err := io.Copy(out, io.LimitReader(rc, int64(f.UncompressedSize64)+1))
		rc.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil || n != int64(f.UncompressedSize64) {
			return 0, fmt.Errorf("the zip is damaged (%s)", name)
		}
		count++
	}
	return count, nil
}

// safeEntry is true for a relative path that stays inside the folder.
func safeEntry(name string) bool {
	if strings.ContainsAny(name, `\:`) || strings.HasPrefix(name, "/") || path.Clean(name) != name {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." || seg == "" || strings.TrimRight(seg, ". ") != seg {
			return false
		}
	}
	return true
}

func readReceipt(dir string) *Receipt {
	b, err := os.ReadFile(filepath.Join(dir, receiptName))
	if err != nil {
		return nil
	}
	var r Receipt
	if json.Unmarshal(b, &r) != nil || r.ID == "" {
		return nil
	}
	return &r
}

// State lists which of the games are installed under root.
func State(root string, all []catalog.Game) map[string]Receipt {
	out := map[string]Receipt{}
	if root == "" {
		return out
	}
	for _, g := range all {
		if r := readReceipt(Dir(root, g)); r != nil && r.ID == g.ID {
			out[g.ID] = *r
		}
	}
	return out
}

// Remove deletes an installed game's folder. It only ever deletes a folder that
// holds this game's receipt, and only inside root.
func Remove(root string, g catalog.Game) error {
	dir := Dir(root, g)
	rel, err := filepath.Rel(root, dir)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return errors.New("the game is not inside the games folder")
	}
	if r := readReceipt(dir); r == nil || r.ID != g.ID {
		return errors.New("this game was not installed by WinForge, so it is not removed")
	}
	return os.RemoveAll(dir)
}

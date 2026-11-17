// Package diag builds WinForge's diagnostic report: one JSON document a person can look at,
// save and attach to a bug report by hand. WinForge sends nothing anywhere.
//
// The report is a whitelist. It contains only the fields of Report below: WinForge's version and
// build, the Windows version, edition and architecture, the language, whether it runs as
// administrator, the winget version, the size of the catalog, the names, sizes and dates of
// WinForge's own files in its own folder, the tail of WinForge's own log when it has one, and counts
// from the last scans. It never contains the list of installed software, paths outside WinForge's
// folder, environment variables, registry contents or the contents of profiles; the package has no
// way to be handed those. Every string goes through a Scrubber that removes the Windows user name
// and the profile folder.
//
// Everything here is pure: the machine is read by the caller and passed in as Input, so the same
// code runs in tests with a planted user name.
package diag

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// Limits keep the report small enough to read before saving.
const (
	// LogLines is how many lines of WinForge's own log are kept (the last ones).
	LogLines = 50
	// MaxLineLen cuts a single log line.
	MaxLineLen = 300
	// MaxFiles caps the file list.
	MaxFiles = 200
	// maxDepth is how deep the file walk goes below WinForge's folder.
	maxDepth = 3
	// logTailBytes is how much of the end of the log file is read.
	logTailBytes = 64 << 10
	// SchemaVersion is the version of the report layout.
	SchemaVersion = 1
)

// Windows is what the Windows installation says about itself.
type Windows struct {
	Version string `json:"version"` // for example 10.0.26100
	Build   string `json:"build"`   // for example 26100.4351
	Edition string `json:"edition"` // for example Professional
	Arch    string `json:"arch"`    // amd64 or arm64
}

// Scan is the summary of one scan: a name and counts, never the things counted.
type Scan struct {
	Name   string         `json:"name"`
	Counts map[string]int `json:"counts"`
}

// Input is everything the report is made of, read by the caller.
type Input struct {
	Version         string
	Build           string
	Windows         Windows
	Language        string
	Admin           bool
	WingetVersion   string // "" when winget was not found
	CatalogApps     int
	CatalogProfiles int
	// DataFS is WinForge's own folder (%AppData%\WinForge). nil means there is none.
	DataFS fs.FS
	// DataDir is the real path of DataFS. It is never shown: log lines that mention it show
	// %AppData%WinForge instead, and every other absolute path in a log line becomes <path>.
	DataDir string
	Scans   []Scan
	// Users and Homes are what to scrub: the Windows user names and profile folders of this PC.
	Users []string
	Homes []string
}

// Report is the document. Its fields are the whole whitelist.
type Report struct {
	Schema        int        `json:"schema"`
	Kind          string     `json:"kind"`
	WinForge      WinForge   `json:"winforge"`
	Windows       Windows    `json:"windows"`
	Language      string     `json:"language"`
	Administrator bool       `json:"administrator"`
	Winget        Winget     `json:"winget"`
	Catalog       Catalog    `json:"catalog"`
	DataFolder    string     `json:"dataFolder"`
	Files         []FileInfo `json:"files"`
	Log           *Log       `json:"log"`
	Scans         []Scan     `json:"scans"`
}

type WinForge struct {
	Version string `json:"version"`
	Build   string `json:"build"`
}

type Winget struct {
	Found   bool   `json:"found"`
	Version string `json:"version"`
}

type Catalog struct {
	Apps     int `json:"apps"`
	Profiles int `json:"profiles"`
}

// FileInfo is one of WinForge's own files: its name relative to the folder, never its contents.
type FileInfo struct {
	Name     string `json:"name"`
	Bytes    int64  `json:"bytes"`
	Modified string `json:"modified"` // RFC 3339, UTC
}

// Log is the end of WinForge's own log file.
type Log struct {
	File  string   `json:"file"`
	Lines []string `json:"lines"`
}

// Kind identifies the document.
const Kind = "winforge-diagnostic-report"

// DataFolderLabel is how the folder is named in the report: the variable, not the real path.
const DataFolderLabel = `%AppData%\WinForge`

// Build assembles the report from the input, scrubbing every string.
func Build(in Input) Report {
	sc := NewScrubber(in.Users, in.Homes)
	c := sc.Clean
	r := Report{
		Schema:        SchemaVersion,
		Kind:          Kind,
		WinForge:      WinForge{Version: c(in.Version), Build: c(in.Build)},
		Windows:       Windows{Version: c(in.Windows.Version), Build: c(in.Windows.Build), Edition: c(in.Windows.Edition), Arch: c(in.Windows.Arch)},
		Language:      c(in.Language),
		Administrator: in.Admin,
		Winget:        Winget{Found: in.WingetVersion != "", Version: c(in.WingetVersion)},
		Catalog:       Catalog{Apps: in.CatalogApps, Profiles: in.CatalogProfiles},
		DataFolder:    DataFolderLabel,
		Files:         []FileInfo{},
		Scans:         []Scan{},
	}
	if in.DataFS != nil {
		r.Files = listFiles(in.DataFS, sc)
		r.Log = tailLog(in.DataFS, sc, in.DataDir)
	}
	for _, s := range in.Scans {
		counts := map[string]int{}
		for k, v := range s.Counts {
			counts[c(k)] = v
		}
		r.Scans = append(r.Scans, Scan{Name: c(s.Name), Counts: counts})
	}
	return r
}

// Render is the text of the file: the report as indented JSON. This exact text is what the
// preview shows and what is saved. A last pass scrubs the finished text too, which also catches
// the JSON-escaped form of a path (C:\\Users\\ana).
func Render(in Input) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(Build(in))
	return NewScrubber(in.Users, in.Homes).CleanPaths(b.String())
}

func listFiles(fsys fs.FS, sc Scrubber) []FileInfo {
	out := []FileInfo{}
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are left out
		}
		if d.IsDir() {
			if p != "." && strings.Count(p, "/") >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if len(out) >= MaxFiles {
			return fs.SkipAll
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		out = append(out, FileInfo{Name: sc.Clean(p), Bytes: info.Size(), Modified: info.ModTime().UTC().Format(time.RFC3339)})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// tailLog finds WinForge's own log (the newest *.log in the folder itself) and returns its last lines.
func tailLog(fsys fs.FS, sc Scrubber, dataDir string) *Log {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	var newest string
	var when time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(path.Ext(e.Name()), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(when) {
			newest, when = e.Name(), info.ModTime()
		}
	}
	if newest == "" {
		return nil
	}
	f, err := fsys.Open(newest)
	if err != nil {
		return nil
	}
	defer f.Close()
	data := readTail(f)
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > LogLines {
		lines = lines[len(lines)-LogLines:]
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = sc.Clean(HidePaths(strings.ToValidUTF8(l, "?"), dataDir))
		if r := []rune(l); len(r) > MaxLineLen {
			l = string(r[:MaxLineLen]) + "..."
		}
		out = append(out, l)
	}
	return &Log{File: sc.Clean(newest), Lines: out}
}

// readTail reads at most logTailBytes from the end of f.
func readTail(f fs.File) []byte {
	if s, ok := f.(interface {
		Seek(int64, int) (int64, error)
	}); ok {
		if st, err := f.Stat(); err == nil && st.Size() > logTailBytes {
			if _, err := s.Seek(st.Size()-logTailBytes, 0); err == nil {
				b := make([]byte, logTailBytes)
				n, _ := readFull(f, b)
				// the first line is probably cut in half
				b = b[:n]
				if i := bytes.IndexByte(b, '\n'); i >= 0 {
					b = b[i+1:]
				}
				return b
			}
		}
	}
	b := make([]byte, 0, 8192)
	buf := make([]byte, 8192)
	for {
		n, err := f.Read(buf)
		b = append(b, buf[:n]...)
		if len(b) > 2*logTailBytes {
			b = b[len(b)-logTailBytes:]
		}
		if err != nil {
			break
		}
	}
	if len(b) > logTailBytes {
		b = b[len(b)-logTailBytes:]
	}
	return b
}

func readFull(f fs.File, b []byte) (int, error) {
	total := 0
	for total < len(b) {
		n, err := f.Read(b[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

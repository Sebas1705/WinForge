package storage

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// VirtualDisk is a disk image that WSL or Docker Desktop keeps. They grow as files are written and do not
// shrink when files are deleted, so they are often far bigger than what is inside. WinForge only lists
// them: compacting one means shutting WSL down, which would end the person's running sessions.
type VirtualDisk struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"` // the distribution, or "Docker Desktop"
	Kind      string    `json:"kind"` // "wsl" or "docker"
	SizeBytes int64     `json:"sizeBytes"`
	Modified  time.Time `json:"modified"`
}

// VirtualDisks lists the disk images WSL distributions and Docker Desktop keep for this user, biggest first.
func VirtualDisks(e Env) []VirtualDisk {
	if e.LocalAppData == "" {
		return []VirtualDisk{}
	}
	var patterns []struct{ glob, kind string }
	patterns = append(patterns,
		struct{ glob, kind string }{filepath.Join(e.LocalAppData, "Docker", "wsl", "*", "*.vhdx"), "docker"},
		struct{ glob, kind string }{filepath.Join(e.LocalAppData, "Packages", "*", "LocalState", "*.vhdx"), "wsl"},
		struct{ glob, kind string }{filepath.Join(e.LocalAppData, "wsl", "*", "*.vhdx"), "wsl"},
	)
	out := []VirtualDisk{}
	seen := map[string]bool{}
	for _, p := range patterns {
		matches, err := filepath.Glob(p.glob)
		if err != nil {
			continue
		}
		for _, m := range matches {
			st, err := os.Lstat(m)
			if err != nil || !st.Mode().IsRegular() || seen[strings.ToLower(m)] {
				continue
			}
			seen[strings.ToLower(m)] = true
			out = append(out, VirtualDisk{Path: m, Name: diskName(e, m, p.kind), Kind: p.kind, SizeBytes: st.Size(), Modified: st.ModTime()})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SizeBytes != out[j].SizeBytes {
			return out[i].SizeBytes > out[j].SizeBytes
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// diskName makes a readable name: Docker Desktop, or the store package a distribution came from.
func diskName(e Env, path, kind string) string {
	if kind == "docker" {
		return "Docker Desktop"
	}
	rel, err := filepath.Rel(filepath.Join(e.LocalAppData, "Packages"), path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		pkg, _, _ := strings.Cut(rel, string(filepath.Separator))
		// "CanonicalGroupLimited.Ubuntu22.04LTS_79rhkp1fndgsc" -> "Ubuntu22.04LTS"
		name, _, _ := strings.Cut(pkg, "_")
		if _, after, ok := strings.Cut(name, "."); ok && after != "" {
			return after
		}
		return name
	}
	return filepath.Base(filepath.Dir(path))
}

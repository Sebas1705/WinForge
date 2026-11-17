package storage

import "path/filepath"

// CommandsFor lists, in order, the exact commands Clean runs for a category that is cleaned by
// Windows' own tools (the Recycle Bin, Windows.old and the two Docker categories). Each command is
// the program followed by its arguments. It returns nil for a category that is a list of folders
// and for an unknown id. It exists so a plan can state exactly what will run; a test checks that
// it matches what Clean really runs.
func CommandsFor(id string, env Env) [][]string {
	switch id {
	case "recycle-bin":
		return [][]string{{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
			"Clear-RecycleBin -Force -ErrorAction SilentlyContinue"}}
	case "docker-build-cache":
		return [][]string{{"docker", "builder", "prune", "--all", "--force"}}
	case "docker-dangling":
		return [][]string{{"docker", "image", "prune", "--force"}}
	case "windows-old":
		r := Options{Env: env}.windowsOld()
		if r == "" {
			return nil
		}
		return [][]string{
			{"takeown.exe", "/F", r, "/R", "/D", "Y"},
			{"icacls.exe", r, "/grant", "*S-1-5-32-544:F", "/T", "/C", "/Q"},
			{"cmd.exe", "/C", "rd", "/s", "/q", r},
		}
	}
	return nil
}

// Folders returns the folders (or folder patterns, where a "*" stands for one folder name) whose
// contents a folder-based category removes, for the given PC layout. It is nil for the
// command-based categories and for an unknown id.
func Folders(id string, env Env) []string {
	s, ok := SpecFor(id)
	if !ok || s.Dirs == nil {
		return nil
	}
	var out []string
	for _, d := range s.Dirs(env) {
		if d != "" {
			out = append(out, filepath.Clean(d))
		}
	}
	return out
}

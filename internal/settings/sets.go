// Package settings backs up and restores the configuration of common apps
// (editor, terminal, git...) so a reinstalled PC feels like the old one. It
// only touches an explicit list of files: nothing outside it is ever read from
// the PC or written to it, and secrets (SSH keys, tokens) are not on it.
package settings

import "sort"

// Roots are the folders a Path is relative to. They are resolved per PC, which
// is why a backup made on one account restores on another.
const (
	RootAppData      = "appdata"
	RootLocalAppData = "localappdata"
	RootHome         = "home"
	RootDocuments    = "documents"
)

// Path is a file, or a folder whose files are all included, under a root.
type Path struct {
	Root string
	Rel  string // forward slashes
	Dir  bool
}

// Set is one app's settings.
type Set struct {
	ID    string
	Paths []Path
	// Extensions, when set, is the command that lists installed editor
	// extensions; they are reinstalled on restore rather than copied.
	Extensions *ExtensionCmd
}

// ExtensionCmd lists and installs editor extensions through the editor's CLI.
type ExtensionCmd struct {
	Exe     string   // "code"
	List    []string // arguments that print one extension id per line
	Install []string // arguments before the id
}

// Sets is the whole list. Adding an app is adding an entry here; the files
// are never discovered by scanning the PC.
var Sets = []Set{
	{ID: "vscode", Paths: []Path{
		{RootAppData, "Code/User/settings.json", false},
		{RootAppData, "Code/User/keybindings.json", false},
		{RootAppData, "Code/User/snippets", true},
	}, Extensions: &ExtensionCmd{Exe: "code", List: []string{"--list-extensions"}, Install: []string{"--install-extension"}}},
	{ID: "git", Paths: []Path{
		{RootHome, ".gitconfig", false},
		{RootHome, ".gitignore_global", false},
	}},
	{ID: "terminal", Paths: []Path{
		{RootLocalAppData, "Packages/Microsoft.WindowsTerminal_8wekyb3d8bbwe/LocalState/settings.json", false},
		{RootLocalAppData, "Packages/Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe/LocalState/settings.json", false},
	}},
	{ID: "powershell", Paths: []Path{
		{RootDocuments, "PowerShell/Microsoft.PowerShell_profile.ps1", false},
		{RootDocuments, "WindowsPowerShell/Microsoft.PowerShell_profile.ps1", false},
	}},
	// Only the config file: never keys, never known_hosts.
	{ID: "ssh", Paths: []Path{{RootHome, ".ssh/config", false}}},
	{ID: "notepadpp", Paths: []Path{
		{RootAppData, "Notepad++/config.xml", false},
		{RootAppData, "Notepad++/shortcuts.xml", false},
		{RootAppData, "Notepad++/stylers.xml", false},
		{RootAppData, "Notepad++/themes", true},
		{RootAppData, "Notepad++/userDefineLangs", true},
	}},
	{ID: "vlc", Paths: []Path{{RootAppData, "vlc/vlcrc", false}}},
	{ID: "winget", Paths: []Path{
		{RootLocalAppData, "Packages/Microsoft.DesktopAppInstaller_8wekyb3d8bbwe/LocalState/settings.json", false},
	}},
}

// IDs lists every set id, sorted.
func IDs() []string {
	out := make([]string, 0, len(Sets))
	for _, s := range Sets {
		out = append(out, s.ID)
	}
	sort.Strings(out)
	return out
}

func find(id string) *Set {
	for i := range Sets {
		if Sets[i].ID == id {
			return &Sets[i]
		}
	}
	return nil
}

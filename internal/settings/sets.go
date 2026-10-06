// Package settings backs up and restores the configuration of common apps
// (editors, terminals, git...) so a reinstalled PC feels like the old one. It
// only touches an explicit list of files: nothing outside it is ever read from
// the PC or written to it, and secrets (SSH keys, tokens, saved logins,
// browser profiles) are not on it.
package settings

import (
	"path"
	"sort"
	"strings"
)

// Roots are the folders a Path is relative to. They are resolved per PC, which
// is why a backup made on one account restores on another.
const (
	RootAppData      = "appdata"
	RootLocalAppData = "localappdata"
	RootHome         = "home"
	RootDocuments    = "documents"
)

// Path is a file, or a folder whose files are all included, under a root. A
// segment may be "*" (any one name, for example a versioned folder such as
// JetBrains/IntelliJIdea2025.1).
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

func vscodeLike(id, folder, exe string) Set {
	return Set{ID: id, Paths: []Path{
		{RootAppData, folder + "/User/settings.json", false},
		{RootAppData, folder + "/User/keybindings.json", false},
		{RootAppData, folder + "/User/snippets", true},
	}, Extensions: &ExtensionCmd{Exe: exe, List: []string{"--list-extensions"}, Install: []string{"--install-extension"}}}
}

// Sets is the whole list. Adding an app is adding an entry here; the files
// are never discovered by scanning the PC. Nothing that holds a login (browser
// profiles, chat apps, SSH keys, CLI tokens, kube configs) belongs here.
var Sets = []Set{
	vscodeLike("vscode", "Code", "code"),
	vscodeLike("vscode-insiders", "Code - Insiders", "code-insiders"),
	vscodeLike("vscodium", "VSCodium", "codium"),
	vscodeLike("cursor", "Cursor", "cursor"),
	{ID: "jetbrains", Paths: []Path{
		{RootAppData, "JetBrains/*/options", true},
		{RootAppData, "JetBrains/*/keymaps", true},
		{RootAppData, "JetBrains/*/codestyles", true},
		{RootAppData, "JetBrains/*/colors", true},
	}},
	{ID: "sublime", Paths: []Path{{RootAppData, "Sublime Text/Packages/User", true}}},
	{ID: "vim", Paths: []Path{
		{RootHome, ".vimrc", false},
		{RootHome, "_vimrc", false},
		{RootHome, ".ideavimrc", false},
		{RootLocalAppData, "nvim", true},
	}},
	{ID: "git", Paths: []Path{
		{RootHome, ".gitconfig", false},
		{RootHome, ".gitignore_global", false},
		{RootHome, ".config/git/config", false},
		{RootHome, ".config/git/ignore", false},
	}},
	{ID: "githubcli", Paths: []Path{{RootAppData, "GitHub CLI/config.yml", false}}}, // never hosts.yml: it holds the token
	{ID: "terminal", Paths: []Path{
		{RootLocalAppData, "Packages/Microsoft.WindowsTerminal_8wekyb3d8bbwe/LocalState/settings.json", false},
		{RootLocalAppData, "Packages/Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe/LocalState/settings.json", false},
	}},
	{ID: "alacritty", Paths: []Path{
		{RootAppData, "alacritty/alacritty.toml", false},
		{RootAppData, "alacritty/alacritty.yml", false},
	}},
	{ID: "wezterm", Paths: []Path{
		{RootHome, ".wezterm.lua", false},
		{RootHome, ".config/wezterm", true},
	}},
	{ID: "powershell", Paths: []Path{
		{RootDocuments, "PowerShell/Microsoft.PowerShell_profile.ps1", false},
		{RootDocuments, "PowerShell/powershell.config.json", false},
		{RootDocuments, "WindowsPowerShell/Microsoft.PowerShell_profile.ps1", false},
	}},
	{ID: "shell", Paths: []Path{
		{RootHome, ".bashrc", false},
		{RootHome, ".bash_profile", false},
		{RootHome, ".profile", false},
		{RootHome, ".zshrc", false},
		{RootHome, ".config/starship.toml", false},
	}},
	// Only the config file: never keys, never known_hosts.
	{ID: "ssh", Paths: []Path{{RootHome, ".ssh/config", false}}},
	{ID: "docker", Paths: []Path{
		{RootAppData, "Docker/settings.json", false},
		{RootAppData, "Docker/settings-store.json", false},
	}},
	{ID: "wsl", Paths: []Path{{RootHome, ".wslconfig", false}}},
	{ID: "cargo", Paths: []Path{{RootHome, ".cargo/config.toml", false}}},
	{ID: "notepadpp", Paths: []Path{
		{RootAppData, "Notepad++/config.xml", false},
		{RootAppData, "Notepad++/shortcuts.xml", false},
		{RootAppData, "Notepad++/stylers.xml", false},
		{RootAppData, "Notepad++/themes", true},
		{RootAppData, "Notepad++/userDefineLangs", true},
	}},
	{ID: "powertoys", Paths: []Path{
		{RootLocalAppData, "Microsoft/PowerToys/settings.json", false},
		{RootLocalAppData, "Microsoft/PowerToys/*/settings.json", false},
	}},
	{ID: "flowlauncher", Paths: []Path{{RootAppData, "FlowLauncher/Settings/Settings.json", false}}},
	{ID: "altsnap", Paths: []Path{{RootAppData, "AltSnap/AltSnap.ini", false}}},
	{ID: "autohotkey", Paths: []Path{{RootDocuments, "AutoHotkey", true}}},
	{ID: "sharex", Paths: []Path{
		{RootDocuments, "ShareX/ApplicationConfig.json", false},
		{RootDocuments, "ShareX/HotkeysConfig.json", false},
	}},
	{ID: "obs", Paths: []Path{{RootAppData, "obs-studio/basic/scenes", true}}}, // scenes only: profiles hold the stream key
	{ID: "vlc", Paths: []Path{{RootAppData, "vlc/vlcrc", false}}},
	{ID: "mpv", Paths: []Path{{RootAppData, "mpv", true}}},
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

// matches reports whether rel (a concrete relative path, forward slashes) is
// covered by the pattern: a file path equal to it, or any file under a folder
// path. Each "*" stands for exactly one path segment.
func (p Path) matches(rel string) bool {
	pat := strings.Split(p.Rel, "/")
	got := strings.Split(rel, "/")
	if p.Dir {
		if len(got) <= len(pat) {
			return false
		}
	} else if len(got) != len(pat) {
		return false
	}
	for i, seg := range pat {
		if ok, err := path.Match(seg, got[i]); err != nil || !ok {
			return false
		}
	}
	return true
}

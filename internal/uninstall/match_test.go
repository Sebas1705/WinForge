package uninstall_test

import (
	"testing"

	"github.com/Sebas1705/WinForge/internal/uninstall"
)

var (
	git     = uninstall.App{ID: "git", Name: "Git", Publisher: "The Git Development Community", Winget: "Git.Git"}
	vscode  = uninstall.App{ID: "vscode", Name: "Visual Studio Code", Publisher: "Microsoft Corporation", Winget: "Microsoft.VisualStudioCode"}
	sevenZ  = uninstall.App{ID: "7zip", Name: "7-Zip", Publisher: "Igor Pavlov", Winget: "7zip.7zip"}
	npp     = uninstall.App{ID: "npp", Name: "Notepad++", Publisher: "Notepad++ Team", Winget: "Notepad++.Notepad++"}
	ghcli   = uninstall.App{ID: "gh", Name: "GitHub CLI", Publisher: "GitHub, Inc.", Winget: "GitHub.cli"}
	firefox = uninstall.App{ID: "firefox", Name: "Mozilla Firefox", Publisher: "Mozilla", Winget: "Mozilla.Firefox"}
	golang  = uninstall.App{ID: "go", Name: "Go", Publisher: "Google", Winget: "GoLang.Go"}
	foo     = uninstall.App{ID: "foo", Name: "Foo Tool", Publisher: "Acme", Winget: "Acme.cli"}
)

type matchCase struct {
	app  uninstall.App
	name string
	want uninstall.Rule // "" means no match
}

// Every row says what a folder or shortcut called `name` is to the app. The "" rows are the near misses
// the rules exist to refuse.
var matchTable = []matchCase{
	// Git: exact, case, trademark and spacing only.
	{git, "Git", uninstall.RuleName},
	{git, "git", uninstall.RuleName},
	{git, "GIT", uninstall.RuleName},
	{git, "  Git  ", uninstall.RuleName},
	{git, "Git®", uninstall.RuleName},
	{git, "Git.Git", uninstall.RuleWingetID},
	{git, "The Git Development Community Git", uninstall.RulePublisherName},
	{git, "digital", ""},
	{git, "Digital Editions", ""},
	{git, "GitHub", ""},
	{git, "GitHub Desktop", ""},
	{git, "Git LFS", ""},
	{git, "GitKraken", ""},
	{git, "Gitea", ""},
	{git, "Git2", ""},
	{git, "G i t", ""},
	{git, "git-lfs", ""},
	{git, "legit", ""},
	{git, "Git Gui", ""},
	{git, "The Git Development Community", ""}, // the publisher alone is a company, not an app
	{git, "", ""},
	{git, "   ", ""},
	{git, ".git", ""},
	{git, "Git.Git.bak", ""},

	// Visual Studio Code: the publisher alone and look-alikes never match.
	{vscode, "Visual Studio Code", uninstall.RuleName},
	{vscode, "visual studio code", uninstall.RuleName},
	{vscode, "VisualStudioCode", uninstall.RuleName}, // separators are ignored, so this is the name written without spaces
	{vscode, "Microsoft.VisualStudioCode", uninstall.RulePublisherName},
	{vscode, "Microsoft Visual Studio Code", uninstall.RulePublisherName},
	{vscode, "Microsoft Corporation Visual Studio Code", uninstall.RulePublisherName},
	{vscode, "Microsoft", ""},
	{vscode, "Microsoft Corporation", ""},
	{vscode, "Windows", ""},
	{vscode, "Visual Studio", ""},
	{vscode, "Visual Studio Code Insiders", ""},
	{vscode, "Code", ""},
	{vscode, "Visual", ""},
	{vscode, "Studio", ""},
	{vscode, "Microsoft VS Code", ""},

	// 7-Zip: separators are written every way, but short or different names stay different.
	{sevenZ, "7-Zip", uninstall.RuleName},
	{sevenZ, "7zip", uninstall.RuleName},
	{sevenZ, "7 Zip", uninstall.RuleName},
	{sevenZ, "7_zip", uninstall.RuleName},
	{sevenZ, "7z", ""},
	{sevenZ, "zip", ""},
	{sevenZ, "7-Zip Portable", ""},
	{sevenZ, "p7zip", ""},

	// Notepad++: the plus signs are part of the name.
	{npp, "Notepad++", uninstall.RuleName},
	{npp, "notepad++", uninstall.RuleName},
	{npp, "Notepad", ""},
	{npp, "Notepad+", ""},
	{npp, "Notepad+++", ""},

	// GitHub CLI: "cli" is too generic to be a package match on its own.
	{ghcli, "GitHub CLI", uninstall.RuleName},
	{ghcli, "GitHub.cli", uninstall.RuleName},
	{ghcli, "cli", ""},
	{ghcli, "CLI", ""},
	{ghcli, "GitHub", ""},
	{ghcli, "GitHub Desktop", ""},
	{ghcli, "GitHub, Inc.", ""},

	// A publisher's own name inside the app's name counts once, as the name.
	{firefox, "Mozilla Firefox", uninstall.RuleName},
	{firefox, "Firefox", uninstall.RuleWingetPackage},
	{firefox, "Mozilla", ""},
	{firefox, "Mozilla Thunderbird", ""},
	{firefox, "Firefox Developer Edition", ""},

	// Names of one or two letters only match when written exactly alike.
	{golang, "Go", uninstall.RuleName},
	{golang, "go", uninstall.RuleName},
	{golang, "g.o", ""},
	{golang, "G-o", ""},
	{golang, "Google", ""},
	{golang, "Gopher", ""},
	{golang, "GoLang.Go", uninstall.RuleWingetID},

	// A package part that is a generic word never matches by itself.
	{foo, "Foo Tool", uninstall.RuleName},
	{foo, "cli", ""},
	{foo, "Acme.cli", uninstall.RuleWingetID},
	{foo, "Acme", ""},
}

func TestMatchTable(t *testing.T) {
	for _, c := range matchTable {
		why, ok := uninstall.Match(c.app, c.name)
		switch {
		case c.want == "" && ok:
			t.Errorf("%s: %q must not match, but matched by %s (%q)", c.app.ID, c.name, why.Rule, why.Value)
		case c.want != "" && !ok:
			t.Errorf("%s: %q must match by %s, but did not", c.app.ID, c.name, c.want)
		case c.want != "" && why.Rule != c.want:
			t.Errorf("%s: %q matched by %s, want %s", c.app.ID, c.name, why.Rule, c.want)
		}
	}
}

func TestMatchNested(t *testing.T) {
	cases := []struct {
		app           uninstall.App
		parent, child string
		want          bool
	}{
		{firefox, "Mozilla", "Firefox", true},
		{firefox, "mozilla", "firefox", true},
		{firefox, "Mozilla", "Mozilla Firefox", true},
		{ghcli, "GitHub", "GitHub CLI", true}, // the publisher "GitHub, Inc." without its suffix
		{ghcli, "GitHub, Inc.", "GitHub CLI", true},
		{ghcli, "GitHub", "cli", false},
		{firefox, "Mozilla", "Firefox Developer Edition", false},
		{firefox, "Mozilla", "Thunderbird", false},
		{firefox, "Other", "Firefox", false},
		{firefox, "Moz", "Firefox", false},
		{firefox, "Mozilla Foundation", "Firefox", false},
		{vscode, "Microsoft", "Visual Studio Code", false}, // Microsoft's folder holds parts of Windows
		{vscode, "Microsoft Corporation", "Visual Studio Code", false},
		{uninstall.App{Name: "Edge", Publisher: "Microsoft", Winget: "Microsoft.Edge"}, "Microsoft", "Edge", false},
		{uninstall.App{Name: "Thing", Publisher: "Windows", Winget: "Windows.Thing"}, "Windows", "Thing", false},
		{uninstall.App{Name: "Thing", Publisher: "Common Files", Winget: "Acme.Thing"}, "Common Files", "Thing", false},
		{uninstall.App{Name: "Thing", Winget: "Thing"}, "", "Thing", false},
	}
	for _, c := range cases {
		why, ok := uninstall.MatchNested(c.app, c.parent, c.child)
		if ok != c.want {
			t.Errorf("%q\\%q for %s: got %v, want %v", c.parent, c.child, c.app.ID, ok, c.want)
		}
		if ok && why.Rule != uninstall.RuleNested {
			t.Errorf("%q\\%q: rule %s, want nested", c.parent, c.child, why.Rule)
		}
	}
}

func TestSystemNamesAreNeverOffered(t *testing.T) {
	for _, n := range []string{"Windows", "windows", "Microsoft", "Common Files", "WindowsApps", "Windows NT", "Windows Defender",
		"WindowsPowerShell", "Program Files", "Packages", "Temp", "Programs", "Users", "Microsoft Edge", "dotnet", "Microsoft Shared"} {
		if !uninstall.IsSystemName(n) {
			t.Errorf("%q must be a protected name", n)
		}
	}
	for _, n := range []string{"Git", "Visual Studio Code", "Windows Terminal", "PowerToys", ""} {
		if uninstall.IsSystemName(n) {
			t.Errorf("%q must not be a protected name", n)
		}
	}
}

func TestInstallSizePicksTheBiggestMatchingEntry(t *testing.T) {
	regs := []uninstall.RegApp{
		{Name: "Git version 2.50", SizeKB: 300},
		{Name: "Git LFS", SizeKB: 5000},
		{Name: "Other", SizeKB: 999999},
		{Name: "Git version 2.49", SizeKB: 0},
	}
	if got := uninstall.InstallSize([]string{`^Git version`}, regs); got != 300*1024 {
		t.Fatalf("got %d", got)
	}
	if got := uninstall.InstallSize([]string{`(`, `^Git LFS`}, regs); got != 5000*1024 { // a broken pattern is skipped
		t.Fatalf("got %d", got)
	}
	if got := uninstall.InstallSize(nil, regs); got != 0 {
		t.Fatalf("got %d", got)
	}
}

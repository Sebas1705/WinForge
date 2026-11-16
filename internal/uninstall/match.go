// Package uninstall finds what an uninstalled catalog app left behind and moves it, on request, to the
// Recycle Bin.
//
// The search is deliberately narrow. It looks only in a fixed list of places (see Env.Places), only at the
// top level of each (one level deeper for "Publisher\Name"), and only for names that are exactly the app's
// name, its publisher and name together, or the parts of its winget id. It never matches part of a name, a
// Windows folder or anything that belongs to another catalog app that is still installed, and it never
// lists a file of the person's other than a shortcut to the removed app. Nothing here deletes: removal is a
// move to the Recycle Bin, one folder or shortcut at a time, each proved again at the moment it moves.
//
// The logic is pure; touching the disk and the registry goes through System, which a real implementation
// and an in-memory fake (in the tests) both satisfy.
package uninstall

import (
	"strings"
	"unicode"
)

// App is what the search knows about a catalog app: nothing but catalog data.
type App struct {
	ID        string
	Name      string
	Publisher string
	Winget    string
}

// Rule says why a name matched an app. The interface turns it into a sentence in the person's language.
type Rule string

const (
	// RuleName: the name is the app's name.
	RuleName Rule = "name"
	// RulePublisherName: the name is the publisher followed by the app's name.
	RulePublisherName Rule = "publisherName"
	// RuleNested: a folder named like the app inside a folder named like its publisher.
	RuleNested Rule = "nested"
	// RuleWingetID: the name is the whole winget id, such as "Git.Git".
	RuleWingetID Rule = "wingetId"
	// RuleWingetPackage: the name is the package part of the winget id (what follows the publisher).
	RuleWingetPackage Rule = "wingetPackage"
)

// Why is the reason a candidate was listed: which rule, and the text of the app it was compared with.
type Why struct {
	Rule  Rule   `json:"rule"`
	Value string `json:"value"`
}

// rank orders rules from the most to the least certain, so a name that fits several is explained by the best.
var rank = map[Rule]int{RuleName: 0, RulePublisherName: 1, RuleNested: 2, RuleWingetID: 3, RuleWingetPackage: 4}

// fold is how names are compared: case-insensitive, trademark signs dropped, spaces collapsed. Nothing else
// is changed, so "Notepad++" stays different from "Notepad".
func fold(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '®', '™', '©', ' ':
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// separators are the characters that vendors write inconsistently ("7-Zip", "7zip", "7 Zip").
const separators = " -_."

func compact(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(separators, r) {
			return -1
		}
		return r
	}, fold(s))
}

// minCompact is the shortest name that may match with separators ignored. Shorter names ("Go", "R") must
// be written exactly alike, so "go" cannot reach "g.o".
const minCompact = 4

// same says whether two names are equal. Never a prefix, a suffix or a substring: "git" is not "digital",
// "GitHub" and "Git" are two different apps.
func same(a, b string) bool {
	fa, fb := fold(a), fold(b)
	if fa == "" || fb == "" {
		return false
	}
	if fa == fb {
		return true
	}
	ca, cb := compact(a), compact(b)
	return ca == cb && len([]rune(ca)) >= minCompact
}

// companySuffixes are dropped from the end of a publisher's name, so "GitHub, Inc." is "GitHub".
var companySuffixes = map[string]bool{
	"inc": true, "inc.": true, "llc": true, "ltd": true, "ltd.": true, "limited": true, "corp": true, "corp.": true,
	"corporation": true, "co": true, "co.": true, "company": true, "gmbh": true, "ag": true, "ab": true, "ev": true,
	"e.v.": true, "s.r.o.": true, "sro": true, "foundation": true, "team": true, "group": true, "bv": true, "b.v.": true,
}

// shortPublisher drops commas and company suffixes from a publisher's name.
func shortPublisher(p string) string {
	words := strings.Fields(strings.ReplaceAll(fold(p), ",", " "))
	for len(words) > 1 && companySuffixes[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	return strings.Join(words, " ")
}

// wingetParts splits "Publisher.Package.More" into its publisher and the rest. An id with no dot has no
// publisher part.
func wingetParts(id string) (publisher, pkg string) {
	i := strings.Index(id, ".")
	if i <= 0 || i == len(id)-1 {
		return "", id
	}
	return id[:i], id[i+1:]
}

// genericPackages are package names too common to mean one app on their own: a folder called "cli" or
// "desktop" says nothing about which app made it. They can still match through the app's own name.
var genericPackages = map[string]bool{
	"cli": true, "app": true, "desktop": true, "client": true, "core": true, "setup": true, "installer": true,
	"launcher": true, "studio": true, "portable": true, "stable": true, "community": true, "server": true,
	"tools": true, "tool": true, "manager": true, "player": true, "editor": true, "browser": true, "sdk": true, "jdk": true,
}

// pattern is one way a folder or shortcut name can match an app.
type pattern struct {
	text string
	why  Why
}

// patterns lists every name that counts as this app, with the reason. The publisher alone is never one of
// them: it names a company, not an app.
func patterns(a App) []pattern {
	var out []pattern
	add := func(text string, r Rule, value string) {
		if fold(text) != "" {
			out = append(out, pattern{text, Why{r, value}})
		}
	}
	add(a.Name, RuleName, a.Name)
	if a.Publisher != "" && a.Name != "" {
		for _, p := range uniq(a.Publisher, shortPublisher(a.Publisher)) {
			add(p+" "+a.Name, RulePublisherName, a.Publisher+" "+a.Name)
		}
	}
	if a.Winget != "" {
		add(a.Winget, RuleWingetID, a.Winget)
		_, pkg := wingetParts(a.Winget)
		if pkg != a.Winget && !genericPackages[compact(pkg)] && len([]rune(compact(pkg))) >= 3 {
			add(pkg, RuleWingetPackage, pkg)
		}
	}
	return out
}

func uniq(a, b string) []string {
	if fold(a) == fold(b) {
		return []string{a}
	}
	return []string{a, b}
}

// Match says whether a folder or shortcut name belongs to the app, and why.
func Match(a App, name string) (Why, bool) {
	best := Why{}
	found := false
	for _, p := range patterns(a) {
		if !same(p.text, name) {
			continue
		}
		if !found || rank[p.why.Rule] < rank[best.Rule] {
			best, found = p.why, true
		}
	}
	return best, found
}

// protectedVendors are folders that hold parts of Windows or of many programs at once: a folder inside
// one of them is never offered as "Publisher\Name", whatever its name.
var protectedVendors = map[string]bool{
	"microsoft": true, "windows": true, "windows nt": true, "common files": true, "microsoft shared": true,
	"packages": true, "programs": true, "apps": true, "windowsapps": true, "microsoft.net": true,
}

func protectedVendor(name string) bool {
	return protectedVendors[fold(name)] || protectedVendors[shortPublisher(name)]
}

// MatchNested says whether a folder named child, inside a folder named parent, is "Publisher\Name" of the app.
func MatchNested(a App, parent, child string) (Why, bool) {
	if a.Publisher == "" || protectedVendor(parent) {
		return Why{}, false
	}
	if !same(a.Publisher, parent) && !same(shortPublisher(a.Publisher), parent) {
		return Why{}, false
	}
	for _, p := range patterns(a) {
		if p.why.Rule != RuleName && p.why.Rule != RuleWingetPackage {
			continue
		}
		if same(p.text, child) {
			return Why{RuleNested, a.Publisher + "\\" + p.why.Value}, true
		}
	}
	return Why{}, false
}

// systemNames are folder and shortcut names that are Windows's own or belong to every program: they are
// never offered, even if an app were called that.
var systemNames = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range []string{
		"Windows", "Windows NT", "Windows Defender", "Windows Defender Advanced Threat Protection", "Windows Mail",
		"Windows Media Player", "Windows Photo Viewer", "Windows Portable Devices", "Windows Security", "Windows Sidebar",
		"WindowsApps", "WindowsPowerShell", "ModifiableWindowsApps", "Internet Explorer", "Microsoft", "Microsoft Shared",
		"Microsoft.NET", "Common Files", "Reference Assemblies", "MSBuild", "dotnet", "Packages", "PackageCache",
		"Package Cache", "Programs", "Program Files", "Program Files (x86)", "ProgramData", "Users", "Default", "Public",
		"Temp", "Tmp", "Local", "LocalLow", "Roaming", "Microsoft Edge", "Microsoft Edge Update", "EdgeWebView",
		"Application Data", "Desktop", "Documents", "Downloads", "Pictures", "Music", "Videos", "OneDrive", "Startup",
		"Start Menu", "Accessories", "Administrative Tools", "System Tools", "Windows Kits", "Windows Accessories",
		"Windows PowerShell", "Windows System", "Windows Ease of Access", "Maintenance", "Uninstall Information",
		"Installer", "InstallShield Installation Information", "System Volume Information", "$Recycle.Bin", "Recovery",
		"Intel", "AMD", "NVIDIA", "NVIDIA Corporation", "Realtek", "Drivers", "DriverStore", "WinSxS", "Fonts",
	} {
		m[fold(n)] = true
	}
	return m
}()

// IsSystemName reports whether a name is one of the protected ones.
func IsSystemName(name string) bool { return systemNames[fold(name)] }

// shortcutName is the name a shortcut shows: its file name without the .lnk. It reports false for anything
// that is not a .lnk file.
func shortcutName(file string) (string, bool) {
	if len(file) <= 4 || !strings.EqualFold(file[len(file)-4:], ".lnk") {
		return "", false
	}
	return strings.TrimFunc(file[:len(file)-4], unicode.IsSpace), true
}

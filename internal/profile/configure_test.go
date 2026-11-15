package profile_test

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/profile"
)

type configFile struct {
	Properties struct {
		Resources []struct {
			Resource   string `yaml:"resource"`
			ID         string `yaml:"id"`
			Directives struct {
				Description     string `yaml:"description"`
				AllowPrerelease bool   `yaml:"allowPrerelease"`
			} `yaml:"directives"`
			Settings struct {
				ID      string `yaml:"id"`
				Source  string `yaml:"source"`
				Version string `yaml:"version"`
			} `yaml:"settings"`
		} `yaml:"resources"`
		ConfigurationVersion string `yaml:"configurationVersion"`
	} `yaml:"properties"`
}

func parse(t *testing.T, b []byte) configFile {
	t.Helper()
	var f configFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		t.Fatalf("the output is not valid YAML: %v\n%s", err, b)
	}
	return f
}

func configCatalog() *catalog.Catalog {
	return &catalog.Catalog{Apps: map[string]*catalog.App{
		"git":      {ID: "git", Name: "Git", Winget: "Git.Git"},
		"vscode":   {ID: "vscode", Name: "Visual Studio Code", Winget: "Microsoft.VisualStudioCode"},
		"net8":     {ID: "net8", Name: ".NET SDK 8", Winget: "Microsoft.DotNet.SDK.8"},
		"a.b":      {ID: "a.b", Name: "Dotted", Winget: "Vendor.Dotted"},
		"a-b":      {ID: "a-b", Name: "Dashed", Winget: "Vendor.Dashed"},
		"quirky":   {ID: "quirky", Name: `Say "hi" \ # now: {x}`, Winget: "Vendor.Quirky"},
		"c++tools": {ID: "c++tools", Name: "C++ Tools", Winget: "Vendor.CppTools"},
	}}
}

func TestConfigureListsEveryAppAsAWinGetPackage(t *testing.T) {
	cat := configCatalog()
	b, err := profile.WingetConfigure("Rust setup", []catalog.ProfileApp{{ID: "git"}, {ID: "vscode", Version: "1.99.0"}, {ID: "net8"}}, nil, cat)
	if err != nil {
		t.Fatal(err)
	}
	f := parse(t, b)
	if f.Properties.ConfigurationVersion != "0.2.0" || len(f.Properties.Resources) != 3 {
		t.Fatalf("%s", b)
	}
	for i, want := range []struct{ id, winget, version, desc string }{
		{"git", "Git.Git", "", "Install Git"},
		{"vscode", "Microsoft.VisualStudioCode", "1.99.0", "Install Visual Studio Code"},
		{"net8", "Microsoft.DotNet.SDK.8", "", "Install .NET SDK 8"},
	} {
		r := f.Properties.Resources[i]
		if r.Resource != "Microsoft.WinGet.DSC/WinGetPackage" || r.ID != want.id || r.Settings.ID != want.winget || r.Settings.Source != "winget" ||
			r.Settings.Version != want.version || r.Directives.Description != want.desc || !r.Directives.AllowPrerelease {
			t.Errorf("resource %d: %+v", i, r)
		}
	}
	text := string(b)
	if !strings.HasPrefix(text, "# yaml-language-server: $schema=https://aka.ms/configuration-dsc-schema/0.2\n") {
		t.Fatalf("the schema line comes first so editors can validate: %.80s", text)
	}
	if !strings.Contains(text, "winget configure -f") || !strings.Contains(text, "read it before you apply it") {
		t.Fatal("the file must say how to apply it and to read it first")
	}
}

func TestOnlyAPinnedAppCarriesAVersion(t *testing.T) {
	b, _ := profile.WingetConfigure("x", []catalog.ProfileApp{{ID: "git"}}, nil, configCatalog())
	if strings.Contains(string(b), "version:") {
		t.Fatalf("an unpinned app must follow the latest version:\n%s", b)
	}
}

func TestEveryValueIsQuotedSoNothingCanBreakOut(t *testing.T) {
	cat := configCatalog()
	b, err := profile.WingetConfigure("My \"setup\"\nwith: tricks # and more", []catalog.ProfileApp{{ID: "quirky"}, {ID: "c++tools"}}, nil, cat)
	if err != nil {
		t.Fatal(err)
	}
	f := parse(t, b)
	if len(f.Properties.Resources) != 2 {
		t.Fatalf("%s", b)
	}
	if got := f.Properties.Resources[0].Directives.Description; got != `Install Say "hi" \ # now: {x}` {
		t.Fatalf("the description must come back exactly: %q", got)
	}
	// the profile name sits in a comment on one line: a newline in it must not start a new line
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "tricks") && !strings.HasPrefix(line, "#") {
			t.Fatalf("a name leaked out of its comment: %q", line)
		}
	}
	if f.Properties.Resources[1].ID != "c-tools" { // the id keeps only letters, digits, dash and underscore
		t.Fatalf("resource id: %q", f.Properties.Resources[1].ID)
	}
}

func TestResourceIdsStayUnique(t *testing.T) {
	cat := configCatalog()
	b, err := profile.WingetConfigure("x", []catalog.ProfileApp{{ID: "a.b"}, {ID: "a-b"}}, nil, cat)
	if err != nil {
		t.Fatal(err)
	}
	f := parse(t, b)
	if len(f.Properties.Resources) != 2 || f.Properties.Resources[0].ID == f.Properties.Resources[1].ID {
		t.Fatalf("two apps must not share a resource id:\n%s", b)
	}
	// and the settings still point at the right packages
	if f.Properties.Resources[0].Settings.ID != "Vendor.Dotted" || f.Properties.Resources[1].Settings.ID != "Vendor.Dashed" {
		t.Fatalf("%s", b)
	}
}

func TestRecipesAreNamedButNeverWritten(t *testing.T) {
	b, err := profile.WingetConfigure("x", []catalog.ProfileApp{{ID: "git"}}, []string{"git-defaults", "wsl-default"}, configCatalog())
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "# Left out, because WinForge recipes run scripts: git-defaults, wsl-default") {
		t.Fatalf("the person must be told what was left out:\n%s", text)
	}
	if len(parse(t, b).Properties.Resources) != 1 {
		t.Fatal("recipes must not become resources")
	}
	plain, _ := profile.WingetConfigure("x", []catalog.ProfileApp{{ID: "git"}}, nil, configCatalog())
	if strings.Contains(string(plain), "Left out") {
		t.Fatal("no recipes, no note")
	}
}

func TestAnEmptyProfileIsStillAValidFile(t *testing.T) {
	b, err := profile.WingetConfigure("Empty", nil, nil, configCatalog())
	if err != nil {
		t.Fatal(err)
	}
	f := parse(t, b)
	if len(f.Properties.Resources) != 0 || f.Properties.ConfigurationVersion != "0.2.0" {
		t.Fatalf("%s", b)
	}
}

func TestBadInputIsRefused(t *testing.T) {
	cat := configCatalog()
	if _, err := profile.WingetConfigure("x", []catalog.ProfileApp{{ID: "nope"}}, nil, cat); err == nil {
		t.Fatal("an app the catalog does not know must be refused")
	}
	for _, v := range []string{"1.0 beta", "1.0\"; evil: yes", "a\nb", "1.0'x", ""} {
		_, err := profile.WingetConfigure("x", []catalog.ProfileApp{{ID: "git", Version: v}}, nil, cat)
		if v == "" {
			if err != nil {
				t.Errorf("no version is fine: %v", err)
			}
			continue
		}
		if err == nil {
			t.Errorf("version %q must be refused", v)
		}
	}
}

func TestOutputIsStable(t *testing.T) {
	cat := configCatalog()
	apps := []catalog.ProfileApp{{ID: "git"}, {ID: "vscode", Version: "1.0"}}
	a, _ := profile.WingetConfigure("x", apps, []string{"r"}, cat)
	b, _ := profile.WingetConfigure("x", apps, []string{"r"}, cat)
	if string(a) != string(b) {
		t.Fatal("the same profile must give the same file, so it diffs cleanly in a repository")
	}
}

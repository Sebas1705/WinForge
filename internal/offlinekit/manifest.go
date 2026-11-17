package offlinekit

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/goccy/go-yaml"
)

// maxYAMLBytes bounds a winget manifest: real ones are a few KB, with release notes up to some tens.
const maxYAMLBytes = 2 << 20

// Switches are an installer's command line switches in a winget manifest.
type Switches struct {
	Silent             string `yaml:"Silent"`
	SilentWithProgress string `yaml:"SilentWithProgress"`
	Custom             string `yaml:"Custom"`
}

// ManifestInstaller is one installer of a merged winget manifest, with the package-level defaults filled in.
type ManifestInstaller struct {
	Arch      string
	Type      string
	Scope     string
	Sha256    string
	Silent    string // Silent, or SilentWithProgress when the manifest has no Silent
	Custom    string
	Elevation bool
	Codes     []int
}

// Manifest is what WinForge reads from the manifest `winget download` writes next to the installer.
type Manifest struct {
	ID         string
	Version    string
	Installers []ManifestInstaller
}

type RawInstaller struct {
	Architecture          string   `yaml:"Architecture"`
	InstallerType         string   `yaml:"InstallerType"`
	Scope                 string   `yaml:"Scope"`
	InstallerSha256       string   `yaml:"InstallerSha256"`
	InstallerSwitches     Switches `yaml:"InstallerSwitches"`
	Switches              Switches `yaml:"Switches"` // the name older manifests used
	ElevationRequirement  string   `yaml:"ElevationRequirement"`
	InstallerSuccessCodes []int    `yaml:"InstallerSuccessCodes"`
}

type rawManifest struct {
	PackageIdentifier string         `yaml:"PackageIdentifier"`
	PackageVersion    string         `yaml:"PackageVersion"`
	Defaults          RawInstaller   `yaml:",inline"`
	Installers        []RawInstaller `yaml:"Installers"`
}

// ParseManifest reads a merged winget manifest. Installer-level values win over package-level ones.
func ParseManifest(b []byte) (*Manifest, error) {
	if len(b) > maxYAMLBytes {
		return nil, errors.New("manifest too large")
	}
	var raw rawManifest
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if raw.PackageIdentifier == "" || len(raw.Installers) == 0 {
		return nil, errors.New("manifest: no package or no installers")
	}
	m := &Manifest{ID: raw.PackageIdentifier, Version: literalVersion(b, raw.PackageVersion)}
	for _, in := range raw.Installers {
		mi := ManifestInstaller{
			Arch:   strings.ToLower(pick(in.Architecture, raw.Defaults.Architecture)),
			Type:   strings.ToLower(pick(in.InstallerType, raw.Defaults.InstallerType)),
			Scope:  strings.ToLower(pick(in.Scope, raw.Defaults.Scope)),
			Sha256: strings.ToLower(strings.TrimSpace(in.InstallerSha256)),
			Codes:  in.InstallerSuccessCodes,
		}
		if len(mi.Codes) == 0 {
			mi.Codes = raw.Defaults.InstallerSuccessCodes
		}
		sw := mergeSwitches(raw.Defaults.InstallerSwitches, raw.Defaults.Switches)
		isw := mergeSwitches(in.InstallerSwitches, in.Switches)
		mi.Silent = pick(isw.Silent, isw.SilentWithProgress, sw.Silent, sw.SilentWithProgress)
		mi.Custom = pick(isw.Custom, sw.Custom)
		mi.Elevation = strings.EqualFold(pick(in.ElevationRequirement, raw.Defaults.ElevationRequirement), "elevationRequired")
		m.Installers = append(m.Installers, mi)
	}
	return m, nil
}

var versionLine = regexp.MustCompile(`(?m)^PackageVersion:[ \t]*(?:"([^"\r\n]*)"|'([^'\r\n]*)'|([^\s#]+))`)

// literalVersion is PackageVersion as it is written in the file. A YAML reader turns an unquoted 1.10 into
// the number 1.1, which would be a different version; the text is what the package calls itself.
func literalVersion(b []byte, parsed string) string {
	if m := versionLine.FindSubmatch(b); m != nil {
		for _, g := range m[1:] {
			if len(g) > 0 {
				return string(g)
			}
		}
	}
	return parsed
}

func pick(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func mergeSwitches(a, b Switches) Switches {
	return Switches{
		Silent:             pick(a.Silent, b.Silent),
		SilentWithProgress: pick(a.SilentWithProgress, b.SilentWithProgress),
		Custom:             pick(a.Custom, b.Custom),
	}
}

// Select chooses the installer entry that describes a downloaded file: the one whose pinned hash is the
// file's own. A manifest with a single entry and no pinned hash is taken as it is. When entries pin hashes and
// none is this file's, ok is false: the file is not what the manifest published.
func (m *Manifest) Select(fileSha string) (in ManifestInstaller, ok bool) {
	fileSha = strings.ToLower(fileSha)
	for _, i := range m.Installers {
		if i.Sha256 != "" && i.Sha256 == fileSha {
			return i, true
		}
	}
	if len(m.Installers) == 1 && m.Installers[0].Sha256 == "" {
		return m.Installers[0], true
	}
	return ManifestInstaller{}, false
}

// Class is how an installer can be run.
type Class string

const (
	ClassMSI Class = "msi" // msiexec /i
	ClassEXE Class = "exe" // the installer itself, with silent switches
	ClassNo  Class = ""    // cannot be run safely offline
)

// Classify says how an installer of a winget type and file name is run. Types that are not an installer
// (portable, zip), that need a package manager of their own (msix, appx, msstore, pwa) or are unknown are
// not run: they are listed as "cannot install offline" rather than guessed.
func Classify(typ, file string) Class {
	ext := strings.ToLower(file[strings.LastIndexByte(file, '.')+1:])
	switch strings.ToLower(typ) {
	case "msi", "wix":
		if ext == "msi" {
			return ClassMSI
		}
	case "exe", "nullsoft", "inno", "burn":
		if ext == "exe" {
			return ClassEXE
		}
	}
	return ClassNo
}

// defaultSilent are winget's own silent switches for the installer frameworks it knows.
var defaultSilent = map[string]string{
	"inno":     "/SP- /VERYSILENT /SUPPRESSMSGBOXES /NORESTART",
	"nullsoft": "/S",
	"burn":     "/quiet /norestart",
}

// Command is what to run: a program and its arguments, kept apart.
type Command struct {
	Exe  string
	Args []string
}

// ErrSwitches means the installer is an exe whose silent switches are not known.
var ErrSwitches = errors.New("no silent switches for this installer")

// BuildCommand builds the command that installs path silently. An msi goes through msiexec with /qn
// (its manifest's Custom switches, usually properties, follow); an exe gets the manifest's Silent switches or
// the framework's default, then Custom. The path is one argument and nothing is joined into a string, so a
// path with spaces or quotes cannot change what runs.
func BuildCommand(in Installer, path string) (Command, error) {
	switch Classify(in.Type, in.File) {
	case ClassMSI:
		custom, err := splitSwitches(in.Custom)
		if err != nil {
			return Command{}, err
		}
		args := append([]string{"/i", path, "/qn", "/norestart"}, custom...)
		return Command{Exe: "msiexec", Args: args}, nil
	case ClassEXE:
		silent := in.Silent
		if silent == "" {
			silent = defaultSilent[strings.ToLower(in.Type)]
		}
		if silent == "" {
			return Command{}, ErrSwitches
		}
		sw, err := splitSwitches(silent)
		if err != nil {
			return Command{}, err
		}
		custom, err := splitSwitches(in.Custom)
		if err != nil {
			return Command{}, err
		}
		return Command{Exe: path, Args: append(sw, custom...)}, nil
	}
	return Command{}, errors.New("installer type cannot be run offline")
}

// splitSwitches splits a manifest's switch string into arguments. Double quotes group words; they are not
// kept. A placeholder such as <INSTALLPATH> is refused: it would need a value WinForge does not have.
func splitSwitches(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQuote, has := false, false
	for _, r := range s {
		switch {
		case unicode.IsControl(r) && !unicode.IsSpace(r):
			return nil, errors.New("control character in switches")
		case r == '"':
			inQuote, has = !inQuote, true
		case unicode.IsSpace(r) && !inQuote:
			if has {
				out = append(out, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(r)
			has = true
		}
	}
	if inQuote {
		return nil, errors.New("unbalanced quote in switches")
	}
	if has {
		out = append(out, cur.String())
	}
	for _, a := range out {
		if strings.ContainsAny(a, "<>") {
			return nil, errors.New("switches need a value WinForge does not have")
		}
	}
	return out, nil
}

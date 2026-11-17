// Package offlinekit prepares the installers of a set of catalog apps in a folder
// (a USB stick, a share) while there is a network, and installs from that folder
// on a PC that has none.
//
// The folder holds one sub-folder per app, as winget wrote it with
// `winget download`, and a kit.json at its root that records every file with its
// SHA-256 and the installer facts (type, silent switches) read from the winget
// manifest. Opening a kit checks every file against kit.json and against the
// manifest's InstallerSha256 and refuses anything that changed, that is not
// listed, or that is a link; installing runs nothing that kit.json does not name,
// from a private copy that was checked once more.
//
// Everything that touches the machine goes through FS, Downloader and the
// install.Executor, so the logic is tested with in-memory fakes and never
// downloads or installs anything.
package offlinekit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// ManifestName is the file at the root of a kit.
const ManifestName = "kit.json"

// Format is the kit.json version this code writes and reads.
const Format = 1

// maxManifestBytes bounds how much of kit.json is read: a kit file is a few KB per app.
const maxManifestBytes = 8 << 20

// File is one file of an app, as it was after the download.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Installer is what the downloaded winget manifest says about the installer.
type Installer struct {
	// File is the installer's file name inside the app's folder.
	File string `json:"file"`
	// Type is winget's InstallerType, lower-cased (msi, wix, exe, nullsoft, inno, burn, msix, zip...).
	Type  string `json:"type"`
	Arch  string `json:"arch,omitempty"`
	Scope string `json:"scope,omitempty"`
	// Silent and Custom are the manifest's switches, as written there.
	Silent string `json:"silent,omitempty"`
	Custom string `json:"custom,omitempty"`
	// SuccessCodes are the manifest's InstallerSuccessCodes.
	SuccessCodes []int `json:"successCodes,omitempty"`
	// ElevationRequired is true when the manifest says the installer needs administrator rights.
	ElevationRequired bool `json:"elevationRequired,omitempty"`
	// ManifestFile is the manifest's file name inside the app's folder.
	ManifestFile string `json:"manifestFile"`
	// ManifestSha256 is the InstallerSha256 the manifest pins for this installer.
	ManifestSha256 string `json:"manifestSha256,omitempty"`
}

// Entry is one app in the kit.
type Entry struct {
	ID      string `json:"id"`
	Winget  string `json:"winget"`
	Name    string `json:"name"`
	Version string `json:"version"`
	// Dir is the app's folder, one name directly under the kit's root.
	Dir       string    `json:"dir"`
	Files     []File    `json:"files"`
	Installer Installer `json:"installer"`
	Size      int64     `json:"size"`
	Admin     bool      `json:"admin,omitempty"`
	Created   string    `json:"created"`
	WinForge  string    `json:"winforge"`
}

// Reasons an app is not in a kit, as keys the UI translates.
const (
	NotStore       = "store"       // a Microsoft Store app: winget cannot download its installer
	NotType        = "type"        // the installer's type cannot be run safely offline (Detail says which)
	NotSwitches    = "switches"    // no silent switches known for that installer
	NotDownload    = "download"    // winget could not download it
	NotManifest    = "manifest"    // the download came without a manifest WinForge can read
	NotHash        = "hash"        // the download does not match the manifest's hash
	NotSpace       = "space"       // not enough free space on the kit's drive
	NotCancelled   = "cancelled"   // the creation was stopped before this app
	NotUnknownApp  = "unknown"     // not an app of the catalog
	NotInvalidName = "invalidname" // its winget id cannot be a folder name
)

// NotIncluded is an app that is not in the kit, and why.
type NotIncluded struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Winget string `json:"winget,omitempty"`
	Reason string `json:"reason"`
	// Detail is technical context for the reason: the installer type, winget's exit code.
	Detail string `json:"detail,omitempty"`
}

// Kit is kit.json.
type Kit struct {
	Format      int           `json:"format"`
	WinForge    string        `json:"winforge"`
	Created     string        `json:"created"`
	Updated     string        `json:"updated,omitempty"`
	Profile     string        `json:"profile,omitempty"`
	Apps        []Entry       `json:"apps"`
	NotIncluded []NotIncluded `json:"notIncluded"`
}

// Error codes. They travel inside the error text ("kit:<code>") so the UI can say them in the person's language.
const (
	CodeNoKit     = "nokit"     // there is no kit.json in the folder
	CodeBadKit    = "badkit"    // kit.json is not a kit WinForge wrote
	CodeNewer     = "newer"     // kit.json is from a newer WinForge
	CodeBusy      = "busy"      // another kit task is running
	CodeCancelled = "cancelled" // stopped by the person
	CodeFolder    = "folder"    // the folder cannot be used for a kit
	CodeNotOpen   = "notopen"   // nothing was opened (or the PC changed), open the kit again
	CodeNotListed = "notlisted" // an id the opened kit did not offer
	CodeNothing   = "nothing"   // nothing selected
)

// Error is a refusal with a stable code.
type Error struct {
	Code   string
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return "kit:" + e.Code
	}
	return "kit:" + e.Code + " " + e.Detail
}

// IsCode reports whether err is a kit Error with that code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Encode renders a kit as the JSON kit.json holds.
func Encode(k *Kit) ([]byte, error) {
	b, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Decode reads kit.json. It rejects what is not a kit of this format.
func Decode(b []byte) (*Kit, error) {
	if len(b) > maxManifestBytes {
		return nil, &Error{Code: CodeBadKit, Detail: "too large"}
	}
	var k Kit
	if err := json.Unmarshal(b, &k); err != nil {
		return nil, &Error{Code: CodeBadKit, Detail: err.Error()}
	}
	switch {
	case k.Format > Format:
		return nil, &Error{Code: CodeNewer}
	case k.Format < 1:
		return nil, &Error{Code: CodeBadKit, Detail: "no format"}
	}
	if k.Apps == nil {
		k.Apps = []Entry{}
	}
	if k.NotIncluded == nil {
		k.NotIncluded = []NotIncluded{}
	}
	return &k, nil
}

// Fingerprint is the SHA-256 of kit.json's bytes. It is shown when a kit is made and when it is opened, so a
// person can compare the two: a kit whose files and kit.json were both replaced has a different one.
func Fingerprint(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

var (
	wingetID  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,99}$`)
	sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// reservedNames are Windows device names; a file or folder called so (with any extension) is refused.
var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// SafeName reports whether s can be the name of one file or folder inside the kit: a single path segment
// that is not empty, not "." or "..", has no separator, drive letter, wildcard or control character, does not
// end in a dot or a space and is not a device name. Names that come from kit.json are checked with it before
// they are joined to a path, so a kit can never point outside its own folder.
func SafeName(s string) bool {
	if s == "" || len(s) > 200 || s == "." || s == ".." {
		return false
	}
	if s != strings.TrimRight(s, ". ") || s != strings.TrimLeft(s, " ") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return false
		}
	}
	if filepath.IsAbs(s) || filepath.VolumeName(s) != "" || filepath.Base(s) != s {
		return false
	}
	stem, _, _ := strings.Cut(s, ".")
	return !reservedNames[strings.ToUpper(stem)]
}

// ValidWingetID reports whether a winget package identifier can be the name of the app's folder.
func ValidWingetID(id string) bool { return wingetID.MatchString(id) && SafeName(id) }

// isStoreID tells a Microsoft Store product id (12 letters and digits, no dot) from a winget package
// identifier ("Publisher.Package"). winget cannot download a Store app's installer.
func isStoreID(id string) bool { return !strings.Contains(id, ".") }

func newSet[T comparable](items ...T) map[T]bool {
	m := make(map[T]bool, len(items))
	for _, i := range items {
		m[i] = true
	}
	return m
}

func errf(code, format string, a ...any) error {
	return &Error{Code: code, Detail: fmt.Sprintf(format, a...)}
}

// Package compare sets two machines' apps side by side: this PC against a profile file (or a share
// code), or one profile file against another. It only reads. A profile file is data, so nothing in
// one is ever run, and a hostile or damaged file is rejected with a code instead of a crash.
package compare

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/profile"
)

const (
	// MaxFile bounds a profile file (and a share code, both packed and unpacked). A real profile is a
	// few KB; this leaves room for the whole catalog many times over.
	MaxFile = 1 << 20
	// MaxApps bounds the apps a file may list, so a crafted file cannot make the page build a huge list.
	MaxApps = 5000
	// maxID bounds an app id that is shown back. Longer ids cannot be in any catalog.
	maxID   = 80
	maxText = 200
)

// Error is a failure that the interface words itself, in the person's language: Go sends a code, never prose.
type Error struct{ Code string }

func (e *Error) Error() string { return "compare:" + e.Code }

func fail(code string) error { return &Error{Code: code} }

// Codes the interface knows: too-big, not-json, not-profile, newer-format, too-many, not-code, cut-off,
// no-scan, not-picked, unreadable.

// Side is one thing being compared.
type Side struct {
	// Name is the profile's name ("" for this PC).
	Name string
	// Versions maps catalog app id to its version ("" when the side does not say).
	Versions map[string]string
	// Unknown are ids this catalog does not have, in order of appearance, without duplicates.
	Unknown []string
	// UnknownRecipes and UnknownProfiles are references the catalog does not have; they are dropped.
	UnknownRecipes  []string
	UnknownProfiles []string
	// Profile is the file cleaned to what this catalog knows, ready to be saved; nil for this PC.
	Profile *catalog.Profile
	// Versioned is true when at least one app carries a version.
	Versioned bool
}

// PC builds the side for this machine from a scan.
func PC(versions map[string]string) Side {
	s := Side{Versions: make(map[string]string, len(versions))}
	for id, v := range versions {
		s.Versions[strings.ToLower(strings.TrimSpace(id))] = strings.TrimSpace(v)
		if strings.TrimSpace(v) != "" {
			s.Versioned = true
		}
	}
	return s
}

// Differ is an app both sides have, at different versions.
type Differ struct{ ID, A, B string }

// UnknownID is an id the catalog does not have; In says where it was: "a", "b" or "both".
type UnknownID struct{ ID, In string }

// Result is the comparison. The lists are sorted by id and never nil.
type Result struct {
	OnlyA   []string
	OnlyB   []string
	Both    []string // in both, and not known to differ
	Differ  []Differ // in both, with a version on each side and the two differ
	Unknown []UnknownID
}

// Compare sets a against b. Ids are compared as the catalog writes them (lower case); versions only when
// both sides have one.
func Compare(a, b Side) Result {
	r := Result{OnlyA: []string{}, OnlyB: []string{}, Both: []string{}, Differ: []Differ{}, Unknown: []UnknownID{}}
	for id, va := range a.Versions {
		vb, ok := b.Versions[id]
		switch {
		case !ok:
			r.OnlyA = append(r.OnlyA, id)
		case va != "" && vb != "" && va != vb:
			r.Differ = append(r.Differ, Differ{ID: id, A: va, B: vb})
		default:
			r.Both = append(r.Both, id)
		}
	}
	for id := range b.Versions {
		if _, ok := a.Versions[id]; !ok {
			r.OnlyB = append(r.OnlyB, id)
		}
	}
	sort.Strings(r.OnlyA)
	sort.Strings(r.OnlyB)
	sort.Strings(r.Both)
	sort.Slice(r.Differ, func(i, j int) bool { return r.Differ[i].ID < r.Differ[j].ID })

	in := map[string]string{}
	for _, id := range a.Unknown {
		in[id] = "a"
	}
	for _, id := range b.Unknown {
		if in[id] == "a" {
			in[id] = "both"
		} else {
			in[id] = "b"
		}
	}
	for id, where := range in {
		r.Unknown = append(r.Unknown, UnknownID{ID: id, In: where})
	}
	sort.Slice(r.Unknown, func(i, j int) bool { return r.Unknown[i].ID < r.Unknown[j].ID })
	return r
}

// file mirrors profile.File but is read leniently: unknown fields are ignored, so a file from a newer
// WinForge that added a field can still be compared. Wrong types still fail.
type file struct {
	WinForge int `json:"winforge"`
	Profile  *struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Extends     []string `json:"extends"`
		Apps        []struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		} `json:"apps"`
		Recipes []string `json:"recipes"`
	} `json:"profile"`
}

// ParseFile reads the bytes of an exported profile file. It never runs or installs anything.
func ParseFile(b []byte, cat *catalog.Catalog) (Side, error) {
	if len(b) > MaxFile {
		return Side{}, fail("too-big")
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	dec := json.NewDecoder(bytes.NewReader(b))
	var f file
	if err := dec.Decode(&f); err != nil {
		return Side{}, fail("not-json")
	}
	// Whatever follows the document (a second document, stray text) means the file is not what it says.
	if _, err := dec.Token(); err != io.EOF {
		return Side{}, fail("not-json")
	}
	if f.WinForge < 1 || f.Profile == nil {
		return Side{}, fail("not-profile")
	}
	if f.WinForge > profile.FormatVersion {
		return Side{}, fail("newer-format")
	}
	p := f.Profile
	if len(p.Apps) > MaxApps || len(p.Recipes) > MaxApps || len(p.Extends) > MaxApps {
		return Side{}, fail("too-many")
	}
	s := Side{Name: cleanText(p.Name), Versions: map[string]string{}}
	clean := catalog.Profile{ID: "imported", Name: s.Name, Description: cleanText(p.Description), Kind: "custom"}

	// Profiles it extends contribute their apps, with no version.
	for _, e := range p.Extends {
		e = strings.ToLower(strings.TrimSpace(e))
		base := cat.Profiles[e]
		if base == nil {
			s.UnknownProfiles = appendOnce(s.UnknownProfiles, shown(e))
			continue
		}
		clean.Extends = append(clean.Extends, e)
		if res, err := cat.ResolveProfile(base); err == nil {
			for _, a := range res.Apps {
				if _, ok := s.Versions[a.ID]; !ok {
					s.Versions[a.ID] = ""
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, a := range p.Apps {
		id := strings.ToLower(strings.TrimSpace(a.ID))
		if id == "" {
			continue
		}
		if cat.Apps[id] == nil {
			s.Unknown = appendOnce(s.Unknown, shown(id))
			continue
		}
		v := strings.TrimSpace(a.Version)
		if !validVersion(v) {
			v = "" // a version that could not be pinned is not compared
		}
		if seen[id] {
			if v != "" && s.Versions[id] == "" {
				s.Versions[id] = v
				setVersion(clean.Apps, id, v)
			}
			continue
		}
		seen[id] = true
		s.Versions[id] = v
		clean.Apps = append(clean.Apps, catalog.ProfileApp{ID: id, Version: v})
	}
	for _, r := range p.Recipes {
		r = strings.ToLower(strings.TrimSpace(r))
		if cat.Recipes[r] == nil {
			s.UnknownRecipes = appendOnce(s.UnknownRecipes, shown(r))
		} else {
			clean.Recipes = append(clean.Recipes, r)
		}
	}
	for _, v := range s.Versions {
		if v != "" {
			s.Versioned = true
		}
	}
	s.Profile = &clean
	return s, nil
}

func setVersion(apps []catalog.ProfileApp, id, v string) {
	for i := range apps {
		if apps[i].ID == id {
			apps[i].Version = v
		}
	}
}

// ParseCode reads a share code the way profile.DecodeCode does, with the same limits, and then compares
// like a file.
func ParseCode(code string, cat *catalog.Catalog) (Side, error) {
	code = strings.Join(strings.Fields(code), "")
	if len(code) > MaxFile {
		return Side{}, fail("too-big")
	}
	if !strings.HasPrefix(code, profile.CodePrefix) {
		return Side{}, fail("not-code")
	}
	packed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, profile.CodePrefix))
	if err != nil {
		return Side{}, fail("cut-off")
	}
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return Side{}, fail("cut-off")
	}
	raw, err := io.ReadAll(io.LimitReader(zr, MaxFile+1))
	if err != nil {
		return Side{}, fail("cut-off")
	}
	if len(raw) > MaxFile {
		return Side{}, fail("too-big")
	}
	return ParseFile(raw, cat)
}

// validVersion matches pinned winget versions: letters, digits and . _ + ~ : - only, as profile.Import requires.
func validVersion(v string) bool {
	if v == "" || len(v) > 64 {
		return false
	}
	for i, r := range v {
		ok := r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || (i > 0 && strings.ContainsRune("._+~:-", r)))
		if !ok {
			return false
		}
	}
	return true
}

// cleanText keeps free text that is safe to store and show: no control or line-separator characters, bounded.
func cleanText(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	var out []rune
	for _, r := range s {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			continue
		}
		out = append(out, r)
		if len(out) >= maxText {
			break
		}
	}
	return strings.TrimSpace(string(out))
}

// shown makes an id from a file safe to list: bounded, valid text, no control characters.
func shown(id string) string {
	id = cleanText(id)
	if r := []rune(id); len(r) > maxID {
		id = string(r[:maxID]) + "…"
	}
	return id
}

func appendOnce(list []string, s string) []string {
	if s == "" {
		return list
	}
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

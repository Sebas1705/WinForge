package sysapps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/Sebas1705/WinForge/internal/compare"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// maxPathLen bounds a path argument.
const maxPathLen = 1024

// reservedDevice are Windows device names; "NUL.json" still opens the device, so the check is on the part
// of the file name before the first dot.
var reservedDevice = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true,
}

func isDeviceName(base string) bool {
	stem := strings.ToUpper(strings.TrimRight(strings.SplitN(base, ".", 2)[0], " "))
	if reservedDevice[stem] {
		return true
	}
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
		return true
	}
	return false
}

// checkProfilePath applies the rules every path from the model must meet before anything is opened: an
// absolute local path to a .json file, with no UNC or device syntax, no stream, no control characters.
func checkProfilePath(p string) error {
	switch {
	case p == "":
		return errors.New("path is empty")
	case len(p) > maxPathLen:
		return fmt.Errorf("path is longer than %d characters", maxPathLen)
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return errors.New("path has control characters")
		}
	}
	if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") {
		return errors.New("network (UNC) and device paths are not accepted")
	}
	if !filepath.IsAbs(p) {
		return errors.New("path must be absolute")
	}
	clean := filepath.Clean(p)
	if strings.Contains(strings.TrimPrefix(clean, filepath.VolumeName(clean)), ":") {
		return errors.New("alternate data streams are not accepted")
	}
	if !strings.EqualFold(filepath.Ext(clean), ".json") {
		return errors.New("path must end in .json")
	}
	if isDeviceName(filepath.Base(clean)) {
		return errors.New("device names are not accepted")
	}
	return nil
}

// readProfileFile reads a profile file the person pointed at: only a regular file (no directory, link or
// device), at most compare.MaxFile bytes, opened once and checked again after opening.
func readProfileFile(p string) ([]byte, error) {
	if err := checkProfilePath(p); err != nil {
		return nil, fmt.Errorf("path rejected: %w", err)
	}
	clean := filepath.Clean(p)
	st, err := os.Lstat(clean)
	if err != nil {
		return nil, &compare.Error{Code: "unreadable"}
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("path rejected: not a regular file (folders, links and devices are not accepted)")
	}
	if st.Size() > compare.MaxFile {
		return nil, &compare.Error{Code: "too-big"}
	}
	f, err := os.Open(clean)
	if err != nil {
		return nil, &compare.Error{Code: "unreadable"}
	}
	defer f.Close()
	if st2, err := f.Stat(); err != nil || !st2.Mode().IsRegular() {
		return nil, errors.New("path rejected: not a regular file (folders, links and devices are not accepted)")
	}
	b, err := io.ReadAll(io.LimitReader(f, compare.MaxFile+1))
	if err != nil {
		return nil, &compare.Error{Code: "unreadable"}
	}
	if len(b) > compare.MaxFile {
		return nil, &compare.Error{Code: "too-big"}
	}
	return b, nil
}

type compareApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type compareDiffer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	ThisPC  string `json:"thisPc"`
	Profile string `json:"profile"`
}

type compareUnknown struct {
	ID string `json:"id"`
	In string `json:"in"` // pc | profile | both
}

type compareProfileOut struct {
	Source          string           `json:"source"` // file | code
	File            string           `json:"file,omitempty"`
	ProfileName     string           `json:"profileName,omitempty"`
	ProfileApps     int              `json:"profileApps"`
	PCApps          int              `json:"pcApps"`
	Versioned       bool             `json:"versioned"`
	OnlyThisPC      []compareApp     `json:"onlyThisPc"`
	OnlyProfile     []compareApp     `json:"onlyProfile"`
	Both            []compareApp     `json:"both"`
	Differ          []compareDiffer  `json:"differ"`
	Unknown         []compareUnknown `json:"unknown"`
	UnknownRecipes  []string         `json:"unknownRecipes"`
	UnknownProfiles []string         `json:"unknownProfiles"`
	Counts          map[string]int   `json:"counts"`
	Truncated       bool             `json:"truncated"`
	Warnings        []string         `json:"warnings"`
}

func (r *rt) compareProfileTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_compare_profile",
		Title: "Compare this PC with a profile",
		Description: "Compares the catalog apps installed on this PC (a read of the PC's software list: personal data) with a WinForge profile file (path) or a share code (code) the person gives, exactly one of the two. " +
			"Returns what only this PC has, what only the profile lists, what both have, apps whose versions differ, and ids the catalog does not know. " +
			"The file is treated as hostile data: it must be an absolute local path to a regular .json file of at most 1 MiB (folders, devices, links and UNC/network paths are refused), " +
			"it is only parsed against the catalog and nothing in it is ever run. It never installs, changes or writes anything.",
		InputSchema: mcpkit.Schema(`{"type":"object","additionalProperties":false,"properties":{` +
			`"path":{"type":"string","maxLength":1024,"description":"absolute path of a WinForge profile .json file"},` +
			`"code":{"type":"string","maxLength":1048576,"description":"a WinForge share code (starts with WF1.)"},` +
			`"limit":` + limitSchema + `}}`),
		Class: mcpkit.Read,
		Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
			var a struct {
				Path  string `json:"path"`
				Code  string `json:"code"`
				Limit *int   `json:"limit"`
			}
			if bad := decode(c, &a); bad != nil {
				return bad, nil
			}
			if (a.Path == "") == (a.Code == "") {
				return mcpkit.Fail("give exactly one of path or code"), nil
			}
			limit, err := limitOf(a.Limit)
			if err != nil {
				return mcpkit.Fail("%s", err.Error()), nil
			}
			e := r.env(c)
			red := redactor(e)
			cat, bad := r.catalogOf(e)
			if bad != nil {
				return bad, nil
			}
			// Read and parse the other side first: a hostile or wrong file is refused before the PC is scanned.
			var other compare.Side
			out := compareProfileOut{Source: "code"}
			if a.Path != "" {
				b, err := readProfileFile(a.Path)
				if err == nil {
					other, err = compare.ParseFile(b, cat)
				}
				if err != nil {
					return mcpkit.Fail("%s", red(compareMsg(err))), nil
				}
				out.Source, out.File = "file", red(filepath.Base(filepath.Clean(a.Path)))
			} else {
				other, err = compare.ParseCode(a.Code, cat)
				if err != nil {
					return mcpkit.Fail("%s", compareMsg(err)), nil
				}
			}
			s, err := r.scan(ctx, e, cat)
			if err != nil {
				return mcpkit.Fail("could not scan this PC: %s", err.Error()), nil
			}
			pc := compare.PC(versionsOf(s.installed))
			res := compare.Compare(pc, other)

			out.ProfileName, out.ProfileApps, out.PCApps, out.Versioned = other.Name, len(other.Versions), len(pc.Versions), other.Versioned
			out.UnknownRecipes, out.UnknownProfiles = nz(other.UnknownRecipes), nz(other.UnknownProfiles)
			out.Warnings = warnings(s.warning)
			out.Counts = map[string]int{"onlyThisPc": len(res.OnlyA), "onlyProfile": len(res.OnlyB), "both": len(res.Both), "differ": len(res.Differ), "unknown": len(res.Unknown)}
			apps := func(ids []string) ([]compareApp, bool) {
				ids, cut := capList(ids, limit)
				l := make([]compareApp, 0, len(ids))
				for _, id := range ids {
					l = append(l, compareApp{ID: id, Name: appName(cat, id)})
				}
				return l, cut
			}
			var c1, c2, c3, c4, c5 bool
			out.OnlyThisPC, c1 = apps(res.OnlyA)
			out.OnlyProfile, c2 = apps(res.OnlyB)
			out.Both, c3 = apps(res.Both)
			ds, cut := capList(res.Differ, limit)
			c4 = cut
			out.Differ = make([]compareDiffer, 0, len(ds))
			for _, d := range ds {
				out.Differ = append(out.Differ, compareDiffer{ID: d.ID, Name: appName(cat, d.ID), ThisPC: d.A, Profile: d.B})
			}
			us, cut := capList(res.Unknown, limit)
			c5 = cut
			out.Unknown = make([]compareUnknown, 0, len(us))
			for _, u := range us {
				in := map[string]string{"a": "pc", "b": "profile", "both": "both"}[u.In]
				out.Unknown = append(out.Unknown, compareUnknown{ID: u.ID, In: in})
			}
			out.Truncated = c1 || c2 || c3 || c4 || c5
			return mcpkit.JSON(out), nil
		},
	}
}

// compareMsg words a comparison failure: the code compare gives, or the path rule that was broken.
func compareMsg(err error) string {
	var ce *compare.Error
	if errors.As(err, &ce) {
		return "the profile could not be compared: " + ce.Code
	}
	return err.Error()
}

func nz(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

package offlinekit

import (
	"context"
	"io"
	"strings"
)

// Problem kinds, as keys the UI translates.
const (
	ProbInvalid          = "invalid"          // a name or field in kit.json that is not allowed (path tricks, bad hash)
	ProbUnknownApp       = "unknownapp"       // the app is not in this WinForge's catalog, or its winget id differs
	ProbMissingDir       = "missingdir"       // the app's folder is gone
	ProbLink             = "link"             // a link or other special file where a plain file belongs
	ProbNotFile          = "notfile"          // a folder where a file belongs
	ProbExtra            = "extra"            // a file that kit.json does not list
	ProbMissing          = "missing"          // a listed file is gone
	ProbSize             = "size"             // a file's size differs from kit.json
	ProbChanged          = "changed"          // a file's SHA-256 differs from kit.json
	ProbManifestHash     = "manifesthash"     // the installer is not what the winget manifest pins
	ProbManifestMismatch = "manifestmismatch" // kit.json's installer facts differ from the manifest's
	ProbManifest         = "manifest"         // the manifest cannot be read
	ProbDuplicate        = "duplicate"        // the same app or folder twice
	ProbUnreadable       = "unreadable"       // a file cannot be read (a stick that was removed)
)

// Problem is one thing wrong with an app of the kit.
type Problem struct {
	Kind string `json:"kind"`
	File string `json:"file,omitempty"`
}

// App states after verification.
const (
	StateReady       = "ready"       // verified and can be installed offline
	StateProblem     = "problem"     // something is wrong; it will not be run
	StateUnsupported = "unsupported" // intact, but it cannot be installed offline
)

// AppReport is one kit app after verification.
type AppReport struct {
	Entry    Entry
	State    string
	Problems []Problem
	// Unsupported is why an intact app cannot be installed offline: NotType or NotSwitches.
	Unsupported       string
	UnsupportedDetail string
}

// Report is a verified kit.
type Report struct {
	Kit         *Kit
	Fingerprint string
	Apps        []AppReport
	// Unlisted are names in the kit's folder that kit.json does not mention; they are never run.
	Unlisted []string
}

// Progress says how far a long kit task is.
type Progress struct {
	Task    string `json:"task"` // create, verify
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Percent int    `json:"percent,omitempty"`
}

// Known tells whether an app id and winget id are an app of the catalog.
type Known func(id, winget string) bool

// ReadKit reads and decodes kit.json from a folder, with its fingerprint.
func ReadKit(ctx context.Context, fs FS, root string) (*Kit, string, error) {
	path := join(root, ManifestName)
	fi, err := fs.Lstat(path)
	if err != nil {
		return nil, "", &Error{Code: CodeNoKit}
	}
	if fi.Link || fi.Dir {
		return nil, "", &Error{Code: CodeBadKit, Detail: "kit.json is not a plain file"}
	}
	if fi.Size > maxManifestBytes {
		return nil, "", &Error{Code: CodeBadKit, Detail: "too large"}
	}
	f, err := fs.Open(path)
	if err != nil {
		return nil, "", &Error{Code: CodeNoKit}
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(ctxReader{ctx, f}, maxManifestBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", &Error{Code: CodeCancelled}
		}
		return nil, "", &Error{Code: CodeNoKit}
	}
	k, err := Decode(b)
	if err != nil {
		return nil, "", err
	}
	return k, Fingerprint(b), nil
}

// Verify opens a kit: it reads kit.json and checks every app's folder against it. Nothing in a folder may be
// a link, nothing may be there that kit.json does not list, every listed file must have its recorded size and
// SHA-256, and the installer must be the one the winget manifest pins. An app that fails any check is reported
// with what is wrong and is never run; the rest of the kit stays usable.
func Verify(ctx context.Context, fs FS, root string, known Known, progress func(Progress)) (*Report, error) {
	k, fp, err := ReadKit(ctx, fs, root)
	if err != nil {
		return nil, err
	}
	rep := &Report{Kit: k, Fingerprint: fp, Apps: make([]AppReport, 0, len(k.Apps)), Unlisted: []string{}}
	seenID, seenDir := map[string]bool{}, map[string]bool{}
	for i, e := range k.Apps {
		if ctx.Err() != nil {
			return nil, &Error{Code: CodeCancelled}
		}
		if progress != nil {
			progress(Progress{Task: "verify", Done: i, Total: len(k.Apps), ID: e.ID, Name: e.Name})
		}
		ar := AppReport{Entry: e}
		ar.Problems = verifyEntry(ctx, fs, root, e, known)
		if ctx.Err() != nil {
			return nil, &Error{Code: CodeCancelled}
		}
		key, dir := strings.ToLower(e.ID), strings.ToLower(e.Dir)
		if seenID[key] || seenDir[dir] {
			ar.Problems = append(ar.Problems, Problem{Kind: ProbDuplicate})
		}
		seenID[key], seenDir[dir] = true, true
		switch {
		case len(ar.Problems) > 0:
			ar.State = StateProblem
		default:
			if reason, detail := unsupported(e.Installer); reason != "" {
				ar.State, ar.Unsupported, ar.UnsupportedDetail = StateUnsupported, reason, detail
			} else {
				ar.State = StateReady
			}
		}
		rep.Apps = append(rep.Apps, ar)
	}
	if progress != nil {
		progress(Progress{Task: "verify", Done: len(k.Apps), Total: len(k.Apps)})
	}
	rep.Unlisted = unlisted(fs, root, k)
	return rep, nil
}

// unsupported says why an installer cannot be run offline, or "" when it can.
func unsupported(in Installer) (reason, detail string) {
	if Classify(in.Type, in.File) == ClassNo {
		return NotType, in.Type
	}
	if _, err := BuildCommand(in, "x"); err != nil {
		return NotSwitches, ""
	}
	return "", ""
}

// unlisted names what sits in the kit's folder without being in kit.json. Reading the folder can fail
// (a stick pulled out); then there is simply nothing to report.
func unlisted(fs FS, root string, k *Kit) []string {
	ents, err := fs.ReadDir(root)
	if err != nil {
		return []string{}
	}
	known := newSet(strings.ToLower(ManifestName))
	for _, e := range k.Apps {
		known[strings.ToLower(e.Dir)] = true
	}
	out := []string{}
	for _, e := range ents {
		if !known[strings.ToLower(e.Name)] {
			out = append(out, e.Name)
		}
	}
	return out
}

func verifyEntry(ctx context.Context, fs FS, root string, e Entry, known Known) []Problem {
	var probs []Problem
	add := func(kind, file string) { probs = append(probs, Problem{Kind: kind, File: file}) }

	// 1. Everything that will become a path must be a plain name, before any disk is touched.
	if e.ID == "" || !ValidWingetID(e.Winget) || e.Dir != e.Winget || !SafeName(e.Dir) {
		add(ProbInvalid, "")
		return probs
	}
	if known != nil && !known(e.ID, e.Winget) {
		add(ProbUnknownApp, "")
		return probs
	}
	listed := map[string]File{}
	for _, f := range e.Files {
		lk := strings.ToLower(f.Name)
		if !SafeName(f.Name) || !sha256Hex.MatchString(f.SHA256) || f.Size < 0 {
			add(ProbInvalid, f.Name)
			return probs
		}
		if _, dup := listed[lk]; dup {
			add(ProbInvalid, f.Name)
			return probs
		}
		listed[lk] = f
	}
	inst, man := strings.ToLower(e.Installer.File), strings.ToLower(e.Installer.ManifestFile)
	if _, ok := listed[inst]; !ok || inst == "" {
		add(ProbInvalid, e.Installer.File)
		return probs
	}
	if _, ok := listed[man]; !ok || man == "" || man == inst {
		add(ProbInvalid, e.Installer.ManifestFile)
		return probs
	}

	// 2. The folder: a plain folder, holding plain files that are all listed.
	dir := join(root, e.Dir)
	di, err := fs.Lstat(dir)
	if err != nil {
		add(ProbMissingDir, "")
		return probs
	}
	if di.Link || !di.Dir {
		add(ProbLink, e.Dir)
		return probs
	}
	ents, err := fs.ReadDir(dir)
	if err != nil {
		add(ProbUnreadable, e.Dir)
		return probs
	}
	present := map[string]Info{}
	for _, en := range ents {
		lk := strings.ToLower(en.Name)
		present[lk] = en
		if _, ok := listed[lk]; !ok {
			add(ProbExtra, en.Name)
			continue
		}
		switch {
		case en.Link:
			add(ProbLink, en.Name)
		case en.Dir:
			add(ProbNotFile, en.Name)
		}
	}
	if len(probs) > 0 {
		// Do not read anything from a folder that holds links or strangers.
		for _, f := range e.Files {
			if _, ok := present[strings.ToLower(f.Name)]; !ok {
				add(ProbMissing, f.Name)
			}
		}
		return probs
	}

	// 3. Every listed file: present, its recorded size, its recorded SHA-256.
	actual := map[string]string{}
	for _, f := range e.Files {
		en, ok := present[strings.ToLower(f.Name)]
		if !ok {
			add(ProbMissing, f.Name)
			continue
		}
		if en.Size != f.Size {
			add(ProbSize, f.Name)
			continue
		}
		size, sum, err := hashFile(ctx, fs, join(dir, f.Name))
		switch {
		case err != nil:
			add(ProbUnreadable, f.Name)
		case size != f.Size:
			add(ProbSize, f.Name)
		case sum != strings.ToLower(f.SHA256):
			add(ProbChanged, f.Name)
		default:
			actual[strings.ToLower(f.Name)] = sum
		}
	}
	if len(probs) > 0 {
		return probs
	}

	// 4. The winget manifest, which is itself a verified file: its pinned hash must be the installer's own,
	// and what kit.json records about the installer must be what the manifest says.
	return append(probs, checkManifest(ctx, fs, dir, e, actual[inst])...)
}

func checkManifest(ctx context.Context, fs FS, dir string, e Entry, installerSha string) []Problem {
	f, err := fs.Open(join(dir, e.Installer.ManifestFile))
	if err != nil {
		return []Problem{{Kind: ProbUnreadable, File: e.Installer.ManifestFile}}
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(ctxReader{ctx, f}, maxYAMLBytes+1))
	if err != nil {
		return []Problem{{Kind: ProbUnreadable, File: e.Installer.ManifestFile}}
	}
	m, err := ParseManifest(b)
	if err != nil {
		return []Problem{{Kind: ProbManifest, File: e.Installer.ManifestFile}}
	}
	mi, ok := m.Select(installerSha)
	if !ok {
		return []Problem{{Kind: ProbManifestHash, File: e.Installer.File}}
	}
	in := e.Installer
	if !strings.EqualFold(mi.Type, in.Type) || mi.Silent != in.Silent || mi.Custom != in.Custom ||
		!strings.EqualFold(mi.Sha256, in.ManifestSha256) || !sameCodes(mi.Codes, in.SuccessCodes) {
		return []Problem{{Kind: ProbManifestMismatch, File: e.Installer.ManifestFile}}
	}
	return nil
}

func sameCodes(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

package offlinekit

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
)

// Free-space figures. winget does not say how big an installer is before it downloads it, so the up-front
// check is an estimate (an average installer is well under this) and each app is checked again, for real,
// just before it is downloaded.
const (
	// EstimatePerApp is what the up-front check assumes one installer needs.
	EstimatePerApp int64 = 150 << 20
	// Reserve is the least free space an app's download is started with.
	Reserve int64 = 256 << 20
)

// Downloader fetches one winget package's installer and manifest into a folder.
type Downloader interface {
	Download(ctx context.Context, wingetID, dir string, onLine func(string)) (exitCode int, err error)
}

// WingetDownloader runs `winget download`.
type WingetDownloader struct{ Ex install.Executor }

// DownloadArgs builds the winget command line. Dependencies are skipped: the catalog names what an app
// requires, and a Dependencies folder would be files kit.json does not list.
func DownloadArgs(wingetID, dir string) []string {
	return []string{"download", "--id", wingetID, "--exact", "--source", "winget", "--download-directory", dir,
		"--skip-dependencies", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}
}

// Download implements Downloader.
func (d WingetDownloader) Download(ctx context.Context, wingetID, dir string, onLine func(string)) (int, error) {
	return d.Ex.Run(ctx, "winget", DownloadArgs(wingetID, dir), onLine)
}

// CreatePlan is what creating a kit would do, worked out before anything is downloaded.
type CreatePlan struct {
	Folder      string        `json:"folder"`
	Apps        []PlanApp     `json:"apps"`
	NotIncluded []NotIncluded `json:"notIncluded"`
	// NeedBytes is an estimate of the space the downloads need; FreeBytes is what the drive has (FreeKnown is
	// false when it could not be read).
	NeedBytes int64 `json:"needBytes"`
	FreeBytes int64 `json:"freeBytes"`
	FreeKnown bool  `json:"freeKnown"`
	Enough    bool  `json:"enough"`
	// ExistingKit is true when the folder already holds a kit: the apps are added to it.
	ExistingKit bool `json:"existingKit"`
	// Problem is a Code when the folder cannot be used.
	Problem string `json:"problem,omitempty"`
}

// PlanApp is an app that will be downloaded.
type PlanApp struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Winget string `json:"winget"`
}

// Plan works out which apps would go into a kit in folder and whether the drive has room for them. It reads
// the disk and changes nothing.
func Plan(ctx context.Context, fs FS, cat *catalog.Catalog, ids []string, folder string) CreatePlan {
	p := CreatePlan{Folder: folder, Apps: []PlanApp{}, NotIncluded: []NotIncluded{}}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		app := cat.Apps[id]
		switch {
		case app == nil:
			p.NotIncluded = append(p.NotIncluded, NotIncluded{ID: id, Name: id, Reason: NotUnknownApp})
		case isStoreID(app.Winget):
			p.NotIncluded = append(p.NotIncluded, NotIncluded{ID: id, Name: app.Name, Winget: app.Winget, Reason: NotStore})
		case !ValidWingetID(app.Winget):
			p.NotIncluded = append(p.NotIncluded, NotIncluded{ID: id, Name: app.Name, Winget: app.Winget, Reason: NotInvalidName})
		default:
			p.Apps = append(p.Apps, PlanApp{ID: id, Name: app.Name, Winget: app.Winget})
		}
	}
	p.NeedBytes = int64(len(p.Apps)) * EstimatePerApp
	if folder == "" || !filepath.IsAbs(folder) {
		p.Problem = CodeFolder
		return p
	}
	if fi, err := fs.Lstat(folder); err == nil {
		if fi.Link || !fi.Dir {
			p.Problem = CodeFolder
			return p
		}
		ents, err := fs.ReadDir(folder)
		if err != nil {
			p.Problem = CodeFolder
			return p
		}
		hasKit := false
		for _, e := range ents {
			if strings.EqualFold(e.Name, ManifestName) {
				hasKit = !e.Dir && !e.Link
			}
		}
		switch {
		case hasKit:
			if _, _, err := ReadKit(ctx, fs, folder); err != nil {
				p.Problem = CodeBadKit
				return p
			}
			p.ExistingKit = true
		case len(ents) > 0:
			p.Problem = CodeFolder // never write into a folder that holds somebody's files
			return p
		}
	}
	if free, err := fs.FreeBytes(folder); err == nil {
		p.FreeBytes, p.FreeKnown = free, true
	}
	p.Enough = !p.FreeKnown || p.FreeBytes >= p.NeedBytes+Reserve
	return p
}

// Options are what Create needs from the machine.
type Options struct {
	FS       FS
	Dl       Downloader
	Cat      *catalog.Catalog
	Version  string // WinForge's version, recorded in the kit
	Now      func() time.Time
	Progress func(Progress)
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// Result is what a creation did.
type Result struct {
	Kit         *Kit
	Fingerprint string
	// Added are the apps this run put in the kit; Failed the ones it could not (also listed in the kit).
	Added     []string
	Failed    []NotIncluded
	Bytes     int64
	Cancelled bool
}

// Create downloads the apps into folder, one folder per app, and writes kit.json (after every app, so a
// stop or a crash leaves a usable kit). A failed app is reported and the rest carry on. When the folder already
// holds a kit the new apps are added to it. A cancelled run keeps what finished.
func Create(ctx context.Context, o Options, ids []string, folder, profileName string) (*Result, error) {
	plan := Plan(ctx, o.FS, o.Cat, ids, folder)
	if plan.Problem != "" {
		return nil, &Error{Code: plan.Problem}
	}
	if err := o.FS.MkdirAll(folder); err != nil {
		return nil, errf(CodeFolder, "%v", err)
	}
	now := o.now().UTC().Format(time.RFC3339)
	kit := &Kit{Format: Format, WinForge: o.Version, Created: now, Profile: profileName, Apps: []Entry{}, NotIncluded: []NotIncluded{}}
	if plan.ExistingKit {
		old, _, err := ReadKit(ctx, o.FS, folder)
		if err != nil {
			return nil, err
		}
		kit = old
		kit.Updated = now
		if profileName != "" && kit.Profile != profileName {
			kit.Profile = ""
		}
	}
	res := &Result{Kit: kit}
	// What the plan refused up front is part of the kit's honest record.
	for _, n := range plan.NotIncluded {
		kit.setNotIncluded(n)
		res.Failed = append(res.Failed, n)
	}
	total := len(plan.Apps)
	report := func(i int, a PlanApp, pct int) {
		if o.Progress != nil {
			o.Progress(Progress{Task: "create", Done: i, Total: total, ID: a.ID, Name: a.Name, Percent: pct})
		}
	}
	for i, a := range plan.Apps {
		if ctx.Err() != nil {
			res.Cancelled = true
			n := NotIncluded{ID: a.ID, Name: a.Name, Winget: a.Winget, Reason: NotCancelled}
			kit.setNotIncluded(n)
			res.Failed = append(res.Failed, n)
			continue
		}
		report(i, a, 0)
		entry, bad := o.addApp(ctx, folder, a, func(pct int) { report(i, a, pct) })
		switch {
		case bad == nil:
			kit.setEntry(*entry)
			res.Added = append(res.Added, a.ID)
			res.Bytes += entry.Size
		case bad.Reason == NotCancelled:
			res.Cancelled = true
			kit.setNotIncluded(*bad)
			res.Failed = append(res.Failed, *bad)
		default:
			kit.setNotIncluded(*bad)
			res.Failed = append(res.Failed, *bad)
		}
		if err := writeKit(o.FS, folder, kit); err != nil {
			return res, errf(CodeFolder, "%v", err)
		}
	}
	if err := writeKit(o.FS, folder, kit); err != nil {
		return res, errf(CodeFolder, "%v", err)
	}
	b, _ := Encode(kit)
	res.Fingerprint = Fingerprint(b)
	if o.Progress != nil {
		o.Progress(Progress{Task: "create", Done: total, Total: total})
	}
	return res, nil
}

// setEntry puts an app in the kit, replacing an older copy, and takes it off the not-included list.
func (k *Kit) setEntry(e Entry) {
	replaced := false
	for i := range k.Apps {
		if k.Apps[i].ID == e.ID {
			k.Apps[i], replaced = e, true
		}
	}
	if !replaced {
		k.Apps = append(k.Apps, e)
	}
	k.dropNotIncluded(e.ID)
}

// setNotIncluded records that an app is not in the kit. An app that is already in it (from an earlier
// creation) stays: a failed refresh must not remove what still works.
func (k *Kit) setNotIncluded(n NotIncluded) {
	for _, e := range k.Apps {
		if e.ID == n.ID {
			return
		}
	}
	k.dropNotIncluded(n.ID)
	k.NotIncluded = append(k.NotIncluded, n)
}

func (k *Kit) dropNotIncluded(id string) {
	out := k.NotIncluded[:0]
	for _, n := range k.NotIncluded {
		if n.ID != id {
			out = append(out, n)
		}
	}
	k.NotIncluded = out
}

// addApp downloads one app and describes it. On any failure the app's folder is removed and bad says why.
func (o Options) addApp(ctx context.Context, root string, a PlanApp, pct func(int)) (entry *Entry, bad *NotIncluded) {
	fail := func(reason, detail string) (*Entry, *NotIncluded) {
		return nil, &NotIncluded{ID: a.ID, Name: a.Name, Winget: a.Winget, Reason: reason, Detail: detail}
	}
	if free, err := o.FS.FreeBytes(root); err == nil && free < Reserve {
		return fail(NotSpace, "")
	}
	dir := join(root, a.Winget)
	// A folder from an earlier creation is replaced: the plan only accepts a folder that is empty or already a kit.
	if err := o.FS.RemoveAll(dir); err != nil {
		return fail(NotDownload, err.Error())
	}
	if err := o.FS.MkdirAll(dir); err != nil {
		return fail(NotDownload, err.Error())
	}
	keep := false
	defer func() {
		if !keep {
			_ = o.FS.RemoveAll(dir)
		}
	}()
	code, err := o.Dl.Download(ctx, a.Winget, dir, func(line string) {
		if p := install.ParsePercent(line); p >= 0 {
			pct(p)
		}
	})
	if ctx.Err() != nil {
		return fail(NotCancelled, "")
	}
	if err != nil {
		return fail(NotDownload, err.Error())
	}
	if code != 0 {
		return fail(NotDownload, fmt.Sprintf("%#x", uint32(code)))
	}
	e, reason, detail := o.describe(ctx, dir, a)
	if ctx.Err() != nil {
		return fail(NotCancelled, "")
	}
	if reason != "" {
		return fail(reason, detail)
	}
	keep = true
	return e, nil
}

// describe reads what winget wrote into dir: the installer, its manifest and their hashes.
func (o Options) describe(ctx context.Context, dir string, a PlanApp) (*Entry, string, string) {
	ents, err := o.FS.ReadDir(dir)
	if err != nil {
		return nil, NotDownload, err.Error()
	}
	var files []File
	var installers, manifests []string
	var size int64
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name < ents[j].Name })
	for _, en := range ents {
		if en.Link || en.Dir || !SafeName(en.Name) {
			return nil, NotManifest, "unexpected " + en.Name
		}
		sz, sum, err := hashFile(ctx, o.FS, join(dir, en.Name))
		if err != nil {
			return nil, NotDownload, err.Error()
		}
		files = append(files, File{Name: en.Name, Size: sz, SHA256: sum})
		size += sz
		if strings.EqualFold(filepath.Ext(en.Name), ".yaml") {
			manifests = append(manifests, en.Name)
		} else {
			installers = append(installers, en.Name)
		}
	}
	if len(manifests) != 1 {
		return nil, NotManifest, "manifests"
	}
	if len(installers) != 1 {
		return nil, NotManifest, "installers"
	}
	raw, err := readSmall(ctx, o.FS, join(dir, manifests[0]))
	if err != nil {
		return nil, NotManifest, err.Error()
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return nil, NotManifest, ""
	}
	var instSum string
	for _, f := range files {
		if f.Name == installers[0] {
			instSum = f.SHA256
		}
	}
	mi, ok := m.Select(instSum)
	if !ok {
		return nil, NotHash, ""
	}
	in := Installer{
		File: installers[0], Type: mi.Type, Arch: mi.Arch, Scope: mi.Scope, Silent: mi.Silent, Custom: mi.Custom,
		SuccessCodes: mi.Codes, ElevationRequired: mi.Elevation, ManifestFile: manifests[0], ManifestSha256: mi.Sha256,
	}
	if reason, detail := unsupported(in); reason != "" {
		return nil, reason, detail
	}
	app := o.Cat.Apps[a.ID]
	admin := mi.Elevation || (app != nil && app.Admin) || (Classify(in.Type, in.File) == ClassMSI && mi.Scope != "user")
	return &Entry{
		ID: a.ID, Winget: a.Winget, Name: a.Name, Version: m.Version, Dir: a.Winget, Files: files, Installer: in,
		Size: size, Admin: admin, Created: o.now().UTC().Format(time.RFC3339), WinForge: o.Version,
	}, "", ""
}

// readSmall reads a manifest, refusing one that is unreasonably large.
func readSmall(ctx context.Context, fs FS, path string) ([]byte, error) {
	f, err := fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(ctxReader{ctx, f}, maxYAMLBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxYAMLBytes {
		return nil, fmt.Errorf("manifest too large")
	}
	return b, nil
}

// writeKit writes kit.json beside itself and renames it over the old one, so a crash cannot leave half a file.
func writeKit(fs FS, root string, k *Kit) error {
	b, err := Encode(k)
	if err != nil {
		return err
	}
	tmp := join(root, ManifestName+".tmp")
	_ = fs.RemoveAll(tmp)
	w, err := fs.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return fs.Rename(tmp, join(root, ManifestName))
}

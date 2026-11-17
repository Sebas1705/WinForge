package offlinekit

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/system"
)

// copyHeadroom is the free space left over after the private copy of an installer, so the installer itself
// still has room to unpack.
const copyHeadroom int64 = 256 << 20

// Selection is what BuildPlan made of the apps a person ticked.
type Selection struct {
	Plan install.Plan
	// Apps are the entries that will run, in the kit's order.
	Apps []Entry
}

// BuildPlan turns the ticked apps of a verified kit into a plan of kit steps, in the kit's order (which is the
// profile's dependency order). Only an app that verified as ready can be named: anything else is refused, so a
// plan can never contain a step the kit did not vouch for. An app that is already installed is reported as
// such and gets no step.
func BuildPlan(cat *catalog.Catalog, rep *Report, ids []string, installed map[string]system.Installed) (Selection, error) {
	if rep == nil {
		return Selection{}, &Error{Code: CodeNotOpen}
	}
	if len(ids) == 0 {
		return Selection{}, &Error{Code: CodeNothing}
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	ready := map[string]bool{}
	for _, a := range rep.Apps {
		if a.State == StateReady {
			ready[a.Entry.ID] = true
		}
	}
	for id := range want {
		if !ready[id] {
			return Selection{}, errf(CodeNotListed, "%s", id)
		}
	}
	sel := Selection{Plan: install.Plan{Steps: []install.Step{}, AlreadyInstalled: []string{}}}
	for _, a := range rep.Apps {
		if !want[a.Entry.ID] {
			continue
		}
		if _, have := installed[a.Entry.ID]; have {
			sel.Plan.AlreadyInstalled = append(sel.Plan.AlreadyInstalled, a.Entry.ID)
			continue
		}
		name := a.Entry.Name
		if app := cat.Apps[a.Entry.ID]; app != nil {
			name = app.Name
		}
		sel.Plan.Steps = append(sel.Plan.Steps, install.Step{Kind: install.StepKit, ID: a.Entry.ID, Name: name, Version: a.Entry.Version, Admin: a.Entry.Admin})
		sel.Plan.NeedsAdmin = sel.Plan.NeedsAdmin || a.Entry.Admin
		sel.Apps = append(sel.Apps, a.Entry)
	}
	return sel, nil
}

// Session answers the runner's kit steps for one verified selection. It implements install.KitResolver: for
// each app it copies the kit's files to a private folder, checking the copy against kit.json once more (so
// nothing can change between the check and the run, and the installer never runs from the stick), and builds
// the command from kit.json's installer facts. It knows only the apps it was built with.
type Session struct {
	fs   FS
	root string
	mu   sync.Mutex
	apps map[string]Entry
	tmp  string
}

// NewSession makes a resolver for the selected entries of the kit at root.
func NewSession(fs FS, root string, apps []Entry) *Session {
	m := make(map[string]Entry, len(apps))
	for _, a := range apps {
		m[a.ID] = a
	}
	return &Session{fs: fs, root: root, apps: m}
}

var _ install.KitResolver = (*Session)(nil)

func kitErr(reason, format string, a ...any) error {
	return &install.KitError{Reason: reason, Err: fmt.Errorf(format, a...)}
}

// Prepare implements install.KitResolver.
func (s *Session) Prepare(ctx context.Context, appID string, emit func(string)) (install.KitCommand, error) {
	e, ok := s.apps[appID]
	if !ok {
		return install.KitCommand{}, kitErr(install.ReasonKitClosed, "%s is not part of the opened kit", appID)
	}
	// The names were checked when the kit was opened; they are checked again because they become paths here.
	if !ValidWingetID(e.Winget) || e.Dir != e.Winget || !SafeName(e.Dir) {
		return install.KitCommand{}, kitErr(ReasonInvalid, "invalid folder name")
	}
	tmp, err := s.tempRoot()
	if err != nil {
		return install.KitCommand{}, kitErr(install.ReasonDisk, "%v", err)
	}
	if free, err := s.fs.FreeBytes(tmp); err == nil && free < e.Size+copyHeadroom {
		return install.KitCommand{}, kitErr(install.ReasonDisk, "not enough free space for the installer")
	}
	src := join(s.root, e.Dir)
	di, err := s.fs.Lstat(src)
	if err != nil {
		return install.KitCommand{}, kitErr(install.ReasonKitFile, "%v", err)
	}
	if di.Link || !di.Dir {
		return install.KitCommand{}, kitErr(install.ReasonKitHash, "the app's folder is not a plain folder")
	}
	dst := join(tmp, e.Dir)
	_ = s.fs.RemoveAll(dst)
	if err := s.fs.MkdirAll(dst); err != nil {
		return install.KitCommand{}, kitErr(install.ReasonDisk, "%v", err)
	}
	var instSum string
	for i, f := range e.Files {
		if !SafeName(f.Name) {
			return install.KitCommand{}, kitErr(ReasonInvalid, "invalid file name")
		}
		fi, err := s.fs.Lstat(join(src, f.Name))
		if err != nil {
			return install.KitCommand{}, kitErr(install.ReasonKitFile, "%s: %v", f.Name, err)
		}
		if fi.Link || fi.Dir {
			return install.KitCommand{}, kitErr(install.ReasonKitHash, "%s is not a plain file", f.Name)
		}
		size, sum, err := copyHashed(ctx, s.fs, join(src, f.Name), join(dst, f.Name))
		if err != nil {
			if ctx.Err() != nil {
				return install.KitCommand{}, ctx.Err()
			}
			return install.KitCommand{}, kitErr(install.ReasonKitFile, "%s: %v", f.Name, err)
		}
		if size != f.Size || sum != strings.ToLower(f.SHA256) {
			return install.KitCommand{}, kitErr(install.ReasonKitHash, "%s changed since the kit was opened", f.Name)
		}
		if strings.EqualFold(f.Name, e.Installer.File) {
			instSum = sum
		}
		if emit != nil {
			emit(fmt.Sprintf("%d%%", (i+1)*100/len(e.Files)))
		}
	}
	if e.Installer.ManifestSha256 != "" && !strings.EqualFold(e.Installer.ManifestSha256, instSum) {
		return install.KitCommand{}, kitErr(install.ReasonKitHash, "the installer is not the one the manifest pins")
	}
	cmd, err := BuildCommand(e.Installer, join(dst, e.Installer.File))
	if err != nil {
		return install.KitCommand{}, kitErr(install.ReasonKitUnsupported, "%v", err)
	}
	return install.KitCommand{Exe: cmd.Exe, Args: cmd.Args, OKCodes: e.Installer.SuccessCodes}, nil
}

// ReasonInvalid is a kit.json name that cannot be a path; it is reported like a changed file.
const ReasonInvalid = install.ReasonKitHash

func (s *Session) tempRoot() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tmp != "" {
		return s.tmp, nil
	}
	dir, err := s.fs.MkdirTemp("winforge-kit-*")
	if err != nil {
		return "", err
	}
	s.tmp = dir
	return dir, nil
}

// Done implements install.KitResolver: the private copy of one app goes.
func (s *Session) Done(appID string) {
	e, ok := s.apps[appID]
	s.mu.Lock()
	tmp := s.tmp
	s.mu.Unlock()
	if ok && tmp != "" && SafeName(e.Dir) {
		_ = s.fs.RemoveAll(join(tmp, e.Dir))
	}
}

// Close removes every private copy.
func (s *Session) Close() {
	s.mu.Lock()
	tmp := s.tmp
	s.tmp = ""
	s.mu.Unlock()
	if tmp != "" {
		_ = s.fs.RemoveAll(tmp)
	}
}

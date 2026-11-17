package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Sebas1705/WinForge/catalogdata"
	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/cliout"
	"github.com/Sebas1705/WinForge/internal/exitcode"
	"github.com/Sebas1705/WinForge/internal/install"
	"github.com/Sebas1705/WinForge/internal/profile"
	"github.com/Sebas1705/WinForge/internal/sched"
	"github.com/Sebas1705/WinForge/internal/system"
)

// deps is everything the commands need from the machine; tests fill it with fakes.
type deps struct {
	catalog  func() (*catalog.Catalog, error)
	snapshot func(context.Context) (system.Inventory, error)
	exec     install.Executor
	elevated func() bool
	lockPath func() (string, error)
	readFile func(string) ([]byte, error)
	health   func(ctx context.Context, withUpdates bool) (*healthOut, error)
	updates  func(args []string, o cliout.Options, out, errw io.Writer) int
}

func realDeps() deps {
	return deps{
		catalog:  func() (*catalog.Catalog, error) { return catalog.Load(catalogdata.FS) },
		snapshot: system.Snapshot,
		exec:     install.OSExecutor{},
		elevated: system.IsElevated,
		lockPath: func() (string, error) {
			dir, err := sched.DefaultDir() // the same lock as `updates`: two winget runs must not overlap
			return filepath.Join(dir, "run.lock"), err
		},
		readFile: os.ReadFile,
		health:   collectHealth,
		updates: func(args []string, o cliout.Options, out, errw io.Writer) int {
			return sched.RunCLIWith(args, o, out, errw, sched.Deps{})
		},
	}
}

// cmdCtx is one command's view of the run: where its output goes.
type cmdCtx struct {
	ctx  context.Context
	o    cliout.Options
	out  io.Writer
	errw io.Writer
	d    deps
	cmd  string
	cat  *catalog.Catalog
}

// printf writes a command's result for a person. With --json the document is the result, so
// nothing is printed here.
func (c *cmdCtx) printf(format string, a ...any) {
	if !c.o.JSON {
		fmt.Fprintf(c.out, format, a...)
	}
}

// logf writes progress and other information: stdout for a person, stderr with --json (so stdout
// stays one document), nothing with --quiet.
func (c *cmdCtx) logf(format string, a ...any) {
	switch {
	case c.o.Quiet:
	case c.o.JSON:
		fmt.Fprintf(c.errw, format, a...)
	default:
		fmt.Fprintf(c.out, format, a...)
	}
}

// problemf is logf for failures: --quiet still shows them (on stderr) when no JSON document says it.
func (c *cmdCtx) problemf(format string, a ...any) {
	switch {
	case c.o.JSON && c.o.Quiet:
	case c.o.JSON || c.o.Quiet:
		fmt.Fprintf(c.errw, format, a...)
	default:
		fmt.Fprintf(c.out, format, a...)
	}
}

func (c *cmdCtx) warnf(format string, a ...any) {
	if !c.o.Quiet {
		fmt.Fprintf(c.errw, format, a...)
	}
}

// done ends a command: it fills the envelope and, with --json, writes the document. msg is the
// sentence shown on stderr (and in error.message) when the code is a failure.
func (c *cmdCtx) done(code int, detail, msg string, env *cliout.Envelope, doc any) int {
	*env = cliout.NewEnvelope(c.cmd, code, detail, msg)
	if msg != "" && !exitcode.Succeeded(code) {
		fmt.Fprintln(c.errw, msg)
	}
	if c.o.JSON {
		_ = cliout.Write(c.out, doc)
	}
	return code
}

// fail ends a command that produced nothing: the document is only the envelope.
func (c *cmdCtx) fail(code int, detail, msg string) int {
	var doc struct{ cliout.Envelope }
	return c.done(code, detail, msg, &doc.Envelope, &doc)
}

func (c *cmdCtx) usageErr(msg string) int {
	if msg != "" {
		fmt.Fprintln(c.errw, msg)
	}
	fmt.Fprintln(c.errw, usageFor(c.cmd))
	var doc struct{ cliout.Envelope }
	doc.Envelope = cliout.NewEnvelope(c.cmd, exitcode.Usage, "", strings.TrimSpace(msg+" "+usageFor(c.cmd)))
	if c.o.JSON {
		_ = cliout.Write(c.out, &doc)
	}
	return exitcode.Usage
}

func (c *cmdCtx) timedOut() bool { return errors.Is(c.ctx.Err(), context.DeadlineExceeded) }

// run is the whole program: args are what follows the program name. It returns the exit code.
func run(ctx context.Context, args []string, out, errw io.Writer, d deps) int {
	rest, o, err := cliout.Split(args)
	if err != nil {
		fmt.Fprintln(errw, err)
		fmt.Fprintln(errw, usage)
		if hasFlag(args, "--json") {
			doc := struct{ cliout.Envelope }{cliout.NewEnvelope("", exitcode.Usage, "", err.Error())}
			_ = cliout.Write(out, doc)
		}
		return exitcode.Usage
	}
	c := &cmdCtx{ctx: ctx, o: o, out: out, errw: errw, d: d}
	if len(rest) == 0 {
		fmt.Fprintln(errw, usage)
		if o.JSON {
			doc := struct{ cliout.Envelope }{cliout.NewEnvelope("", exitcode.Usage, "", "no command given")}
			_ = cliout.Write(out, doc)
		}
		return exitcode.Usage
	}
	cmd := rest[0]
	if cmd == "--help" || cmd == "-h" || cmd == "help" {
		fmt.Fprintln(out, usage)
		return exitcode.OK
	}
	if !contains(commands, cmd) {
		fmt.Fprintln(errw, "unknown command", fmt.Sprintf("%q", cmd))
		fmt.Fprintln(errw, usage)
		if o.JSON {
			doc := struct{ cliout.Envelope }{cliout.NewEnvelope("", exitcode.Usage, "", "unknown command "+cmd)}
			_ = cliout.Write(out, doc)
		}
		return exitcode.Usage
	}
	c.cmd = cmd
	if hasFlag(rest[1:], "--help") || hasFlag(rest[1:], "-h") {
		fmt.Fprint(out, helpFor(cmd))
		return exitcode.OK
	}
	if cmd == "updates" { // before the catalog loads
		return d.updates(rest[1:], o, out, errw)
	}
	for _, a := range rest[1:] {
		if strings.HasPrefix(a, "-") {
			return c.usageErr(fmt.Sprintf("unknown option %q", a))
		}
	}
	if o.Timeout > 0 {
		var cancel context.CancelFunc
		c.ctx, cancel = context.WithTimeout(ctx, o.Timeout)
		defer cancel()
	}
	cat, err := d.catalog()
	if err != nil {
		return c.fail(exitcode.Data, "no-catalog", "could not read the catalog: "+err.Error())
	}
	c.cat = cat
	pargs := rest[1:]
	switch cmd {
	case "profiles":
		return c.profiles(pargs)
	case "scan":
		return c.scanCmd(pargs)
	case "health":
		return c.healthCmd(pargs)
	case "script", "configure", "plan", "apply":
		return c.profileCmd(pargs)
	case "export":
		return c.exportCmd(pargs)
	case "check":
		return c.checkCmd(pargs)
	}
	return exitcode.Usage
}

func hasFlag(args []string, f string) bool { return contains(args, f) }

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// snapshot takes the inventory. problem is a ready sentence when winget (or the inventory) was
// not usable; the inventory is still as good as registry and PATH detection can make it.
func (c *cmdCtx) detect() (installed map[string]system.Installed, snapErr error) {
	inv, err := c.d.snapshot(c.ctx)
	return system.Detect(c.cat, inv), err
}

// snapCode maps an inventory error to the exit code that reports it ("" detail: no problem).
func (c *cmdCtx) snapCode(err error) (code int, detail, msg string) {
	switch {
	case err == nil:
		return exitcode.OK, "", ""
	case c.timedOut() || errors.Is(err, context.DeadlineExceeded):
		return exitcode.Timeout, "timeout", "the time limit was reached while scanning this PC"
	case errors.Is(err, system.ErrNoWinget):
		return exitcode.Prereq, "no-winget", err.Error()
	}
	return exitcode.OK, "check-failed", err.Error() // degraded, still usable: a warning
}

// ---------------------------------------------------------------------------------------------
// profiles

// profilesDoc is the --json document of `profiles`: the envelope and
//
//	profiles[]  {id, kind, name}, sorted by id
type profilesDoc struct {
	cliout.Envelope
	Profiles []profileJSON `json:"profiles"`
}

type profileJSON struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func (c *cmdCtx) profiles(args []string) int {
	if len(args) > 0 {
		return c.usageErr("profiles takes no arguments")
	}
	ids := make([]string, 0, len(c.cat.Profiles))
	for id := range c.cat.Profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	doc := &profilesDoc{Profiles: []profileJSON{}}
	for _, id := range ids {
		p := c.cat.Profiles[id]
		c.printf("%-16s %-5s %s\n", id, p.Kind, p.Name)
		doc.Profiles = append(doc.Profiles, profileJSON{ID: id, Kind: string(p.Kind), Name: p.Name})
	}
	return c.done(exitcode.OK, "", "", &doc.Envelope, doc)
}

// ---------------------------------------------------------------------------------------------
// scan

// scanDoc is the --json document of `scan`: the envelope and
//
//	installed[]       {id, name, version, sources[]} for each catalog app found on this PC, sorted by id;
//	                  sources are winget | registry | path | file
//	installedCount    how many catalog apps are installed
//	catalogCount      how many apps the catalog has
//	warnings[]        sentences about a degraded scan (for example winget could not list apps)
//
// error.detail is "no-winget" (exit 4) when winget is missing: installed[] is still filled from the
// registry and PATH, but it can be incomplete.
type scanDoc struct {
	cliout.Envelope
	Installed      []installedJSON `json:"installed"`
	InstalledCount int             `json:"installedCount"`
	CatalogCount   int             `json:"catalogCount"`
	Warnings       []string        `json:"warnings"`
}

type installedJSON struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Sources []string `json:"sources"`
}

func (c *cmdCtx) scanCmd(args []string) int {
	if len(args) > 0 {
		return c.usageErr("scan takes no arguments")
	}
	installed, serr := c.detect()
	code, detail, msg := c.snapCode(serr)
	if code == exitcode.Timeout {
		return c.fail(code, detail, msg)
	}
	doc := &scanDoc{Installed: []installedJSON{}, Warnings: []string{}, CatalogCount: len(c.cat.Apps), InstalledCount: len(installed)}
	if serr != nil && code == exitcode.OK {
		c.warnf("warning: %v\n", serr)
		doc.Warnings = append(doc.Warnings, serr.Error())
	}
	for _, id := range sortedInstalled(installed) {
		i := installed[id]
		c.printf("%-28s %-16s %v\n", id, i.Version, i.Sources)
		name := id
		if a := c.cat.Apps[id]; a != nil {
			name = a.Name
		}
		doc.Installed = append(doc.Installed, installedJSON{ID: id, Name: name, Version: i.Version, Sources: append([]string{}, i.Sources...)})
	}
	c.printf("\n%d of %d catalog apps installed\n", len(installed), len(c.cat.Apps))
	return c.done(code, detail, msg, &doc.Envelope, doc)
}

func sortedInstalled(m map[string]system.Installed) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ---------------------------------------------------------------------------------------------
// script, configure, plan, apply

// scriptDoc is the --json document of `script`: the envelope and profile (the id), script (the
// PowerShell text).
type scriptDoc struct {
	cliout.Envelope
	Profile string `json:"profile"`
	Script  string `json:"script"`
}

// configureDoc is the --json document of `configure`: the envelope and profile (the id), format
// ("winget-configure") and content (the file's text).
type configureDoc struct {
	cliout.Envelope
	Profile string `json:"profile"`
	Format  string `json:"format"`
	Content string `json:"content"`
}

// planDoc is the --json document of `plan`: the envelope and
//
//	profile           the profile id
//	alreadyInstalled  ids of profile apps that need nothing
//	steps[]           {kind, id, name, version?, admin?} in the order they would run;
//	                  kind is app | recipe | upgrade | uninstall | rollback
//	skippedRecipes[]  recipes left out because the app they follow is neither installed nor planned
//	needsAdmin        true when some step needs administrator rights
type planDoc struct {
	cliout.Envelope
	Profile          string         `json:"profile"`
	AlreadyInstalled []string       `json:"alreadyInstalled"`
	Steps            []install.Step `json:"steps"`
	SkippedRecipes   []string       `json:"skippedRecipes"`
	NeedsAdmin       bool           `json:"needsAdmin"`
}

// applyDoc is the --json document of `apply`: the envelope and
//
//	profile           the profile id
//	alreadyInstalled  ids of profile apps that needed nothing
//	skippedRecipes[]  recipes left out (see plan)
//	needsAdmin        true when some step needs administrator rights
//	rebootRequired    true when a step finished but asks for a restart (exit code 8 when all else worked)
//	counts            {ok, failed, skipped}: how many steps ended each way
//	steps[]           one per planned step, in order:
//	                    id, kind (app | recipe | upgrade | uninstall | rollback), name,
//	                    result  ok | failed | skipped,
//	                    reason  for failed an install.Explain word (admin, busy, network, hash,
//	                            cancelled, disk, installer, ...); for skipped "dependency", "cancelled",
//	                            or what stopped the whole run (admin, prerequisite, busy); "reboot" is
//	                            not a reason here, see the reboot field,
//	                    message the installer's own error text when it failed (English, not for parsing),
//	                    from    the version installed before the step ("" when it was not installed),
//	                    to      the version installed after it ("" when removed or not installed),
//	                    reboot  true when the step finished but Windows has to restart,
//	                    admin   true when the step needs administrator rights
//
// When the run was refused before it started (exit 7, 4 or 3) every step is "skipped" with that reason.
type applyDoc struct {
	cliout.Envelope
	Profile          string         `json:"profile"`
	AlreadyInstalled []string       `json:"alreadyInstalled"`
	SkippedRecipes   []string       `json:"skippedRecipes"`
	NeedsAdmin       bool           `json:"needsAdmin"`
	RebootRequired   bool           `json:"rebootRequired"`
	Counts           map[string]int `json:"counts"`
	Steps            []stepJSON     `json:"steps"`
}

type stepJSON struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Result  string `json:"result"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	From    string `json:"from"`
	To      string `json:"to"`
	Reboot  bool   `json:"reboot"`
	Admin   bool   `json:"admin"`
}

func (c *cmdCtx) profileCmd(args []string) int {
	if len(args) != 1 {
		return c.usageErr("")
	}
	id := args[0]
	res, err := c.cat.Resolve(id)
	if err != nil {
		return c.usageErr(err.Error())
	}
	name := c.cat.Profiles[id].Name
	if c.cmd == "configure" {
		b, err := profile.WingetConfigure(name, res.Apps, res.Recipes, c.cat)
		if err != nil {
			return c.fail(exitcode.Failures, "", err.Error())
		}
		if !c.o.JSON {
			c.out.Write(b)
		}
		doc := &configureDoc{Profile: id, Format: "winget-configure", Content: string(b)}
		return c.done(exitcode.OK, "", "", &doc.Envelope, doc)
	}

	installed, serr := c.detect()
	code, detail, msg := c.snapCode(serr)
	if code == exitcode.Timeout {
		return c.fail(code, detail, msg)
	}
	if serr != nil && code == exitcode.OK {
		c.warnf("warning: %v\n", serr)
	}
	plan := install.BuildPlan(c.cat, res, installed)
	switch c.cmd {
	case "script":
		s := install.Script(c.cat, plan, name)
		c.printf("%s", s)
		doc := &scriptDoc{Profile: id, Script: s}
		return c.done(code, detail, msg, &doc.Envelope, doc)
	case "plan":
		c.printf("%d already installed, %d steps\n", len(plan.AlreadyInstalled), len(plan.Steps))
		for _, s := range plan.Steps {
			c.printf("  %-7s %-28s %s\n", s.Kind, s.ID, adminMark(s.Admin))
		}
		for _, r := range plan.SkippedRecipes {
			c.printf("  skipped recipe %s (its app is not part of this plan)\n", r)
		}
		doc := &planDoc{Profile: id, AlreadyInstalled: nz(plan.AlreadyInstalled), Steps: plan.Steps, SkippedRecipes: nz(plan.SkippedRecipes), NeedsAdmin: plan.NeedsAdmin}
		if doc.Steps == nil {
			doc.Steps = []install.Step{}
		}
		return c.done(code, detail, msg, &doc.Envelope, doc)
	}
	return c.apply(id, plan, installed, serr)
}

func nz(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func adminMark(a bool) string {
	if a {
		return "(needs admin)"
	}
	return ""
}

func stepKey(s install.Step) string { return string(s.Kind) + "/" + s.ID }

func (c *cmdCtx) apply(id string, plan install.Plan, installed map[string]system.Installed, serr error) int {
	doc := &applyDoc{Profile: id, AlreadyInstalled: nz(plan.AlreadyInstalled), SkippedRecipes: nz(plan.SkippedRecipes),
		NeedsAdmin: plan.NeedsAdmin, Counts: map[string]int{"ok": 0, "failed": 0, "skipped": 0}, Steps: []stepJSON{}}
	at := map[string]int{}
	needsWinget := false
	for _, s := range plan.Steps {
		at[stepKey(s)] = len(doc.Steps)
		from := ""
		if s.Kind != install.StepApp && s.Kind != install.StepRecipe {
			from = installed[s.ID].Version
		}
		doc.Steps = append(doc.Steps, stepJSON{ID: s.ID, Kind: string(s.Kind), Name: s.Name, Result: "skipped", From: from, To: from, Admin: s.Admin})
		needsWinget = needsWinget || s.Kind != install.StepRecipe
	}
	count := func() {
		doc.Counts = map[string]int{"ok": 0, "failed": 0, "skipped": 0}
		for _, st := range doc.Steps {
			doc.Counts[st.Result]++
		}
	}
	refuse := func(code int, reason, msg string) int {
		for i := range doc.Steps {
			doc.Steps[i].Reason = reason
		}
		count()
		return c.done(code, reason, msg, &doc.Envelope, doc)
	}

	if errors.Is(serr, system.ErrNoWinget) && needsWinget {
		return refuse(exitcode.Prereq, "prerequisite", serr.Error()+"; nothing was done")
	}
	if c.o.NonInteractive && plan.NeedsAdmin && !(c.d.elevated != nil && c.d.elevated()) {
		return refuse(exitcode.Admin, "admin", "this plan needs administrator rights and this run has none; nothing was done (start winforge-cli from an elevated terminal)")
	}
	if c.d.lockPath != nil {
		if p, err := c.d.lockPath(); err == nil {
			release, lerr := sched.Lock{Path: p}.Acquire()
			if errors.Is(lerr, sched.ErrBusy) {
				return refuse(exitcode.Busy, "busy", "another WinForge run is already in progress; nothing was done")
			}
			if lerr == nil {
				defer release()
			} else {
				c.warnf("warning: could not take the run lock: %v\n", lerr)
			}
		}
	}

	c.logf("%d already installed, %d steps\n", len(plan.AlreadyInstalled), len(plan.Steps))
	for _, s := range plan.Steps {
		c.logf("  %-7s %-28s %s\n", s.Kind, s.ID, adminMark(s.Admin))
	}
	for _, r := range plan.SkippedRecipes {
		c.logf("  skipped recipe %s (its app is not part of this plan)\n", r)
	}
	failed := install.Run(c.ctx, c.cat, plan, c.d.exec, func(e install.Event) {
		i, ok := at[stepKey(e.Step)]
		if !ok {
			return
		}
		st := &doc.Steps[i]
		switch e.Status {
		case install.StatusStart:
			c.logf("-> %s %s\n", e.Step.Kind, e.Step.ID)
		case install.StatusOutput:
			c.logf("   %s\n", e.Line)
		case install.StatusOK:
			st.Result, st.Reboot = "ok", e.Reason == install.ReasonReboot
		case install.StatusFailed:
			st.Result, st.Reason, st.Message = "failed", e.Reason, e.Err
			c.problemf("   %s: %s\n", e.Status, e.Err)
		case install.StatusSkipped:
			st.Result, st.Message = "skipped", e.Err
			st.Reason = "dependency"
			if e.Err == "cancelled" {
				st.Reason = "cancelled"
			}
			c.problemf("   %s: %s\n", e.Status, e.Err)
		}
	})
	if len(failed) > 0 {
		c.problemf("failed: %v\n", failed)
	}
	if c.o.JSON && c.ctx.Err() == nil {
		c.fillAfter(doc, plan, at)
	}
	count()
	for _, st := range doc.Steps {
		doc.RebootRequired = doc.RebootRequired || st.Reboot
	}
	switch {
	case c.timedOut():
		return c.done(exitcode.Timeout, "timeout", "the time limit was reached; the remaining steps were not run", &doc.Envelope, doc)
	case c.ctx.Err() != nil:
		return c.done(exitcode.Failures, "cancelled", "the run was cancelled", &doc.Envelope, doc)
	case len(failed) > 0:
		return c.done(exitcode.Failures, "", fmt.Sprintf("%d of %d steps failed", len(failed), len(plan.Steps)), &doc.Envelope, doc)
	case doc.RebootRequired:
		return c.done(exitcode.Reboot, "", "", &doc.Envelope, doc)
	}
	return c.done(exitcode.OK, "", "", &doc.Envelope, doc)
}

// fillAfter rescans once to report the version each step ended with. A failed scan only leaves the
// "to" of an app step empty.
func (c *cmdCtx) fillAfter(doc *applyDoc, plan install.Plan, at map[string]int) {
	done := false
	for _, st := range doc.Steps {
		done = done || st.Result == "ok"
	}
	if !done {
		return
	}
	after, err := c.detect()
	if err != nil && len(after) == 0 {
		return
	}
	for _, s := range plan.Steps {
		st := &doc.Steps[at[stepKey(s)]]
		if st.Result != "ok" {
			continue
		}
		switch s.Kind {
		case install.StepUninstall:
			st.To = ""
		case install.StepRecipe:
			st.To = ""
		default:
			st.To = after[s.ID].Version
			if st.To == "" {
				st.To = s.Version
			}
		}
	}
}

// ---------------------------------------------------------------------------------------------
// export, check

// exportDoc is the --json document of `export`: the envelope and profile, the exported profile
// file itself (the same JSON the plain command prints: {winforge, profile}).
type exportDoc struct {
	cliout.Envelope
	Profile json.RawMessage `json:"profile"`
}

func (c *cmdCtx) exportCmd(args []string) int {
	if len(args) < 1 || len(args) > 2 || (len(args) == 2 && args[1] != "pin") {
		return c.usageErr("")
	}
	installed, serr := c.detect()
	code, detail, msg := c.snapCode(serr)
	if code == exitcode.Timeout {
		return c.fail(code, detail, msg)
	}
	if serr != nil && code == exitcode.OK {
		c.warnf("warning: %v\n", serr)
	}
	versions := map[string]string{}
	for id, i := range installed {
		versions[id] = i.Version
	}
	p := profile.FromInstalled(args[0], args[0], versions, len(args) == 2)
	b, err := profile.Export(p)
	if err != nil {
		return c.fail(exitcode.Failures, "", err.Error())
	}
	if !c.o.JSON {
		c.out.Write(b)
	}
	doc := &exportDoc{Profile: json.RawMessage(b)}
	return c.done(code, detail, msg, &doc.Envelope, doc)
}

// checkDoc is the --json document of `check`: the envelope and
//
//	file            the path that was given
//	name            the profile's name
//	apps            how many apps it lists
//	unknownApps[]   ids this catalog does not know
//	unknownRecipes[]
//
// An unreadable or invalid file is exit 5 and the document is only the envelope.
type checkDoc struct {
	cliout.Envelope
	File           string   `json:"file"`
	Name           string   `json:"name"`
	Apps           int      `json:"apps"`
	UnknownApps    []string `json:"unknownApps"`
	UnknownRecipes []string `json:"unknownRecipes"`
}

func (c *cmdCtx) checkCmd(args []string) int {
	if len(args) != 1 {
		return c.usageErr("")
	}
	b, err := c.d.readFile(args[0])
	if err != nil {
		return c.fail(exitcode.Data, "unreadable", err.Error())
	}
	imp, err := profile.Import(b, c.cat)
	if err != nil {
		return c.fail(exitcode.Data, "invalid", err.Error())
	}
	c.printf("ok: %q, %d apps, %d unknown apps, %d unknown recipes\n",
		imp.Profile.Name, len(imp.Profile.Apps), len(imp.UnknownApps), len(imp.UnknownRecipes))
	doc := &checkDoc{File: args[0], Name: imp.Profile.Name, Apps: len(imp.Profile.Apps), UnknownApps: nz(imp.UnknownApps), UnknownRecipes: nz(imp.UnknownRecipes)}
	return c.done(exitcode.OK, "", "", &doc.Envelope, doc)
}

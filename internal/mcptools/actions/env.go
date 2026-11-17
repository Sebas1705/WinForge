package actions

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/envdoctor"
	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// ---- winforge_prepare_env_fix ----

type envFixArgs struct {
	FindingIDs []string `json:"findingIds"`
}

func (k *kit) envFixTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_env_fix",
		Title: "Prepare environment variable fixes",
		Description: "Prepares fixing findings of the environment doctor (a dead, duplicated, empty or relative PATH entry, a *_HOME variable that points " +
			"nowhere) by changing the signed-in user's own environment variables. A fresh environment scan runs now and only findings it lists as " +
			"fixable can be named in `findingIds`; machine-wide variables are never changed and findings about them are refused. The plan lists each " +
			"finding with what the fix does (remove one entry from the user PATH, or delete one user variable). Before writing, the previous value and " +
			"type of every changed variable is saved in a backup; the result carries its id and winforge_prepare_env_restore puts it back. " +
			"No administrator rights are needed. Programs that are already open keep the old variables until they are restarted. When the plan " +
			"runs it scans again and refuses, changing nothing, if any finding differs from the plan." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"findingIds":{"type":"array","items":{"type":"string","maxLength":2048},"minItems":1,"maxItems":100,"uniqueItems":true,"description":"Ids of fixable findings from a fresh environment scan."}},"required":["findingIds"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareEnvFix,
	}
}

type envFinding struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Scope   string   `json:"scope"`
	Subject string   `json:"subject"`
	Paths   []string `json:"paths"`
	Fix     string   `json:"fix"`
}

type envFixSummary struct {
	Headline string       `json:"headline"`
	Findings []envFinding `json:"findings"`
	Warnings []string     `json:"warnings"`
	Undo     string       `json:"undo"`
}

// envFingerprint is what a finding is, to prove at run time that it is still the same one.
type envFingerprint struct {
	Kind, Scope, Subject, FixKind string
	Paths                         []string
	Fixable                       bool
}

func fingerprint(f envdoctor.Finding) envFingerprint {
	return envFingerprint{Kind: f.Kind, Scope: string(f.Scope), Subject: f.Subject, FixKind: f.FixKind, Paths: append([]string{}, f.Paths...), Fixable: f.Fixable}
}

func (k *kit) fixText(f envdoctor.Finding) string {
	subject := k.redact(f.Subject)
	switch f.FixKind {
	case envdoctor.FixRemovePathEntry:
		return fmt.Sprintf("Remove the entry %q from the user PATH variable (every other entry stays exactly as written).", subject)
	case envdoctor.FixUnsetVariable:
		return fmt.Sprintf("Delete the user variable %s.", subject)
	}
	return "Fix of kind " + f.FixKind
}

func (k *kit) prepareEnvFix(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Env == nil {
		return notAvailable("the environment doctor")
	}
	var a envFixArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	ids := dedupe(a.FindingIDs)
	if len(ids) == 0 {
		return refuse("no finding was chosen")
	}
	rep, err := k.d.Env.Scan()
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(rep.Findings))
	for i, f := range rep.Findings {
		keys[i] = f.ID
	}
	var items []envFinding
	prints := map[string]envFingerprint{}
	var real []string // the scan's own ids, in the order asked
	for _, asked := range ids {
		// The person's name may have been redacted out of an id the model read; the scan's own form is used from here on.
		at, ok, ambiguous := k.match(asked, keys, identity)
		if ambiguous {
			return refuse("%q matches more than one finding; nothing was prepared", k.redact(asked))
		}
		if !ok {
			return refuse("%q is not a finding of the current environment scan", k.redact(asked))
		}
		f := rep.Findings[at]
		id := f.ID
		if _, dup := prints[id]; dup {
			continue
		}
		real = append(real, id)
		switch {
		case f.Scope != envdoctor.User:
			return refuse("%q is about a machine-wide variable; WinForge never changes those", k.redact(id))
		case !f.Fixable:
			return refuse("%q is reported but has no automatic fix", k.redact(id))
		}
		prints[id] = fingerprint(f)
		items = append(items, envFinding{ID: k.redact(id), Kind: f.Kind, Scope: string(f.Scope), Subject: k.redact(f.Subject), Paths: k.redactAll(f.Paths), Fix: k.fixText(f)})
	}
	sum := envFixSummary{
		Headline: fmt.Sprintf("Fix %d finding%s in the current user's environment variables", len(items), plural(len(items), "", "s")),
		Findings: items,
		Warnings: []string{"Only the signed-in user's variables change; machine-wide variables are never touched.", "Programs already open keep the old variables until they are restarted."},
		Undo:     "Yes. The previous values are saved in a backup (id in the result); winforge_prepare_env_restore puts them back.",
	}
	return k.store(c, "env-fix", sum, false, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		return k.runEnvFix(ctx, c, real, prints)
	})
}

type envFixResult struct {
	Applied  int      `json:"applied"`
	Failed   []string `json:"failed"`
	BackupID string   `json:"backupId"`
	Error    string   `json:"error,omitempty"`
	Undo     string   `json:"undo"`
}

func (k *kit) runEnvFix(ctx context.Context, c *mcpkit.Call, ids []string, prints map[string]envFingerprint) (*mcpkit.Result, error) {
	progress(c, 0, 1, "Scanning the environment again")
	rep, err := k.d.Env.Scan()
	if err != nil {
		return refuse("the environment could not be scanned again (%s). Nothing was changed", k.redact(err.Error()))
	}
	by := map[string]envdoctor.Finding{}
	for _, f := range rep.Findings {
		by[f.ID] = f
	}
	for _, id := range ids {
		f, ok := by[id]
		if !ok || !reflect.DeepEqual(fingerprint(f), prints[id]) {
			return refuse("the environment changed since the plan was made (finding %q is gone or different). Nothing was changed; prepare the plan again", k.redact(id))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	progress(c, 0, 1, "Fixing")
	res, err := k.d.Env.Fix(ids)
	out := envFixResult{Applied: res.Applied, Failed: k.redactAll(nonNilStrings(res.Failed)), BackupID: res.BackupID,
		Undo: "winforge_prepare_env_restore with the backup id puts the previous values back."}
	if err != nil {
		out.Error = k.redact(err.Error())
	}
	progress(c, 1, 1, "Done")
	return outcome(out, err != nil && res.Applied == 0), nil
}

// ---- winforge_prepare_env_restore ----

func (k *kit) envRestoreTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_env_restore",
		Title: "Prepare undoing an environment fix",
		Description: "Prepares writing back the user environment variables a saved backup holds, for the variables that backup changed and no others. " +
			"`backupId` must be an id the environment backups list shows now (it comes from the result of an environment fix). Variables changed " +
			"again since the fix are restored too and reported. Only the signed-in user's own variables; no administrator rights are needed. " +
			"Programs already open keep the variables they started with until restarted." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"backupId":{"type":"string","pattern":"^[0-9]{8}T[0-9]{6}Z(-[0-9]+)?$","description":"A backup id from an environment fix."}},"required":["backupId"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareEnvRestore,
	}
}

func (k *kit) findEnvBackup(id string) (envdoctor.Backup, bool, error) {
	list, err := k.d.Env.Backups()
	if err != nil {
		return envdoctor.Backup{}, false, err
	}
	for _, b := range list {
		if b.ID == id {
			return b, true, nil
		}
	}
	return envdoctor.Backup{}, false, nil
}

func (k *kit) prepareEnvRestore(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Env == nil {
		return notAvailable("the environment doctor")
	}
	var a backupArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	b, found, err := k.findEnvBackup(a.BackupID)
	if err != nil {
		return nil, err
	}
	if !found {
		return refuse("backup %q is not in the list of environment backups", a.BackupID)
	}
	sum := restoreSummary{
		Headline: fmt.Sprintf("Write back the previous value of %d user variable%s: %s", len(b.Changed), plural(len(b.Changed), "", "s"), strings.Join(k.redactAll(b.Changed), ", ")),
		BackupID: b.ID, Made: b.Time.UTC().Format(time.RFC3339), Items: k.redactAll(b.Changed),
		Note: "Each variable the backup recorded is set back to the value and type it had before the fix (or recreated if the fix deleted it). Variables changed again since are restored too and reported.",
		Undo: "Run the environment fix again if wanted; the restore itself is not saved as a new backup.",
	}
	id := b.ID
	return k.store(c, "env-restore", sum, false, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		if _, found, err := k.findEnvBackup(id); err != nil || !found {
			return refuse("backup %q is no longer in the list. Nothing was changed", id)
		}
		progress(c, 0, 1, "Restoring")
		res, err := k.d.Env.Restore(id)
		out := envRestoreResult{BackupID: id, Restored: nonNilStrings(k.redactAll(res.Restored)), ChangedSince: nonNilStrings(k.redactAll(res.ChangedSince))}
		if err != nil {
			out.Error = k.redact(err.Error())
		}
		return outcome(out, err != nil && len(res.Restored) == 0), nil
	})
}

type envRestoreResult struct {
	BackupID     string   `json:"backupId"`
	Restored     []string `json:"restored"`
	ChangedSince []string `json:"changedSince"`
	Error        string   `json:"error,omitempty"`
}

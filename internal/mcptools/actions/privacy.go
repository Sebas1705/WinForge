package actions

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/privacy"
)

// ---- winforge_prepare_privacy ----

type privacyArgs struct {
	Apply []string `json:"apply"`
	Reset []string `json:"reset"`
}

func (k *kit) privacyTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_privacy",
		Title: "Prepare privacy setting changes",
		Description: "Prepares switching WinForge's curated Windows privacy settings (advertising ID, tips and suggestions, and similar) to their " +
			"private value (`apply`) or giving them back to Windows (`reset`, the values are removed). Ids come from the privacy settings " +
			"list and are checked against it now. Only the signed-in user's own registry values (HKEY_CURRENT_USER) are written; no " +
			"administrator rights are needed. The plan lists every registry value that will change: path, name, old value -> new value, " +
			"and type. Before the first write the exact previous state of each changed value is saved in a backup; the result carries " +
			"the backup id, and winforge_prepare_privacy_restore puts it back exactly (values edited since are restored too and " +
			"reported). When the plan runs it reads the registry again and refuses, changing nothing, if any value differs from the plan." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{` +
			`"apply":{"type":"array","items":{"type":"string","maxLength":64},"maxItems":64,"uniqueItems":true,"description":"Privacy setting ids to switch to their private value."},` +
			`"reset":{"type":"array","items":{"type":"string","maxLength":64},"maxItems":64,"uniqueItems":true,"description":"Privacy setting ids to give back to Windows."}` +
			`},"additionalProperties":false}`),
		Class:   mcpkit.Prepare,
		Handler: k.preparePrivacy,
	}
}

type privacyChange struct {
	Setting string `json:"setting"`
	Action  string `json:"action"` // "apply" or "reset"
	Path    string `json:"path"`
	Name    string `json:"name"`
	Old     string `json:"old"` // "(does not exist)" when absent
	New     string `json:"new"` // "(value removed)" for a reset
	Type    string `json:"type"`
}

type privacySummary struct {
	Headline       string          `json:"headline"`
	Changes        []privacyChange `json:"changes"`
	AlreadyAsAsked []string        `json:"alreadyAsAsked,omitempty"`
	Undo           string          `json:"undo"`
}

func describeChanges(list []privacy.PlannedChange, action string) []privacyChange {
	out := make([]privacyChange, 0, len(list))
	for _, p := range list {
		pc := privacyChange{Setting: p.Tweak, Action: action, Path: p.Path, Name: p.Name, Old: "(does not exist)", New: "(value removed)", Type: p.OldType}
		if p.Existed {
			pc.Old = p.Old
		}
		if !p.Remove {
			pc.New, pc.Type = p.New, p.NewType
		}
		if pc.Type == "" {
			pc.Type = "-"
		}
		out = append(out, pc)
	}
	return out
}

func (k *kit) preparePrivacy(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Privacy == nil {
		return notAvailable("privacy settings")
	}
	var a privacyArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	apply, reset := dedupe(a.Apply), dedupe(a.Reset)
	if len(apply)+len(reset) == 0 {
		return refuse("no setting was chosen: give ids in `apply` or `reset`")
	}
	inReset := map[string]bool{}
	for _, id := range reset {
		inReset[id] = true
	}
	for _, id := range apply {
		if inReset[id] {
			return refuse("%q is in both apply and reset; choose one", id)
		}
	}
	list, err := k.d.Privacy.List()
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, t := range list {
		known[t.ID] = true
	}
	for _, id := range append(append([]string{}, apply...), reset...) {
		if !known[id] {
			return refuse("%q is not a privacy setting in the current list", id)
		}
	}
	changes, already, wantApply, wantReset, refusal := k.privacyPreview(apply, reset)
	if refusal != "" {
		return refuse("%s", refusal)
	}
	if len(changes) == 0 {
		return refuse("nothing to change: every chosen setting is already as asked (%s)", strings.Join(already, ", "))
	}
	sum := privacySummary{
		Headline:       fmt.Sprintf("Change %d registry value%s of the current user for %d privacy setting%s", len(changes), plural(len(changes), "", "s"), len(apply)+len(reset)-len(already), plural(len(apply)+len(reset)-len(already), "", "s")),
		Changes:        changes,
		AlreadyAsAsked: already,
		Undo:           "Yes. A backup of the exact previous state is saved before writing; its id is in the result and winforge_prepare_privacy_restore puts it back.",
	}
	return k.store(c, "privacy", sum, false, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		return k.runPrivacy(ctx, c, apply, reset, wantApply, wantReset)
	})
}

// privacyPreview reads what apply and reset would change now. It returns the summary lines, the ids
// already as asked and the engine's own planned changes (kept to compare when the plan runs); refusal
// is non-empty when the registry could not be read.
func (k *kit) privacyPreview(apply, reset []string) (changes []privacyChange, already []string, wantApply, wantReset []privacy.PlannedChange, refusal string) {
	changes = []privacyChange{}
	for _, part := range []struct {
		ids    []string
		action string
	}{{apply, privacy.ActionApply}, {reset, privacy.ActionReset}} {
		if len(part.ids) == 0 {
			continue
		}
		list, res, err := k.d.Privacy.Preview(part.ids, part.action)
		if err != nil {
			return nil, nil, nil, nil, "the settings could not be read: " + k.redact(err.Error())
		}
		if len(res.Failed) > 0 {
			return nil, nil, nil, nil, "the registry values of these settings could not be read, so no plan was made: " + strings.Join(res.Failed, ", ")
		}
		changes = append(changes, describeChanges(list, part.action)...)
		already = append(already, res.Skipped...)
		if part.action == privacy.ActionApply {
			wantApply = list
		} else {
			wantReset = list
		}
	}
	return changes, already, wantApply, wantReset, ""
}

type privacyResult struct {
	Applied   int      `json:"applied"`
	Reset     int      `json:"reset"`
	Skipped   []string `json:"skipped"`
	Failed    []string `json:"failed"`
	BackupIDs []string `json:"backupIds"`
	Error     string   `json:"error,omitempty"`
	Undo      string   `json:"undo"`
}

func (k *kit) runPrivacy(ctx context.Context, c *mcpkit.Call, apply, reset []string, wantApply, wantReset []privacy.PlannedChange) (*mcpkit.Result, error) {
	progress(c, 0, 2, "Reading the registry again")
	var nowApply, nowReset []privacy.PlannedChange
	if len(apply) > 0 {
		var err error
		if nowApply, _, err = k.d.Privacy.Preview(apply, privacy.ActionApply); err != nil {
			return refuse("the settings could not be read again (%s). Nothing was changed", k.redact(err.Error()))
		}
	}
	if len(reset) > 0 {
		var err error
		if nowReset, _, err = k.d.Privacy.Preview(reset, privacy.ActionReset); err != nil {
			return refuse("the settings could not be read again (%s). Nothing was changed", k.redact(err.Error()))
		}
	}
	if !samePlanned(nowApply, wantApply) || !samePlanned(nowReset, wantReset) {
		return refuse("the registry no longer matches the plan (a value changed since it was prepared). Nothing was changed; prepare the plan again")
	}
	out := privacyResult{Skipped: []string{}, Failed: []string{}, BackupIDs: []string{},
		Undo: "winforge_prepare_privacy_restore with the backup id puts the previous values back exactly."}
	failed := false
	step := 0.0
	for _, part := range []struct {
		ids    []string
		action string
	}{{apply, privacy.ActionApply}, {reset, privacy.ActionReset}} {
		if len(part.ids) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			out.Error = "cancelled before " + part.action + " ran"
			failed = out.Applied+out.Reset == 0
			break
		}
		progress(c, step, 2, "Writing the "+part.action+" changes")
		var res privacy.Result
		var err error
		if part.action == privacy.ActionApply {
			res, err = k.d.Privacy.Apply(part.ids)
			out.Applied += res.Applied
		} else {
			res, err = k.d.Privacy.Reset(part.ids)
			out.Reset += res.Applied
		}
		out.Skipped = append(out.Skipped, res.Skipped...)
		out.Failed = append(out.Failed, res.Failed...)
		if res.BackupID != "" {
			out.BackupIDs = append(out.BackupIDs, res.BackupID)
		}
		if err != nil {
			out.Error = k.redact(err.Error())
			failed = true
			break
		}
		step++
	}
	progress(c, 2, 2, "Done")
	return outcome(out, failed && out.Applied+out.Reset == 0), nil
}

func samePlanned(a, b []privacy.PlannedChange) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// ---- winforge_prepare_privacy_restore ----

type backupArgs struct {
	BackupID string `json:"backupId"`
}

func (k *kit) privacyRestoreTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_privacy_restore",
		Title: "Prepare undoing a privacy change",
		Description: "Prepares putting back exactly what one saved privacy backup holds, for the registry values that backup changed and no others " +
			"(a value that did not exist before is removed again; one that existed gets its original type and data back). `backupId` must be an id " +
			"the privacy backups list shows now (from the result of winforge_apply_plan for a privacy change); a backup already restored is " +
			"refused. Values edited since the change are restored too and reported. Only the signed-in user's own registry values; no administrator " +
			"rights are needed. A restore can be undone by changing the settings again." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"backupId":{"type":"string","pattern":"^[0-9]{8}T[0-9]{6}Z(-[0-9]+)?$","description":"A backup id from a privacy change."}},"required":["backupId"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.preparePrivacyRestore,
	}
}

type restoreSummary struct {
	Headline string   `json:"headline"`
	BackupID string   `json:"backupId"`
	Made     string   `json:"made"`
	Action   string   `json:"action,omitempty"`
	Items    []string `json:"items"`
	Note     string   `json:"note"`
	Undo     string   `json:"undo"`
}

func (k *kit) findPrivacyBackup(id string) (privacy.Backup, bool, error) {
	list, err := k.d.Privacy.Backups()
	if err != nil {
		return privacy.Backup{}, false, err
	}
	for _, b := range list {
		if b.ID == id {
			return b, true, nil
		}
	}
	return privacy.Backup{}, false, nil
}

func (k *kit) preparePrivacyRestore(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Privacy == nil {
		return notAvailable("privacy settings")
	}
	var a backupArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	b, found, err := k.findPrivacyBackup(a.BackupID)
	if err != nil {
		return nil, err
	}
	if !found {
		return refuse("backup %q is not in the list of privacy backups", a.BackupID)
	}
	if b.Restored != nil {
		return refuse("backup %q was already restored on %s; make a new change first if the person wants to undo again", a.BackupID, b.Restored.UTC().Format(time.RFC3339))
	}
	sum := restoreSummary{
		Headline: fmt.Sprintf("Put back the previous registry values of %d privacy setting%s", len(b.Tweaks), plural(len(b.Tweaks), "", "s")),
		BackupID: b.ID, Made: b.Time.UTC().Format(time.RFC3339), Action: b.Action, Items: b.Tweaks,
		Note: "Each value the backup recorded goes back to exactly what it held before that change; a value that did not exist is removed. Values edited since are restored too and reported.",
		Undo: "Change the settings again with winforge_prepare_privacy.",
	}
	id := b.ID
	return k.store(c, "privacy-restore", sum, false, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		cur, found, err := k.findPrivacyBackup(id)
		if err != nil {
			return refuse("the backups could not be read (%s). Nothing was changed", k.redact(err.Error()))
		}
		if !found || cur.Restored != nil {
			return refuse("backup %q is gone or was already restored. Nothing was changed", id)
		}
		progress(c, 0, 1, "Restoring")
		res, err := k.d.Privacy.Restore(id)
		out := privacyRestoreResult{BackupID: id, Restored: nonNilStrings(res.Restored), ChangedSince: nonNilStrings(res.ChangedSince), Failed: nonNilStrings(res.Failed),
			Undo: "Change the settings again with winforge_prepare_privacy."}
		if err != nil {
			out.Error = k.redact(err.Error())
		}
		return outcome(out, err != nil && len(res.Restored) == 0), nil
	})
}

type privacyRestoreResult struct {
	BackupID     string   `json:"backupId"`
	Restored     []string `json:"restored"`
	ChangedSince []string `json:"changedSince"`
	Failed       []string `json:"failed"`
	Error        string   `json:"error,omitempty"`
	Undo         string   `json:"undo"`
}

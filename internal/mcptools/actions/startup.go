package actions

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
	"github.com/Sebas1705/WinForge/internal/services"
	"github.com/Sebas1705/WinForge/internal/startup"
	"github.com/Sebas1705/WinForge/internal/tasks"
)

// ---- winforge_prepare_startup_toggle ----

type startupArgs struct {
	ID      string `json:"id"`
	Enabled *bool  `json:"enabled"`
}

func (k *kit) startupToggleTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_startup_toggle",
		Title: "Prepare switching a startup program",
		Description: "Prepares switching one program that starts with Windows on or off, the way Task Manager's Startup tab does (the StartupApproved flag). " +
			"`id` must be one the startup list shows now. Nothing is deleted: the Run value or the startup file stays, so the same tool with the " +
			"opposite `enabled` undoes it. Items that start for every user need administrator rights; when WinForge is not elevated they are " +
			"refused now. Asking for the state an item already has is refused. The plan names the program, its publisher and command, and " +
			"warns when it looks like a Windows or security component." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"id":{"type":"string","maxLength":512,"description":"Startup item id from the startup list."},"enabled":{"type":"boolean","description":"true to start with Windows, false to not."}},"required":["id","enabled"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareStartup,
	}
}

type startupSummary struct {
	Headline  string   `json:"headline"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Publisher string   `json:"publisher,omitempty"`
	Command   string   `json:"command"`
	Source    string   `json:"source"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Warnings  []string `json:"warnings,omitempty"`
	Undo      string   `json:"undo"`
}

func onOff(b bool) string {
	if b {
		return "starts with Windows"
	}
	return "does not start with Windows"
}

func (k *kit) findStartup(id string) (startup.Item, bool, error) {
	items, err := k.d.Startup.List()
	if err != nil {
		return startup.Item{}, false, err
	}
	for _, it := range items {
		if it.ID == id {
			return it, true, nil
		}
	}
	return startup.Item{}, false, nil
}

func (k *kit) prepareStartup(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Startup == nil {
		return notAvailable("the startup manager")
	}
	var a startupArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	if a.ID == "" || a.Enabled == nil {
		return refuse("id and enabled are both required")
	}
	enabled := *a.Enabled
	it, found, err := k.findStartup(a.ID)
	if err != nil {
		return nil, err
	}
	if !found {
		return refuse("%q is not in the startup list right now", k.redact(a.ID))
	}
	if it.Enabled == enabled {
		return refuse("%q already %s; nothing to change", it.Name, onOff(enabled))
	}
	if it.NeedsAdmin && !k.elevated() {
		return refuse("%q starts for every user, so switching it needs administrator rights and WinForge is not running elevated. Nothing was prepared", it.Name)
	}
	var warn []string
	if it.Caution {
		warn = append(warn, "This looks like a Windows or security component; switching it off can weaken the PC.")
	}
	if it.Missing {
		warn = append(warn, "The program it starts is not there any more.")
	}
	sum := startupSummary{
		Headline:  fmt.Sprintf("Switch %q %s", it.Name, map[bool]string{true: "on", false: "off"}[enabled]),
		ID:        it.ID,
		Name:      k.redact(it.Name),
		Publisher: it.Publisher,
		Command:   k.redact(it.Command),
		Source:    it.Source,
		From:      onOff(it.Enabled),
		To:        onOff(enabled),
		Warnings:  warn,
		Undo:      fmt.Sprintf("Yes: winforge_prepare_startup_toggle with the same id and enabled=%v. Nothing is deleted.", it.Enabled),
	}
	id, before := it.ID, it
	return k.store(c, "startup-toggle", sum, it.NeedsAdmin, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		cur, found, err := k.findStartup(id)
		if err != nil {
			return refuse("the startup list could not be read again (%s). Nothing was changed", k.redact(err.Error()))
		}
		if !found || cur.Enabled != before.Enabled || cur.Command != before.Command || cur.NeedsAdmin != before.NeedsAdmin {
			return refuse("the startup item changed or is gone since the plan was made. Nothing was changed; prepare the plan again")
		}
		if cur.NeedsAdmin && !k.elevated() {
			return refuse("administrator rights are needed and WinForge no longer has them. Nothing was changed")
		}
		progress(c, 0, 1, "Switching")
		if err := k.d.Startup.Toggle(id, enabled); err != nil {
			return outcome(simpleResult{ID: it.ID, Name: k.redact(it.Name), Changed: false, Error: k.redact(err.Error())}, true), nil
		}
		return outcome(simpleResult{ID: it.ID, Name: k.redact(it.Name), Changed: true, Now: onOff(enabled),
			Undo: fmt.Sprintf("winforge_prepare_startup_toggle with the same id and enabled=%v.", before.Enabled)}, false), nil
	})
}

// simpleResult is the answer of the one-item toggles.
type simpleResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Changed bool   `json:"changed"`
	Now     string `json:"now,omitempty"`
	Error   string `json:"error,omitempty"`
	Undo    string `json:"undo,omitempty"`
}

// ---- services ----

type serviceModeArgs struct {
	ID   string `json:"id"`
	Mode string `json:"mode"`
}

func (k *kit) serviceModeTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_service_mode",
		Title: "Prepare changing a service's start mode",
		Description: "Prepares changing only the start mode of one third-party Windows service to auto, delayed, manual or disabled. `id` must be a service the " +
			"services list shows now; Microsoft, driver and protected services are never listed and are refused. The service is not stopped, started, " +
			"deleted or edited. Needs administrator rights: refused now when WinForge is not elevated. The plan lists services that depend on it " +
			"(lowering the mode can break them) and warns when it looks like a security product; the person's approval of the plan is the " +
			"confirmation the engine asks for. WinForge remembers the original start mode, and winforge_prepare_service_restore puts it back." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"id":{"type":"string","maxLength":256,"description":"Service name from the services list."},"mode":{"type":"string","enum":["auto","delayed","manual","disabled"]}},"required":["id","mode"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareServiceMode,
	}
}

func (k *kit) serviceRestoreTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_service_restore",
		Title: "Prepare restoring a service's original start mode",
		Description: "Prepares putting one service back to the start mode it had before WinForge first changed it. `id` must be a listed service WinForge " +
			"changed (the list marks it as changed and shows the original). Same rules as winforge_prepare_service_mode: administrator rights, " +
			"dependents and security products are listed in the plan, nothing is stopped or deleted." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"id":{"type":"string","maxLength":256,"description":"Service name from the services list."}},"required":["id"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareServiceRestore,
	}
}

func modeText(m services.Mode) string {
	switch m {
	case services.Auto:
		return "Automatic"
	case services.Delayed:
		return "Automatic (delayed start)"
	case services.Manual:
		return "Manual"
	case services.Disabled:
		return "Disabled"
	}
	return string(m)
}

// modeRank orders modes by how eagerly the service starts, as the services engine does: lowering it is
// what can break services that need this one.
func modeRank(m services.Mode) int {
	switch m {
	case services.Auto, services.Delayed:
		return 2
	case services.Manual:
		return 1
	}
	return 0
}

func (k *kit) findService(id string) (services.Item, bool, error) {
	items, err := k.d.Services.List()
	if err != nil {
		return services.Item{}, false, err
	}
	for _, it := range items {
		if strings.EqualFold(it.ID, id) {
			return it, true, nil
		}
	}
	return services.Item{}, false, nil
}

type serviceSummary struct {
	Headline   string   `json:"headline"`
	ID         string   `json:"id"`
	Display    string   `json:"displayName"`
	Publisher  string   `json:"publisher,omitempty"`
	Program    string   `json:"program"`
	State      string   `json:"state"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	Dependents []string `json:"dependents"`
	Security   bool     `json:"looksLikeSecurityProduct"`
	Warnings   []string `json:"warnings,omitempty"`
	Undo       string   `json:"undo"`
}

func (k *kit) prepareServiceMode(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Services == nil {
		return notAvailable("the services manager")
	}
	var a serviceModeArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	mode := services.Mode(a.Mode)
	if !mode.Valid() {
		return refuse("mode must be one of auto, delayed, manual, disabled")
	}
	return k.planService(c, a.ID, mode, false)
}

func (k *kit) prepareServiceRestore(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Services == nil {
		return notAvailable("the services manager")
	}
	var a struct {
		ID string `json:"id"`
	}
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	return k.planService(c, a.ID, "", true)
}

// planService builds the plan of a mode change, or of a restore when restore is true.
func (k *kit) planService(c *mcpkit.Call, id string, mode services.Mode, restore bool) (*mcpkit.Result, error) {
	if id == "" {
		return refuse("id is required")
	}
	it, found, err := k.findService(id)
	if err != nil {
		return nil, err
	}
	if !found {
		return refuse("%q is not in the services list right now (hidden, protected and Microsoft services are never listed)", k.redact(id))
	}
	if restore {
		if !it.Changed || !it.Original.Valid() {
			return refuse("WinForge has not changed %q, so there is no original start mode to restore", it.DisplayName)
		}
		mode = it.Original
	}
	if it.StartMode == mode {
		return refuse("%q already has the start mode %s; nothing to change", it.DisplayName, modeText(mode))
	}
	if !k.elevated() {
		return refuse("changing a service's start mode needs administrator rights and WinForge is not running elevated. Nothing was prepared")
	}
	lowering := modeRank(mode) < modeRank(it.StartMode)
	var warn []string
	confirm := false
	if it.Caution {
		confirm = true
		warn = append(warn, "This service looks like a security product; changing it can leave the PC less protected.")
	}
	if lowering && len(it.Dependents) > 0 {
		confirm = true
		warn = append(warn, "Other services need this one and may stop working: "+strings.Join(k.redactAll(it.Dependents), ", ")+".")
	}
	sum := serviceSummary{
		Headline: fmt.Sprintf("Change the start mode of the service %q from %s to %s", it.DisplayName, modeText(it.StartMode), modeText(mode)),
		ID:       it.ID, Display: k.redact(it.DisplayName), Publisher: it.Publisher, Program: k.redact(it.Binary), State: string(it.State),
		From: modeText(it.StartMode), To: modeText(mode), Dependents: nonNilStrings(k.redactAll(it.Dependents)), Security: it.Caution, Warnings: warn,
		Undo: "Yes: winforge_prepare_service_restore puts back the remembered original start mode (or call winforge_prepare_service_mode with the previous mode). The service is not stopped or started.",
	}
	if restore {
		sum.Headline = fmt.Sprintf("Restore the original start mode of the service %q: %s to %s", it.DisplayName, modeText(it.StartMode), modeText(mode))
	}
	before := it
	return k.store(c, "service-mode", sum, true, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		cur, found, err := k.findService(before.ID)
		if err != nil {
			return refuse("the services could not be read again (%s). Nothing was changed", k.redact(err.Error()))
		}
		if !found || cur.StartMode != before.StartMode || cur.Caution != before.Caution || !reflect.DeepEqual(cur.Dependents, before.Dependents) {
			return refuse("the service changed or is gone since the plan was made (start mode, dependents or classification differ). Nothing was changed; prepare the plan again")
		}
		if restore && (!cur.Changed || cur.Original != mode) {
			return refuse("the remembered original start mode changed since the plan was made. Nothing was changed; prepare the plan again")
		}
		if !k.elevated() {
			return refuse("administrator rights are needed and WinForge no longer has them. Nothing was changed")
		}
		progress(c, 0, 1, "Changing the start mode")
		// The person approved the plan, which listed the security-product and dependents warnings, so the
		// engine's confirmation is given exactly when the plan carried one.
		if restore {
			err = k.d.Services.Restore(before.ID, confirm)
		} else {
			err = k.d.Services.SetMode(before.ID, mode, confirm)
		}
		res := serviceResult{ID: before.ID, Name: k.redact(before.DisplayName), From: modeText(before.StartMode), To: modeText(mode)}
		if err != nil {
			res.Error = k.redact(err.Error())
			return outcome(res, true), nil
		}
		res.Changed = true
		res.Undo = "winforge_prepare_service_restore puts back the original start mode " + modeText(before.StartMode) + " (WinForge remembers it)."
		if restore {
			res.Undo = "winforge_prepare_service_mode can set it again."
		}
		return outcome(res, false), nil
	})
}

type serviceResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Changed bool   `json:"changed"`
	From    string `json:"from"`
	To      string `json:"to"`
	Error   string `json:"error,omitempty"`
	Undo    string `json:"undo,omitempty"`
}

// ---- scheduled tasks ----

type taskToggleArgs struct {
	ID      string `json:"id"`
	Enabled *bool  `json:"enabled"`
}

func (k *kit) taskToggleTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_task_toggle",
		Title: "Prepare enabling or disabling a scheduled task",
		Description: "Prepares enabling or disabling one third-party scheduled task that starts at log on or at startup. `id` must be a task the tasks list shows " +
			"now; anything under \\Microsoft\\ and Windows' own tasks are never listed and are refused. The task is never deleted, run or edited. Tasks " +
			"other than the user's own limited ones need administrator rights and are refused now when WinForge is not elevated. Disabling a security " +
			"product's task is warned about in the plan; the person's approval is the confirmation. WinForge remembers the original state and " +
			"winforge_prepare_task_restore puts it back." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"id":{"type":"string","maxLength":512,"description":"Task id (folder and name, for example \\Vendor\\Updater) from the tasks list."},"enabled":{"type":"boolean"}},"required":["id","enabled"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareTaskToggle,
	}
}

func (k *kit) taskRestoreTool() mcpkit.Tool {
	return mcpkit.Tool{
		Name:  "winforge_prepare_task_restore",
		Title: "Prepare restoring a scheduled task's original state",
		Description: "Prepares putting one scheduled task back to the enabled or disabled state it had before WinForge first changed it. `id` must be a listed task " +
			"WinForge changed. Same rules as winforge_prepare_task_toggle: administrator rights where the task needs them, warnings in the plan, " +
			"nothing deleted, run or edited." + prepNote,
		InputSchema: mcpkit.Schema(`{"type":"object","properties":{"id":{"type":"string","maxLength":512,"description":"Task id from the tasks list."}},"required":["id"],"additionalProperties":false}`),
		Class:       mcpkit.Prepare,
		Handler:     k.prepareTaskRestore,
	}
}

func (k *kit) findTask(id string) (tasks.Item, bool, error) {
	items, err := k.d.Tasks.List()
	if err != nil {
		return tasks.Item{}, false, err
	}
	for _, it := range items {
		if strings.EqualFold(it.ID, id) {
			return it, true, nil
		}
	}
	return tasks.Item{}, false, nil
}

func (k *kit) prepareTaskToggle(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Tasks == nil {
		return notAvailable("the scheduled tasks manager")
	}
	var a taskToggleArgs
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	if a.ID == "" || a.Enabled == nil {
		return refuse("id and enabled are both required")
	}
	return k.planTask(c, a.ID, *a.Enabled, false)
}

func (k *kit) prepareTaskRestore(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	if k.d.Tasks == nil {
		return notAvailable("the scheduled tasks manager")
	}
	var a struct {
		ID string `json:"id"`
	}
	if r, ok := decode(c, &a); !ok {
		return r, nil
	}
	return k.planTask(c, a.ID, false, true)
}

type taskSummary struct {
	Headline string   `json:"headline"`
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Triggers []string `json:"triggers"`
	Program  string   `json:"program"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Security bool     `json:"looksLikeSecurityProduct"`
	Warnings []string `json:"warnings,omitempty"`
	Undo     string   `json:"undo"`
}

func enabledText(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func (k *kit) planTask(c *mcpkit.Call, id string, enabled, restore bool) (*mcpkit.Result, error) {
	if id == "" {
		return refuse("id is required")
	}
	it, found, err := k.findTask(id)
	if err != nil {
		return nil, err
	}
	if !found {
		return refuse("%q is not in the scheduled tasks list right now (Microsoft and Windows tasks are never listed)", k.redact(id))
	}
	if restore {
		if !it.Changed || (it.Original != "enabled" && it.Original != "disabled") {
			return refuse("WinForge has not changed %q, so there is no original state to restore", it.Name)
		}
		enabled = it.Original == "enabled"
	}
	if it.Enabled == enabled {
		return refuse("%q is already %s; nothing to change", it.Name, enabledText(enabled))
	}
	if it.NeedsAdmin && !k.elevated() {
		return refuse("%q needs administrator rights to change and WinForge is not running elevated. Nothing was prepared", it.Name)
	}
	confirm := !enabled && it.Caution
	var warn []string
	if confirm {
		warn = append(warn, "This task looks like a security product's; disabling it can leave the PC less protected.")
	}
	sum := taskSummary{
		Headline: fmt.Sprintf("Set the scheduled task %q from %s to %s", it.Name, enabledText(it.Enabled), enabledText(enabled)),
		ID:       it.ID, Name: k.redact(it.Name), Triggers: nonNilStrings(it.Triggers), Program: k.redact(it.Action),
		From: enabledText(it.Enabled), To: enabledText(enabled), Security: it.Caution, Warnings: warn,
		Undo: "Yes: winforge_prepare_task_restore puts back the remembered original state (or call winforge_prepare_task_toggle with the opposite value). The task is never deleted, run or edited.",
	}
	before := it
	return k.store(c, "task-toggle", sum, it.NeedsAdmin, func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
		cur, found, err := k.findTask(before.ID)
		if err != nil {
			return refuse("the tasks could not be read again (%s). Nothing was changed", k.redact(err.Error()))
		}
		if !found || cur.Enabled != before.Enabled || cur.Caution != before.Caution || cur.NeedsAdmin != before.NeedsAdmin || cur.Action != before.Action {
			return refuse("the task changed or is gone since the plan was made. Nothing was changed; prepare the plan again")
		}
		if restore && (!cur.Changed || (cur.Original == "enabled") != enabled) {
			return refuse("the remembered original state changed since the plan was made. Nothing was changed; prepare the plan again")
		}
		if cur.NeedsAdmin && !k.elevated() {
			return refuse("administrator rights are needed and WinForge no longer has them. Nothing was changed")
		}
		progress(c, 0, 1, "Changing the task")
		if restore {
			err = k.d.Tasks.Restore(before.ID, confirm)
		} else {
			err = k.d.Tasks.SetEnabled(before.ID, enabled, confirm)
		}
		res := simpleResult{ID: before.ID, Name: k.redact(before.Name)}
		if err != nil {
			res.Error = k.redact(err.Error())
			return outcome(res, true), nil
		}
		res.Changed, res.Now = true, enabledText(enabled)
		res.Undo = "winforge_prepare_task_toggle with enabled=" + fmt.Sprint(before.Enabled) + ", or winforge_prepare_task_restore."
		return outcome(res, false), nil
	})
}

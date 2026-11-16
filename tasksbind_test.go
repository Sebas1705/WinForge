package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/tasks"
)

func taskXML(author, triggers, user, command string) string {
	p := `<Principal id="Author"><GroupId>S-1-5-32-545</GroupId></Principal>`
	if user != "" {
		p = `<Principal id="Author"><UserId>` + user + `</UserId><RunLevel>LeastPrivilege</RunLevel></Principal>`
	}
	return `<?xml version="1.0" encoding="UTF-16"?><Task xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><RegistrationInfo><Author>` + author +
		`</Author></RegistrationInfo><Triggers>` + triggers + `</Triggers><Principals>` + p + `</Principals><Actions><Exec><Command>` + command + `</Command></Exec></Actions></Task>`
}

func taskApp(t *testing.T) (*App, *tasks.Fake) {
	f := tasks.NewFake()
	f.Tasks = []tasks.Raw{
		{Folder: `\`, Name: "GoogleUpdateTaskMachineCore", State: 3, XML: taskXML("Google LLC", `<LogonTrigger/>`, "", `C:\Program Files\Google\Update\GoogleUpdate.exe`)},
		{Folder: `\`, Name: "MyBackup", State: 3, XML: taskXML("me", `<LogonTrigger/>`, `PC\sebas`, `C:\Backup\b.exe`)},
		{Folder: `\`, Name: "Avast Update", State: 3, XML: taskXML("AVAST Software", `<BootTrigger/>`, "", `C:\Program Files\Avast\up.exe`)},
		{Folder: `\Microsoft\Windows\Defrag\`, Name: "ScheduledDefrag", State: 3, XML: taskXML("Microsoft", `<BootTrigger/>`, "", `C:\Windows\system32\defrag.exe`)},
		{Folder: `\`, Name: "WinForge updates", State: 3, XML: taskXML("WinForge", `<LogonTrigger/>`, "", `C:\WinForge\w.exe`)},
	}
	a := &App{}
	dir := t.TempDir()
	a.svc.tasksSys = f
	a.svc.stateDir = func() (string, error) { return dir, nil }
	a.elevated = func() bool { return true }
	return a, f
}

func taskIDs(items []tasks.Item) string {
	var ids []string
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return strings.Join(ids, ",")
}

func TestTasksListHidesMicrosoftAndOwnTask(t *testing.T) {
	a, _ := taskApp(t)
	items, err := a.TasksList()
	if err != nil {
		t.Fatal(err)
	}
	if got := taskIDs(items); got != `\Avast Update,\GoogleUpdateTaskMachineCore,\MyBackup` {
		t.Fatalf("ids = %s", got)
	}
}

func TestTasksSetEnabledRefusesWhatWasNotListed(t *testing.T) {
	a, f := taskApp(t)
	if _, err := a.TasksSetEnabled(`\MyBackup`, false, false); !errors.Is(err, errTaskNotListed) {
		t.Fatalf("before any list: %v", err)
	}
	_, _ = a.TasksList()
	for _, id := range []string{`\Microsoft\Windows\Defrag\ScheduledDefrag`, `\WinForge updates`, `\winforge updates`, `\Nope`, ``} {
		if _, err := a.TasksSetEnabled(id, false, true); !errors.Is(err, errTaskNotListed) {
			t.Fatalf("%q: %v", id, err)
		}
		if _, err := a.TasksRestore(id, true); !errors.Is(err, errTaskNotListed) {
			t.Fatalf("restore %q: %v", id, err)
		}
	}
	if f.WriteCount() != 0 {
		t.Fatal("nothing may be written")
	}
}

func TestTasksSetEnabledRechecksProtectionAtWriteTime(t *testing.T) {
	a, f := taskApp(t)
	_, _ = a.TasksList()
	// The task is moved under \Microsoft\ between the list and the change.
	f.Tasks[1].Folder = `\Microsoft\Windows\`
	if _, err := a.TasksSetEnabled(`\MyBackup`, false, true); !errors.Is(err, tasks.ErrNotFound) {
		t.Fatalf("err = %v", err) // the id (old folder) is gone
	}
	f.Tasks[0].XML = taskXML("Microsoft Corporation", `<LogonTrigger/>`, "", `C:\x.exe`)
	if _, err := a.TasksSetEnabled(`\GoogleUpdateTaskMachineCore`, false, true); !errors.Is(err, tasks.ErrProtected) {
		t.Fatalf("err = %v", err)
	}
	if f.WriteCount() != 0 {
		t.Fatal("nothing may be written")
	}
}

func TestTasksSetEnabledAndRestore(t *testing.T) {
	a, f := taskApp(t)
	_, _ = a.TasksList()
	items, err := a.TasksSetEnabled(`\GoogleUpdateTaskMachineCore`, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID == `\GoogleUpdateTaskMachineCore` && (it.Enabled || !it.Changed || it.Original != "enabled" || it.State != tasks.Disabled) {
			t.Fatalf("item = %+v", it)
		}
	}
	if f.WriteCount() != 1 {
		t.Fatalf("writes = %v", f.Writes)
	}
	items, err = a.TasksRestore(`\googleupdatetaskmachinecore`, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID == `\GoogleUpdateTaskMachineCore` && (!it.Enabled || it.Changed) {
			t.Fatalf("restored = %+v", it)
		}
	}
}

func TestTasksAdminAndConfirmation(t *testing.T) {
	a, f := taskApp(t)
	_, _ = a.TasksList()
	a.elevated = func() bool { return false }
	if _, err := a.TasksSetEnabled(`\GoogleUpdateTaskMachineCore`, false, false); !errors.Is(err, tasks.ErrNeedsAdmin) {
		t.Fatalf("admin: %v", err)
	}
	// The signed-in user's own limited task needs no administrator.
	if _, err := a.TasksSetEnabled(`\MyBackup`, false, false); err != nil {
		t.Fatalf("per-user: %v", err)
	}
	a.elevated = func() bool { return true }
	if _, err := a.TasksSetEnabled(`\Avast Update`, false, false); !errors.Is(err, tasks.ErrNeedsConfirm) {
		t.Fatalf("security: %v", err)
	}
	if f.WriteCount() != 1 {
		t.Fatalf("writes = %v", f.Writes)
	}
	if _, err := a.TasksSetEnabled(`\Avast Update`, false, true); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
}

func TestTasksListErrorIsReturned(t *testing.T) {
	a, _ := taskApp(t)
	a.svc.tasksSys = taskBroken{tasks.NewFake()}
	if _, err := a.TasksList(); err == nil {
		t.Fatal("want the read error")
	}
}

type taskBroken struct{ *tasks.Fake }

func (taskBroken) List() ([]tasks.Raw, error) { return nil, errors.New("scheduler unavailable") }

package tasks

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/origstate"
)

// xmlOf builds a task definition the way Export-ScheduledTask writes it.
func xmlOf(author, triggers, userID, runLevel, command string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-16"?>` + "\n")
	b.WriteString(`<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">`)
	b.WriteString(`<RegistrationInfo><Author>` + author + `</Author></RegistrationInfo>`)
	b.WriteString(`<Triggers>` + triggers + `</Triggers>`)
	b.WriteString(`<Principals><Principal id="Author">`)
	if userID != "" {
		b.WriteString(`<UserId>` + userID + `</UserId><LogonType>InteractiveToken</LogonType>`)
	} else {
		b.WriteString(`<GroupId>S-1-5-32-545</GroupId>`)
	}
	if runLevel != "" {
		b.WriteString(`<RunLevel>` + runLevel + `</RunLevel>`)
	}
	b.WriteString(`</Principal></Principals>`)
	b.WriteString(`<Settings><Enabled>true</Enabled></Settings>`)
	b.WriteString(`<Actions Context="Author"><Exec><Command>` + command + `</Command><Arguments>--silent</Arguments></Exec></Actions>`)
	b.WriteString(`</Task>`)
	return b.String()
}

const (
	logon = `<LogonTrigger><Enabled>true</Enabled></LogonTrigger>`
	boot  = `<BootTrigger><Delay>PT30S</Delay></BootTrigger>`
	timeT = `<TimeTrigger><StartBoundary>2024-01-01T00:00:00</StartBoundary></TimeTrigger>`
)

func TestParseXMLTriggers(t *testing.T) {
	cases := []struct {
		name     string
		triggers string
		want     []string
	}{
		{"logon", logon, []string{TriggerLogon}},
		{"logon without Enabled element", `<LogonTrigger/>`, []string{TriggerLogon}},
		{"boot", boot, []string{TriggerBoot}},
		{"both", logon + boot, []string{TriggerLogon, TriggerBoot}},
		{"duplicates collapse", logon + logon, []string{TriggerLogon}},
		{"time only", timeT, nil},
		{"mixed with time", timeT + logon, []string{TriggerLogon}},
		{"disabled logon", `<LogonTrigger><Enabled>false</Enabled></LogonTrigger>`, nil},
		{"disabled logon but enabled boot", `<LogonTrigger><Enabled>false</Enabled></LogonTrigger>` + boot, []string{TriggerBoot}},
		{"idle, event, registration are not startup", `<IdleTrigger/><EventTrigger/><RegistrationTrigger/><SessionStateChangeTrigger/>`, nil},
		{"none", ``, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := ParseXML(xmlOf("Acme", c.triggers, "", "", `C:\x.exe`))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(p.Triggers, c.want) {
				t.Fatalf("triggers = %v, want %v", p.Triggers, c.want)
			}
		})
	}
}

func TestParseXMLFields(t *testing.T) {
	p, err := ParseXML(xmlOf("ACME\\admin", logon, `PC\sebas`, "HighestAvailable", `"C:\Program Files\Acme\up.exe"`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Author != `ACME\admin` || p.UserID != `PC\sebas` || p.RunLevel != "HighestAvailable" || p.Command != `"C:\Program Files\Acme\up.exe"` || p.Arguments != "--silent" {
		t.Fatalf("parsed = %+v", p)
	}
	g, _ := ParseXML(xmlOf("A", logon, "", "", `x.exe`))
	if g.GroupID != "S-1-5-32-545" || g.UserID != "" {
		t.Fatalf("group principal = %+v", g)
	}
}

func TestParseXMLToleratesDeclarationsAndBOM(t *testing.T) {
	body := `<Task xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><Triggers><LogonTrigger/></Triggers><Actions><Exec><Command>a.exe</Command></Exec></Actions></Task>`
	for name, in := range map[string]string{
		"utf-16 declaration": `<?xml version="1.0" encoding="UTF-16"?>` + body,
		"utf-8 declaration":  `<?xml version="1.0" encoding="UTF-8"?>` + "\r\n" + body,
		"bom":                "\ufeff" + body,
		"none":               body,
	} {
		p, err := ParseXML(in)
		if err != nil || !reflect.DeepEqual(p.Triggers, []string{TriggerLogon}) {
			t.Errorf("%s: %v %v", name, p, err)
		}
	}
}

func TestParseXMLErrors(t *testing.T) {
	for _, in := range []string{"", "   ", "not xml", "<Task><Triggers></Task>"} {
		if _, err := ParseXML(in); err == nil {
			t.Errorf("ParseXML(%q) should fail", in)
		}
	}
}

func TestParseXMLLocalizedContent(t *testing.T) {
	// Tag names never change with the Windows language; names and paths may.
	p, err := ParseXML(xmlOf("Tarea de Sebastián", logon, `EQUIPO\sebastián`, "", `C:\Users\sebastián\AppData\Local\Programa Ñandú\ñandú.exe`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Author != "Tarea de Sebastián" || !strings.Contains(p.Command, "ñandú.exe") || p.UserID != `EQUIPO\sebastián` {
		t.Fatalf("parsed = %+v", p)
	}
}

func lookup(n string) string { return NewFake().Getenv(n) }

func TestClassifyTable(t *testing.T) {
	me := NewFake().Me
	mk := func(folder, name, author, trig, user, level, cmd string) (Raw, Parsed, error) {
		r := Raw{Folder: folder, Name: name, State: 3, XML: xmlOf(author, trig, user, level, cmd)}
		p, err := ParseXML(r.XML)
		return r, p, err
	}
	type c struct {
		name                   string
		folder, task, author   string
		trig, user, level, cmd string
		show, caution          bool
		needsAdmin             bool
		reason                 string
	}
	cases := []c{
		{"google updater", `\`, "GoogleUpdateTaskMachineCore", "Google LLC", logon, "", "", `"C:\Program Files (x86)\Google\Update\GoogleUpdate.exe" /c`, true, false, true, ReasonShown},
		{"per-user task", `\`, "MyBackup", `PC\sebas`, logon, `PC\sebas`, "LeastPrivilege", `%LOCALAPPDATA%\Backup\b.exe`, true, false, false, ReasonShown},
		{"per-user by short name", `\`, "MyBackup", "x", logon, `sebas`, "", `C:\b.exe`, true, false, false, ReasonShown},
		{"per-user by SID", `\`, "MyBackup", "x", logon, "S-1-5-21-1-2-3-1001", "", `C:\b.exe`, true, false, false, ReasonShown},
		{"my user but highest privileges needs admin", `\`, "Elev", "x", logon, `PC\sebas`, "HighestAvailable", `C:\b.exe`, true, false, true, ReasonShown},
		{"other user needs admin", `\`, "Other", "x", logon, `PC\someoneelse`, "", `C:\b.exe`, true, false, true, ReasonShown},
		{"same short name, different SID-like", `\`, "S", "x", logon, "S-1-5-18", "", `C:\b.exe`, true, false, true, ReasonShown},
		{"group principal needs admin", `\`, "Grp", "x", logon, "", "", `C:\b.exe`, true, false, true, ReasonShown},
		{"boot task", `\Vendor\`, "Agent", "Vendor", boot, "", "", `C:\Program Files\Vendor\a.exe`, true, false, true, ReasonShown},
		{"localized folder", `\Aplicación Ñandú\`, "Sincronizar", "Ñandú", logon, "", "", `C:\Program Files\Ñandú\s.exe`, true, false, true, ReasonShown},
		{"vendor script via powershell", `\`, "Cleanup", "Acme", logon, "", "", `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, true, false, true, ReasonShown},
		{"security vendor", `\`, "Avast Emergency Update", "AVAST Software", logon, "", "", `C:\Program Files\Avast Software\Avast\up.exe`, true, true, true, ReasonShown},
		{"firewall name", `\`, "Firewall Helper", "Acme", logon, "", "", `C:\x.exe`, true, true, true, ReasonShown},

		{"microsoft folder", `\Microsoft\Windows\Defrag\`, "ScheduledDefrag", "Microsoft", logon, "", "", `C:\Windows\system32\defrag.exe`, false, false, false, ReasonMicrosoft},
		{"microsoft folder upper case", `\MICROSOFT\Windows\X\`, "T", "x", logon, "", "", `C:\x.exe`, false, false, false, ReasonMicrosoft},
		{"microsoft folder without trailing slash", `\Microsoft`, "T", "x", logon, "", "", `C:\x.exe`, false, false, false, ReasonMicrosoft},
		{"microsoft root task by author", `\`, "MicrosoftEdgeUpdateTaskMachineCore", "Microsoft Corporation", logon, "", "", `C:\Program Files (x86)\Microsoft\EdgeUpdate\MicrosoftEdgeUpdate.exe`, false, false, false, ReasonMicrosoft},
		{"onedrive by author", `\`, "OneDrive Standalone Update Task-S-1-5-21", "Microsoft Corporation", logon, "", "", `C:\Users\x\OneDrive.exe`, false, false, false, ReasonMicrosoft},
		{"windows author", `\`, "Foo", "Windows Team", logon, "", "", `C:\x.exe`, false, false, false, ReasonMicrosoft},
		{"resource-string author", `\`, "Foo", `$(@%SystemRoot%\system32\foo.dll,-101)`, logon, "", "", `C:\x.exe`, false, false, false, ReasonMicrosoft},
		{"windows binary", `\`, "Foo", "", logon, "", "", `%SystemRoot%\System32\sc.exe`, false, false, false, ReasonWindows},
		{"windows binary upper case", `\`, "Foo", "", logon, "", "", `C:\WINDOWS\system32\foo.exe`, false, false, false, ReasonWindows},
		{"winforge own task", `\`, "WinForge updates", "WinForge", logon, "", "", `C:\Program Files\WinForge\winforge-cli.exe`, false, false, false, ReasonOwn},
		{"winforge own task, other case and folder", `\WinForge\`, "winforge UPDATES", "WinForge", logon, "", "", `C:\x.exe`, false, false, false, ReasonOwn},
		{"similar to winforge is shown", `\`, "WinForge updates helper", "x", logon, "", "", `C:\x.exe`, true, false, true, ReasonShown},
		{"time trigger only", `\`, "Nightly", "Acme", timeT, "", "", `C:\x.exe`, false, false, false, ReasonNoTrigger},
		{"no triggers", `\`, "Manual", "Acme", ``, "", "", `C:\x.exe`, false, false, false, ReasonNoTrigger},
		{"no action", `\`, "NoAction", "Acme", logon, "", "", ``, false, false, false, ReasonNoAction},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, p, err := mk(tc.folder, tc.task, tc.author, tc.trig, tc.user, tc.level, tc.cmd)
			v := Classify(r, p, err, `C:\Windows`, lookup, me)
			if v.Show != tc.show || v.Reason != tc.reason {
				t.Fatalf("show %v reason %q, want %v %q", v.Show, v.Reason, tc.show, tc.reason)
			}
			if tc.show && (v.Caution != tc.caution || v.NeedsAdmin != tc.needsAdmin) {
				t.Fatalf("caution %v admin %v, want %v %v", v.Caution, v.NeedsAdmin, tc.caution, tc.needsAdmin)
			}
		})
	}
}

func TestClassifyUnreadableDefinitionIsHidden(t *testing.T) {
	r := Raw{Folder: `\`, Name: "Broken", State: 3, XML: "<Task><nope"}
	p, err := ParseXML(r.XML)
	if v := Classify(r, p, err, `C:\Windows`, lookup, nil); v.Show || v.Reason != ReasonUnreadable {
		t.Fatalf("verdict = %+v", v)
	}
	r2 := Raw{Folder: `\`, Name: "NoXML", State: 3}
	p, err = ParseXML(r2.XML)
	if v := Classify(r2, p, err, `C:\Windows`, lookup, nil); v.Show {
		t.Fatal("a task without a readable definition is hidden")
	}
}

func TestStateOf(t *testing.T) {
	cases := map[int]State{0: Unknown, 1: Disabled, 2: Ready, 3: Ready, 4: Running, 9: Unknown, -1: Unknown}
	for n, want := range cases {
		if got := StateOf(n); got != want {
			t.Errorf("StateOf(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestIDHelpers(t *testing.T) {
	cases := []struct{ folder, name, id string }{
		{`\`, "A", `\A`}, {`\Vendor\`, "A", `\Vendor\A`}, {`Vendor`, "A", `\Vendor\A`}, {`\Vendor`, "A", `\Vendor\A`}, {`/Vendor/Sub/`, "A", `\Vendor\Sub\A`},
	}
	for _, c := range cases {
		if got := MakeID(c.folder, c.name); got != c.id {
			t.Errorf("MakeID(%q,%q) = %q, want %q", c.folder, c.name, got, c.id)
		}
	}
	if f, n := SplitID(`\Vendor\Sub\A b`); f != `\Vendor\Sub\` || n != "A b" {
		t.Fatalf("SplitID = %q, %q", f, n)
	}
	if f, n := SplitID(`\A`); f != `\` || n != "A" {
		t.Fatalf("SplitID root = %q, %q", f, n)
	}
}

// --- manager -------------------------------------------------------------------------------

type rig struct {
	fake  *Fake
	m     *Manager
	admin bool
	store *origstate.Store
}

func task(folder, name string, state int, author, triggers, user, level, cmd string) Raw {
	return Raw{Folder: folder, Name: name, State: state, XML: xmlOf(author, triggers, user, level, cmd)}
}

func newRig(t *testing.T) *rig {
	r := &rig{fake: NewFake(), admin: true, store: &origstate.Store{Path: filepath.Join(t.TempDir(), "tasks-original.json")}}
	r.fake.Tasks = []Raw{
		task(`\`, "GoogleUpdateTaskMachineCore", 3, "Google LLC", logon+boot, "", "", `"C:\Program Files (x86)\Google\Update\GoogleUpdate.exe"`),
		task(`\`, "MyBackup", 3, "me", logon, `PC\sebas`, "LeastPrivilege", `C:\Backup\b.exe`),
		task(`\`, "OldSync", 1, "Acme", logon, "", "", `C:\Acme\sync.exe`),
		task(`\`, "Avast Update", 3, "AVAST Software", logon, "", "", `C:\Program Files\Avast\up.exe`),
		task(`\Microsoft\Windows\Defrag\`, "ScheduledDefrag", 3, "Microsoft", boot, "", "", `C:\Windows\system32\defrag.exe`),
		task(`\`, "WinForge updates", 3, "WinForge", logon, "", "", `C:\WinForge\w.exe`),
		task(`\`, "Nightly", 3, "Acme", timeT, "", "", `C:\Acme\n.exe`),
	}
	r.fake.Results[`\MyBackup`] = 0
	r.fake.Results[`\OldSync`] = 267011
	r.fake.Results[`\GoogleUpdateTaskMachineCore`] = -2147024894
	r.m = &Manager{Sys: r.fake, Elevated: func() bool { return r.admin }, Store: r.store}
	return r
}

func TestListShowsOnlyThirdPartyStartupTasks(t *testing.T) {
	r := newRig(t)
	items, err := r.m.List()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	byID := map[string]Item{}
	for _, it := range items {
		names = append(names, it.ID)
		byID[it.ID] = it
	}
	if want := []string{`\Avast Update`, `\GoogleUpdateTaskMachineCore`, `\MyBackup`, `\OldSync`}; !reflect.DeepEqual(names, want) {
		t.Fatalf("ids = %v, want %v", names, want)
	}
	g := byID[`\GoogleUpdateTaskMachineCore`]
	if !reflect.DeepEqual(g.Triggers, []string{"logon", "boot"}) || g.Action != `C:\Program Files (x86)\Google\Update\GoogleUpdate.exe` || g.State != Ready || !g.Enabled || !g.NeedsAdmin || g.Folder != `\` {
		t.Fatalf("google = %+v", g)
	}
	if g.LastResult == nil || *g.LastResult != -2147024894 {
		t.Fatalf("last result = %v", g.LastResult)
	}
	if b := byID[`\MyBackup`]; b.NeedsAdmin || b.LastResult == nil || *b.LastResult != 0 {
		t.Fatalf("per-user task = %+v", b)
	}
	if o := byID[`\OldSync`]; o.State != Disabled || o.Enabled || o.LastResult != nil {
		t.Fatalf("never-run disabled task = %+v (267011 means not run yet)", o)
	}
	if !byID[`\Avast Update`].Caution {
		t.Fatal("security product should be a caution")
	}
}

func TestSetEnabledDisablesAndRemembers(t *testing.T) {
	r := newRig(t)
	if err := r.m.SetEnabled(`\GoogleUpdateTaskMachineCore`, false, false); err != nil {
		t.Fatal(err)
	}
	if r.fake.Enabled(`\GoogleUpdateTaskMachineCore`) {
		t.Fatal("should be disabled")
	}
	if !reflect.DeepEqual(r.fake.Writes, []string{`\GoogleUpdateTaskMachineCore=disabled`}) {
		t.Fatalf("writes = %v", r.fake.Writes)
	}
	if v, ok, _ := r.store.Get(`\googleupdatetaskmachinecore`); !ok || v != "enabled" {
		t.Fatalf("original = %q, %v", v, ok)
	}
	items, _ := r.m.List()
	for _, it := range items {
		if it.ID == `\GoogleUpdateTaskMachineCore` && (!it.Changed || it.Original != "enabled" || it.Enabled) {
			t.Fatalf("item = %+v", it)
		}
	}
	if err := r.m.Restore(`\GoogleUpdateTaskMachineCore`, false); err != nil {
		t.Fatal(err)
	}
	if !r.fake.Enabled(`\GoogleUpdateTaskMachineCore`) {
		t.Fatal("should be enabled again")
	}
	if _, ok, _ := r.store.Get(`\GoogleUpdateTaskMachineCore`); ok {
		t.Fatal("restoring forgets the original")
	}
}

func TestEnablingATaskThatWasDisabledRemembersDisabled(t *testing.T) {
	r := newRig(t)
	if err := r.m.SetEnabled(`\OldSync`, true, false); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := r.store.Get(`\OldSync`); v != "disabled" {
		t.Fatalf("original = %q", v)
	}
	if err := r.m.Restore(`\OldSync`, false); err != nil {
		t.Fatal(err)
	}
	if r.fake.Enabled(`\OldSync`) {
		t.Fatal("restored to disabled")
	}
}

func TestSameStateWritesNothing(t *testing.T) {
	r := newRig(t)
	if err := r.m.SetEnabled(`\GoogleUpdateTaskMachineCore`, true, false); err != nil {
		t.Fatal(err)
	}
	if r.fake.WriteCount() != 0 {
		t.Fatal("no write expected")
	}
	if all, _ := r.store.All(); len(all) != 0 {
		t.Fatal("nothing should be remembered")
	}
}

func TestPerUserTaskNeedsNoAdmin(t *testing.T) {
	r := newRig(t)
	r.admin = false
	if err := r.m.SetEnabled(`\MyBackup`, false, false); err != nil {
		t.Fatalf("a per-user task needs no administrator: %v", err)
	}
	if err := r.m.SetEnabled(`\GoogleUpdateTaskMachineCore`, false, false); !errors.Is(err, ErrNeedsAdmin) {
		t.Fatalf("err = %v", err)
	}
	if r.fake.WriteCount() != 1 {
		t.Fatalf("writes = %v", r.fake.Writes)
	}
}

func TestRefusals(t *testing.T) {
	cases := []struct {
		name    string
		id      string
		enabled bool
		pre     func(*rig)
		want    error
	}{
		{"unknown", `\Nope`, false, nil, ErrNotFound},
		{"microsoft task", `\Microsoft\Windows\Defrag\ScheduledDefrag`, false, nil, ErrProtected},
		{"microsoft task, other case", `\microsoft\windows\defrag\scheduleddefrag`, false, nil, ErrProtected},
		{"winforge's own task", `\WinForge updates`, false, nil, ErrProtected},
		{"task without a startup trigger", `\Nightly`, false, nil, ErrProtected},
		{"needs admin", `\GoogleUpdateTaskMachineCore`, false, func(r *rig) { r.admin = false }, ErrNeedsAdmin},
		{"security product asks first", `\Avast Update`, false, nil, ErrNeedsConfirm},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			if c.pre != nil {
				c.pre(r)
			}
			if err := r.m.SetEnabled(c.id, c.enabled, false); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if r.fake.WriteCount() != 0 {
				t.Fatal("a refused change must write nothing")
			}
			if all, _ := r.store.All(); len(all) != 0 {
				t.Fatalf("a refused change must remember nothing: %v", all)
			}
		})
	}
}

func TestProtectedIsRefusedEvenWhenConfirmed(t *testing.T) {
	r := newRig(t)
	for _, id := range []string{`\Microsoft\Windows\Defrag\ScheduledDefrag`, `\WinForge updates`} {
		if err := r.m.SetEnabled(id, false, true); !errors.Is(err, ErrProtected) {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if r.fake.WriteCount() != 0 {
		t.Fatal("nothing may be written")
	}
}

func TestSecurityTaskConfirmedAndEnablingNeverAsks(t *testing.T) {
	r := newRig(t)
	if err := r.m.SetEnabled(`\Avast Update`, false, true); err != nil {
		t.Fatal(err)
	}
	// Turning it back on needs no confirmation.
	if err := r.m.SetEnabled(`\Avast Update`, true, false); err != nil {
		t.Fatal(err)
	}
}

func TestChangeThatDoesNotStickIsReported(t *testing.T) {
	r := newRig(t)
	r.fake.IgnoreSets = true
	if err := r.m.SetEnabled(`\GoogleUpdateTaskMachineCore`, false, false); !errors.Is(err, ErrNotApplied) {
		t.Fatalf("err = %v", err)
	}
}

func TestFailedWriteForgetsTheOriginalItAdded(t *testing.T) {
	r := newRig(t)
	r.fake.SetErr = errors.New("task:failed: boom")
	if err := r.m.SetEnabled(`\GoogleUpdateTaskMachineCore`, false, false); err == nil {
		t.Fatal("want the write error")
	}
	if all, _ := r.store.All(); len(all) != 0 {
		t.Fatalf("remembered %v", all)
	}
}

func TestNoChangeWithoutAWorkingStore(t *testing.T) {
	r := newRig(t)
	r.m.Store = nil
	if err := r.m.SetEnabled(`\GoogleUpdateTaskMachineCore`, false, false); !errors.Is(err, ErrStore) {
		t.Fatalf("err = %v", err)
	}
	r.m.Store = &origstate.Store{Path: t.TempDir()} // a folder where the file should be
	if err := r.m.SetEnabled(`\GoogleUpdateTaskMachineCore`, false, false); !errors.Is(err, ErrStore) {
		t.Fatalf("err = %v", err)
	}
	if r.fake.WriteCount() != 0 {
		t.Fatal("nothing may be written")
	}
}

func TestRestoreRefusals(t *testing.T) {
	r := newRig(t)
	if err := r.m.Restore(`\OldSync`, false); !errors.Is(err, ErrNoOriginal) {
		t.Fatalf("err = %v", err)
	}
	_, _ = r.store.Remember(`\Microsoft\Windows\Defrag\ScheduledDefrag`, "enabled")
	if err := r.m.Restore(`\Microsoft\Windows\Defrag\ScheduledDefrag`, true); !errors.Is(err, ErrProtected) {
		t.Fatalf("err = %v", err)
	}
	_, _ = r.store.Remember(`\OldSync`, "garbage")
	if err := r.m.Restore(`\OldSync`, true); !errors.Is(err, ErrNoOriginal) {
		t.Fatalf("an invalid remembered value is not an original: %v", err)
	}
}

func TestDamagedOriginalsFileDoesNotBlock(t *testing.T) {
	r := newRig(t)
	_, _ = r.store.Remember("x", "enabled")
	if err := os.WriteFile(r.store.Path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.m.List(); err != nil {
		t.Fatal(err)
	}
	if err := r.m.SetEnabled(`\GoogleUpdateTaskMachineCore`, false, false); err != nil {
		t.Fatal(err)
	}
}

func TestListFailsWhenTheSchedulerCannotBeRead(t *testing.T) {
	m := &Manager{Sys: failingSys{NewFake()}}
	if _, err := m.List(); err == nil {
		t.Fatal("want the read error")
	}
}

type failingSys struct{ *Fake }

func (failingSys) List() ([]Raw, error) { return nil, errors.New("scheduler unreadable") }

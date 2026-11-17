package actions

import (
	"context"
	"encoding/binary"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/envdoctor"
	"github.com/Sebas1705/WinForge/internal/origstate"
	"github.com/Sebas1705/WinForge/internal/privacy"
	"github.com/Sebas1705/WinForge/internal/services"
	"github.com/Sebas1705/WinForge/internal/startup"
	"github.com/Sebas1705/WinForge/internal/storage"
	"github.com/Sebas1705/WinForge/internal/tasks"
)

// ---- storage ----

// fakeStorage records every read and every write; Clean and RemoveProjectJunk never touch a disk.
type fakeStorage struct {
	mu          sync.Mutex
	cats        []storage.Category
	scanErr     error
	reads       []string
	writes      []string
	cleaned     [][]string
	cleanResult storage.Result
	junk        []storage.ProjectJunk
	junkErr     error
	junkRoots   [][]string
	junkAge     []time.Duration
	removed     [][]string
	removeRes   storage.ProjectResult
	onScan      func()
}

func (f *fakeStorage) Scan(ctx context.Context, p storage.ProgressFunc) ([]storage.Category, error) {
	f.mu.Lock()
	f.reads = append(f.reads, "scan")
	hook := f.onScan
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]storage.Category(nil), f.cats...), f.scanErr
}

func (f *fakeStorage) Clean(ctx context.Context, ids []string, p storage.ProgressFunc) storage.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, "clean")
	f.cleaned = append(f.cleaned, append([]string(nil), ids...))
	if p != nil {
		p(storage.Progress{Task: "clean", Files: 3, Bytes: 1024, Current: `C:\Users\` + planted + `\AppData\Local\Temp\x.tmp`})
	}
	return f.cleanResult
}

func (f *fakeStorage) FindProjectJunk(ctx context.Context, roots []string, minAge time.Duration, p storage.ProgressFunc) ([]storage.ProjectJunk, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, "find")
	f.junkRoots = append(f.junkRoots, append([]string(nil), roots...))
	f.junkAge = append(f.junkAge, minAge)
	return append([]storage.ProjectJunk(nil), f.junk...), f.junkErr
}

func (f *fakeStorage) RemoveProjectJunk(ctx context.Context, paths []string, p storage.ProgressFunc) storage.ProjectResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, "remove")
	f.removed = append(f.removed, append([]string(nil), paths...))
	return f.removeRes
}

// ---- privacy registry ----

type fakeReg struct {
	mu     sync.Mutex
	vals   map[string]privacy.Value
	keys   map[string]bool
	dir    string
	writes int
}

func newFakeReg(t *testing.T) *fakeReg {
	return &fakeReg{vals: map[string]privacy.Value{}, keys: map[string]bool{}, dir: t.TempDir()}
}

func rk(p, n string) string { return strings.ToLower(p) + "|" + strings.ToLower(n) }

func (f *fakeReg) addKeys(p string) {
	for p != "" {
		f.keys[strings.ToLower(p)] = true
		i := strings.LastIndex(p, `\`)
		if i < 0 {
			break
		}
		p = p[:i]
	}
}

func (f *fakeReg) putDWord(p, n string, v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	f.addKeys(p)
	f.vals[rk(p, n)] = privacy.Value{Exists: true, Type: privacy.TypeDWord, Data: b}
}

func (f *fakeReg) get(p, n string) privacy.Value {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vals[rk(p, n)]
}

func (f *fakeReg) Get(p, n string) (privacy.Value, error) { return f.get(p, n), nil }
func (f *fakeReg) KeyExists(p string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.keys[strings.ToLower(p)], nil
}
func (f *fakeReg) Set(p, n string, typ uint32, d []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addKeys(p)
	f.vals[rk(p, n)] = privacy.Value{Exists: true, Type: typ, Data: append([]byte(nil), d...)}
	f.writes++
	return nil
}
func (f *fakeReg) Delete(p, n string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.vals, rk(p, n))
	f.writes++
	return nil
}
func (f *fakeReg) DeleteKeyIfEmpty(p string) error { return nil }
func (f *fakeReg) Broadcast() error                { return nil }
func (f *fakeReg) BackupDir() (string, error)      { return f.dir, nil }
func (f *fakeReg) writeCount() int                 { f.mu.Lock(); defer f.mu.Unlock(); return f.writes }

const advKey = `Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo`

// ---- environment ----

type fakeEnvSys struct {
	mu      sync.Mutex
	user    map[string]envdoctor.Raw
	machine map[string]envdoctor.Raw
	dirs    map[string]bool
	dir     string
	writes  []string
}

func (f *fakeEnvSys) Env(s envdoctor.Scope) (map[string]envdoctor.Raw, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	src := f.user
	if s == envdoctor.Machine {
		src = f.machine
	}
	out := map[string]envdoctor.Raw{}
	for k, v := range src {
		out[k] = v
	}
	return out, nil
}
func (f *fakeEnvSys) SetUserVar(n, v string, typ uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.user[n] = envdoctor.Raw{Value: v, Type: typ}
	f.writes = append(f.writes, "set "+n)
	return nil
}
func (f *fakeEnvSys) DeleteUserVar(n string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.user, n)
	f.writes = append(f.writes, "del "+n)
	return nil
}
func (f *fakeEnvSys) Broadcast() error { return nil }
func (f *fakeEnvSys) Stat(p string) envdoctor.PathKind {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dirs[strings.ToLower(strings.TrimRight(p, `\`))] {
		return envdoctor.Dir
	}
	return envdoctor.Missing
}
func (f *fakeEnvSys) Getenv(string) string       { return "" }
func (f *fakeEnvSys) BackupDir() (string, error) { return f.dir, nil }
func (f *fakeEnvSys) writeCount() int            { f.mu.Lock(); defer f.mu.Unlock(); return len(f.writes) }
func (f *fakeEnvSys) value(n string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.user[n].Value
}
func (f *fakeEnvSys) set(n, v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.user[n] = envdoctor.Raw{Value: v, Type: envdoctor.TypeExpand}
}

// ---- startup ----

type fakeStartup struct {
	mu       sync.Mutex
	entries  map[startup.Origin][]startup.Raw
	approved map[startup.Origin]map[string][]byte
	files    map[string]bool
	writes   []string
}

func (f *fakeStartup) Entries(o startup.Origin) ([]startup.Raw, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]startup.Raw(nil), f.entries[o]...), nil
}
func (f *fakeStartup) Approved(o startup.Origin) (map[string][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string][]byte{}
	for k, v := range f.approved[o] {
		out[k] = append([]byte(nil), v...)
	}
	return out, nil
}
func (f *fakeStartup) SetApproved(o startup.Origin, name string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.approved[o] == nil {
		f.approved[o] = map[string][]byte{}
	}
	f.approved[o][strings.ToLower(name)] = append([]byte(nil), data...)
	f.writes = append(f.writes, string(o)+"/"+name)
	return nil
}
func (f *fakeStartup) Exists(p string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[strings.ToLower(p)]
}
func (f *fakeStartup) Getenv(string) string                         { return "" }
func (f *fakeStartup) Inspect([]string) map[string]startup.FileInfo { return nil }
func (f *fakeStartup) writeCount() int                              { f.mu.Lock(); defer f.mu.Unlock(); return len(f.writes) }
func (f *fakeStartup) setCommand(o startup.Origin, name, cmd string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.entries[o] {
		if f.entries[o][i].Name == name {
			f.entries[o][i].Command = cmd
		}
	}
}

// ---- the bundle ----

// bundle is a whole fake PC: every backend an in-memory fake, with real managers and engines on top, so
// the plans are built and run by the same code the GUI uses.
type bundle struct {
	Deps
	storage *fakeStorage
	reg     *fakeReg
	envsys  *fakeEnvSys
	start   *fakeStartup
	svc     *services.Fake
	tsk     *tasks.Fake
	admin   *bool
}

func svcRaw(name, display string, start uint32, image string, deps ...string) services.Raw {
	return services.Raw{Name: name, DisplayName: display, Type: 0x10, Start: start, ImagePath: image, DependsOn: deps}
}

func taskXML(author, triggers, user, command string) string {
	p := `<Principal id="Author"><GroupId>S-1-5-32-545</GroupId></Principal>`
	if user != "" {
		p = `<Principal id="Author"><UserId>` + user + `</UserId><RunLevel>LeastPrivilege</RunLevel></Principal>`
	}
	return `<?xml version="1.0" encoding="UTF-16"?><Task xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><RegistrationInfo><Author>` + author +
		`</Author></RegistrationInfo><Triggers>` + triggers + `</Triggers><Principals>` + p + `</Principals><Actions><Exec><Command>` + command + `</Command></Exec></Actions></Task>`
}

// newDeps builds the fake PC. WinForge starts elevated; tests switch *admin.
func newDeps(t *testing.T) *bundle {
	t.Helper()
	admin := true
	elevated := func() bool { return admin }
	b := &bundle{admin: &admin}

	b.storage = &fakeStorage{}
	b.reg = newFakeReg(t)
	b.envsys = &fakeEnvSys{user: map[string]envdoctor.Raw{}, machine: map[string]envdoctor.Raw{}, dirs: map[string]bool{`c:\keep`: true}, dir: t.TempDir()}
	b.envsys.user["Path"] = envdoctor.Raw{Value: `C:\keep;C:\gone;C:\KEEP\;rel;C:\Users\` + planted + `\gone-bin`, Type: envdoctor.TypeExpand}
	b.envsys.user["JAVA_HOME"] = envdoctor.Raw{Value: `C:\jdk-gone`, Type: envdoctor.TypeString}
	b.envsys.machine["Path"] = envdoctor.Raw{Value: `C:\machine-gone`, Type: envdoctor.TypeString}

	b.start = &fakeStartup{entries: map[startup.Origin][]startup.Raw{}, approved: map[startup.Origin]map[string][]byte{}, files: map[string]bool{}}
	b.start.entries[startup.UserRun] = []startup.Raw{{Name: "Chat", Command: `"C:\Apps\chat.exe" --tray`}, {Name: "Gone", Command: `C:\Gone\gone.exe`}}
	b.start.entries[startup.MachineRun] = []startup.Raw{{Name: "Audio", Command: `C:\Audio\audio.exe`}}
	b.start.files[`c:\apps\chat.exe`] = true
	b.start.files[`c:\audio\audio.exe`] = true

	b.svc = services.NewFake()
	b.svc.Services = []services.Raw{
		svcRaw("Steam Client Service", "Steam Client Service", 3, `"C:\Program Files (x86)\Steam\bin\steamservice.exe"`),
		svcRaw("AVP21", "Kaspersky Service", 2, `"C:\Program Files\Kaspersky Lab\avp.exe"`),
		svcRaw("Parent", "Vendor Parent", 2, `"C:\Program Files\Vendor\parent.exe"`),
		svcRaw("Child", "Vendor Child", 3, `"C:\Program Files\Vendor\child.exe"`, "Parent"),
		svcRaw("Solo", "Vendor Solo", 2, `"C:\Program Files\Vendor\solo.exe"`),
		svcRaw("WinDefend", "Microsoft Defender Antivirus Service", 2, `C:\ProgramData\Microsoft\Windows Defender\MsMpEng.exe`),
		svcRaw("Dhcp", "DHCP Client", 2, `C:\Windows\system32\svchost.exe -k x`),
	}
	b.tsk = tasks.NewFake()
	b.tsk.Tasks = []tasks.Raw{
		{Folder: `\`, Name: "GoogleUpdateTaskMachineCore", State: 3, XML: taskXML("Google LLC", `<LogonTrigger/>`, "", `C:\Program Files\Google\Update\GoogleUpdate.exe`)},
		{Folder: `\`, Name: "MyBackup", State: 3, XML: taskXML("me", `<LogonTrigger/>`, `PC\sebas`, `C:\Backup\b.exe`)},
		{Folder: `\`, Name: "Avast Update", State: 3, XML: taskXML("AVAST Software", `<BootTrigger/>`, "", `C:\Program Files\Avast\up.exe`)},
		{Folder: `\Microsoft\Windows\Defrag\`, Name: "ScheduledDefrag", State: 3, XML: taskXML("Microsoft", `<BootTrigger/>`, "", `C:\Windows\system32\defrag.exe`)},
	}
	dir := t.TempDir()
	store := func(name string) *origstate.Store { return &origstate.Store{Path: filepath.Join(dir, name)} }

	now := func() time.Time { return fixedNow }
	b.Deps = Deps{
		Storage:    b.storage,
		Privacy:    &privacy.Manager{Sys: b.reg, Now: now},
		Env:        &envdoctor.Manager{Sys: b.envsys, Now: now},
		Startup:    &startup.Manager{Sys: b.start, Elevated: elevated, Now: now},
		Services:   &services.Manager{Sys: b.svc, Elevated: elevated, Store: store("services-original.json")},
		Tasks:      &tasks.Manager{Sys: b.tsk, Elevated: elevated, Store: store("tasks-original.json")},
		StorageEnv: storage.Env{Home: `C:\Users\` + planted, Temp: `C:\Users\` + planted + `\AppData\Local\Temp`, SystemDrive: "X:"},
	}
	return b
}

func (b *bundle) env(t *testing.T, ex *fakeExec) *harness {
	t.Helper()
	return newHarness(t, testEnv(t, b.admin, ex), b.Deps)
}

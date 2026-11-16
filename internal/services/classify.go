package services

import (
	"strings"

	"github.com/Sebas1705/WinForge/internal/startup"
)

// Env is what classification needs to know about the machine.
type Env struct {
	WinDir string              // %WINDIR%, "" when unknown
	Lookup func(string) string // environment variables, for %ProgramFiles% and the like
}

func (e Env) lookup(n string) string {
	if e.Lookup == nil {
		return ""
	}
	return e.Lookup(n)
}

// Why a service is hidden. Reasons are for tests and diagnostics, never shown as prose.
const (
	ReasonShown       = ""
	ReasonDriver      = "driver"       // kernel, file system, adapter or recognizer driver, or boot/system start
	ReasonUserService = "user-service" // per-session template service
	ReasonNotService  = "not-service"  // neither its own process nor a shared one
	ReasonProtected   = "protected"    // on the explicit protected list
	ReasonStartMode   = "start-mode"   // a start value WinForge does not manage
	ReasonNoBinary    = "no-binary"    // the program cannot be told, so it cannot be judged
	ReasonWindows     = "windows"      // its program is under %WINDIR% or in a Microsoft/Windows folder
	ReasonMicrosoft   = "microsoft"    // its publisher is Microsoft
	ReasonUnsure      = "unsure"       // %WINDIR% is unknown
)

// Verdict is whether a service may be shown, and whether it looks like a security product.
type Verdict struct {
	Show      bool
	Caution   bool
	Reason    string
	Publisher string // filled by the manager when the publisher is known
}

// protectedNames are services WinForge never shows, even if a vendor ships its own binary for
// them: security, networking, update and core plumbing. Lower-case, exact.
var protectedNames = map[string]bool{
	"windefend": true, "wdnissvc": true, "wdnisdrv": true, "wdfilter": true, "wdboot": true, "sense": true, "sgrmbroker": true,
	"securityhealthservice": true, "wscsvc": true, "mpssvc": true, "bfe": true, "mpsdrv": true,
	"eventlog": true, "eventsystem": true, "wuauserv": true, "usosvc": true, "waasmedicsvc": true, "bits": true,
	"trustedinstaller": true, "cryptsvc": true, "keyiso": true, "samss": true, "lsm": true, "netlogon": true,
	"rpcss": true, "rpceptmapper": true, "dcomlaunch": true, "rpclocator": true,
	"winmgmt": true, "schedule": true, "plugplay": true, "power": true, "profsvc": true, "usermanager": true,
	"gpsvc": true, "appinfo": true, "appidsvc": true, "sppsvc": true, "wlidsvc": true,
	"dhcp": true, "dnscache": true, "nlasvc": true, "lanmanserver": true, "lanmanworkstation": true, "nsi": true,
	"wlansvc": true, "w32time": true, "sens": true, "shellhwdetection": true, "vaultsvc": true,
	"termservice": true, "umrdpservice": true, "remoteregistry": true, "mpssvc2": true,
	"wersvc": true, "wecsvc": true, "diagtrack": true, "dps": true, "wdiservicehost": true, "wdisystemhost": true,
	"audiosrv": true, "audioendpointbuilder": true, "spooler": true, "themes": true, "tiledatamodelsvc": true,
}

// protectedPrefixes catch families of services of the same component.
var protectedPrefixes = []string{
	"windefend", "wdnis", "wdfilter", "securityhealth", "mssecflt", "mpssvc", "wuauserv", "usosvc", "waasmedic",
	"rpcss", "rpcept", "dcomlaunch", "msmpeng", "sgrm", "trustedinstaller",
}

func isProtected(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if protectedNames[n] {
		return true
	}
	for _, p := range protectedPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// ExecutablePath is the program a service's ImagePath starts, without arguments and with
// variables and the \SystemRoot\ and \??\ forms expanded. It is "" when it is not an absolute
// path (so it cannot be judged).
func ExecutablePath(image string, env Env) string {
	s := strings.TrimSpace(image)
	s = strings.TrimPrefix(s, `\??\`)
	low := strings.ToLower(s)
	switch {
	case strings.HasPrefix(low, `\systemroot\`) && env.WinDir != "":
		s = env.WinDir + s[len(`\systemroot`):]
	case strings.HasPrefix(low, `system32\`) && env.WinDir != "":
		s = env.WinDir + `\` + s
	}
	p := startup.ParseCommand(s, env.lookup, nil)
	if p.Launcher || !p.Checkable {
		return ""
	}
	return p.Path
}

func normPath(p string) string {
	p = strings.ToLower(strings.ReplaceAll(p, "/", `\`))
	return strings.TrimRight(p, `\`)
}

// under reports whether path is inside dir (or is dir).
func under(path, dir string) bool {
	d := normPath(dir)
	if d == "" {
		return false
	}
	p := normPath(path)
	return p == d || strings.HasPrefix(p, d+`\`)
}

// inWindowsFolder is true for programs in a folder that belongs to Microsoft or to Windows, by
// the folder names on the way: Microsoft, Microsoft SQL Server, Windows Defender, WindowsApps...
func inWindowsFolder(path string) bool {
	segs := strings.Split(normPath(path), `\`)
	for i, s := range segs {
		if i == len(segs)-1 {
			break // the file name itself does not count
		}
		if s == "microsoft" || strings.HasPrefix(s, "microsoft ") || strings.HasPrefix(s, "windows") {
			return true
		}
	}
	return false
}

// securityWords and securityVendors mark security products: switching them off may reduce
// protection, so the page asks first. Matching is on lower-case text.
var securityWords = []string{
	"antivirus", "anti-virus", "antimalware", "anti-malware", "malware", "firewall", "endpoint protection",
	"endpoint security", "defender", "security", "ransomware", "intrusion",
}

var securityVendors = []string{
	"kaspersky", "avast", "avg", "bitdefender", "norton", "symantec", "mcafee", "eset", "malwarebytes", "sophos",
	"trendmicro", "trend", "fsecure", "webroot", "crowdstrike", "sentinelone", "carbonblack", "cylance", "avira",
	"panda", "comodo", "bullguard", "emsisoft", "zonealarm", "gdata", "vipre", "cybereason",
}

func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
}

// LooksLikeSecurity reports whether the text names a security product.
func LooksLikeSecurity(text string) bool {
	l := strings.ToLower(text)
	for _, w := range securityWords {
		if strings.Contains(l, w) {
			return true
		}
	}
	for _, tok := range tokens(text) {
		for _, v := range securityVendors {
			if tok == v || (len(v) >= 5 && strings.HasPrefix(tok, v)) {
				return true
			}
		}
	}
	return false
}

// Classify decides whether a service is shown. publisher is the company of its program, "" when
// not known (yet). Order matters: the cheap structural checks come first, then the program's
// location, then its publisher.
func Classify(r Raw, env Env, publisher string) Verdict {
	hide := func(reason string) Verdict { return Verdict{Reason: reason} }
	if r.Type&0xF != 0 || r.Start <= 1 {
		return hide(ReasonDriver)
	}
	if r.Type&0xC0 != 0 {
		return hide(ReasonUserService)
	}
	if r.Type&0x30 == 0 {
		return hide(ReasonNotService)
	}
	if isProtected(r.Name) {
		return hide(ReasonProtected)
	}
	if _, ok := ModeOf(r.Start, r.Delayed); !ok {
		return hide(ReasonStartMode)
	}
	if strings.TrimSpace(env.WinDir) == "" {
		return hide(ReasonUnsure)
	}
	exe := ExecutablePath(r.ImagePath, env)
	if exe == "" {
		return hide(ReasonNoBinary)
	}
	if under(exe, env.WinDir) || inWindowsFolder(exe) {
		return hide(ReasonWindows)
	}
	pub := strings.ToLower(publisher)
	if strings.Contains(pub, "microsoft") {
		return hide(ReasonMicrosoft)
	}
	// Without a publisher, a name that claims to be Microsoft's is not trusted to be a vendor's.
	if pub == "" {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.DisplayName)), "microsoft") || strings.HasPrefix(strings.ToLower(r.Name), "microsoft") {
			return hide(ReasonMicrosoft)
		}
	}
	return Verdict{Show: true, Caution: LooksLikeSecurity(r.Name + " " + r.DisplayName + " " + publisher + " " + exe), Reason: ReasonShown}
}

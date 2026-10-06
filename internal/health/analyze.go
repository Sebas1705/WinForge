package health

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Severity of a check's outcome.
type Severity string

const (
	OK      Severity = "ok"
	Info    Severity = "info"
	Warn    Severity = "warn"
	Bad     Severity = "bad"
	Unknown Severity = "unknown"
)

// Group is the section a check belongs to.
type Group string

const (
	GroupFirmware Group = "firmware"
	GroupDrivers  Group = "drivers"
	GroupStorage  Group = "storage"
	GroupSecurity Group = "security"
	GroupWindows  Group = "windows"
)

// Finding is the outcome of one check. Key and Params let the interface word
// it in the user's language (finding.<key>.title / .detail); nothing here is
// free text.
type Finding struct {
	Key      string            `json:"key"`
	Group    Group             `json:"group"`
	Severity Severity          `json:"severity"`
	Params   map[string]string `json:"params,omitempty"`
	Links    []Link            `json:"links,omitempty"`
	// Winget is the id of a catalog app that helps with this finding.
	Winget string `json:"winget,omitempty"`
}

// Keys is every finding key Analyze can emit; a test checks the interface has
// a title and detail for each, in both languages.
var Keys = []string{
	"bios-age", "uefi-mode", "secure-boot", "tpm", "virtualization",
	"device-problems", "drivers-unsigned", "drivers-old", "gpu-driver", "wu-drivers", "wu-software",
	"disk-health", "disk-space",
	"antivirus", "av-signatures", "firewall", "bitlocker",
	"windows-eol", "pending-reboot", "uptime",
}

const day = 24 * time.Hour

// windows10EOL is the end of mainstream support for Windows 10.
var windows10EOL = time.Date(2025, time.October, 14, 0, 0, 0, 0, time.UTC)

// importantClasses are device classes where a stale driver costs performance,
// stability or security, as opposed to the long tail of system devices.
var importantClasses = map[string]bool{
	"DISPLAY": true, "NET": true, "MEDIA": true, "HDC": true, "SCSIADAPTER": true,
	"BLUETOOTH": true, "USB": true, "SYSTEM": true, "FIRMWARE": true, "BIOMETRIC": true,
}

// Analyze turns a scan into findings. updates may be nil (not checked yet).
// Every check yields a finding, passing ones included, so the interface can
// show "17 of 20 checks fine".
func Analyze(r *Report, updates *UpdateScan, now time.Time) []Finding {
	var out []Finding
	add := func(f Finding) { out = append(out, f) }

	// ---- firmware and boot
	mfr, model := boardIdentity(r)
	biosLinks := func() []Link {
		var ls []Link
		if l, ok := SupportLink(mfr); ok {
			ls = append(ls, l)
		}
		if model != "" {
			ls = append(ls, SearchLink(strings.TrimSpace(mfr+" "+model+" BIOS update")))
		}
		return ls
	}
	if d, ok := parseDate(r.BIOS.Date); ok {
		months := int(now.Sub(d) / (30 * day))
		sev := OK
		switch {
		case months >= 24:
			sev = Warn
		case months >= 12:
			sev = Info
		}
		add(Finding{Key: "bios-age", Group: GroupFirmware, Severity: sev, Links: biosLinks(), Params: map[string]string{
			"version": r.BIOS.Version, "date": r.BIOS.Date, "months": fmt.Sprint(months), "board": strings.TrimSpace(mfr + " " + model)}})
	} else {
		add(Finding{Key: "bios-age", Group: GroupFirmware, Severity: Unknown, Links: biosLinks(), Params: map[string]string{"board": strings.TrimSpace(mfr + " " + model)}})
	}
	if r.BIOS.UEFI != nil {
		add(Finding{Key: "uefi-mode", Group: GroupFirmware, Severity: boolSev(*r.BIOS.UEFI, Info)})
	}
	switch {
	case r.Security.SecureBoot != nil:
		add(Finding{Key: "secure-boot", Group: GroupFirmware, Severity: boolSev(*r.Security.SecureBoot, Info)})
	case r.BIOS.UEFI != nil && *r.BIOS.UEFI:
		add(Finding{Key: "secure-boot", Group: GroupFirmware, Severity: Unknown})
	}
	switch t := r.Security.TPM; {
	case t == nil, !r.Admin && !(t.Present && t.Ready):
		// Get-Tpm reports "not present" to standard users, so a negative answer
		// without administrator rights is not an answer.
		add(Finding{Key: "tpm", Group: GroupFirmware, Severity: Unknown})
	default:
		add(Finding{Key: "tpm", Group: GroupFirmware, Severity: boolSev(t.Present && t.Ready, Info)})
	}
	if v := r.CPU.VirtualizationOn; v != nil {
		add(Finding{Key: "virtualization", Group: GroupFirmware, Severity: boolSev(*v || r.CPU.HypervisorPresent, Info)})
	}

	// ---- drivers
	if n := len(r.Problems); n > 0 {
		var names []string
		var links []Link
		for _, p := range r.Problems {
			label := strings.TrimSpace(p.Device)
			if label == "" {
				label = p.HardwareID
				if h := HardwareHint(p.HardwareID); h != "" {
					label = h + " " + p.HardwareID
				}
			}
			names = append(names, label)
		}
		if l, ok := SupportLink(mfr); ok {
			links = append(links, l)
		}
		add(Finding{Key: "device-problems", Group: GroupDrivers, Severity: Bad, Links: links,
			Params: map[string]string{"count": fmt.Sprint(n), "devices": strings.Join(first(names, 4), "; ")}})
	} else {
		add(Finding{Key: "device-problems", Group: GroupDrivers, Severity: OK})
	}

	var unsigned []string
	var old []Driver
	for _, d := range r.Drivers {
		if !IsThirdParty(d) {
			continue
		}
		if !d.Signed {
			unsigned = append(unsigned, d.Device)
		}
		if dd, ok := parseDate(d.Date); ok && importantClasses[strings.ToUpper(d.Class)] && now.Sub(dd) > 3*365*day {
			old = append(old, d)
		}
	}
	add(Finding{Key: "drivers-unsigned", Group: GroupDrivers, Severity: countSev(len(unsigned), Warn),
		Params: map[string]string{"count": fmt.Sprint(len(unsigned)), "devices": strings.Join(first(unsigned, 4), "; ")}})
	sort.Slice(old, func(i, j int) bool { return old[i].Date < old[j].Date })
	var oldNames []string
	for _, d := range old {
		oldNames = append(oldNames, fmt.Sprintf("%s (%s)", d.Device, d.Date))
	}
	add(Finding{Key: "drivers-old", Group: GroupDrivers, Severity: countSev(len(old), Warn),
		Params: map[string]string{"count": fmt.Sprint(len(old)), "devices": strings.Join(first(oldNames, 4), "; ")}})

	for _, g := range r.GPUs {
		if g.Vendor == "Microsoft" || strings.Contains(strings.ToLower(g.Name), "basic") {
			continue
		}
		sev := OK
		months := 0
		if d, ok := parseDate(g.DriverDate); ok {
			months = int(now.Sub(d) / (30 * day))
			if months >= 6 {
				sev = Info
			}
		} else {
			sev = Unknown
		}
		link, app := gpuDownload(g.Vendor)
		f := Finding{Key: "gpu-driver", Group: GroupDrivers, Severity: sev, Winget: app, Params: map[string]string{
			"gpu": g.Name, "version": FriendlyGPUVersion(g), "date": g.DriverDate, "months": fmt.Sprint(months), "vendor": g.Vendor}}
		if link.URL != "" {
			f.Links = []Link{link}
		}
		add(f)
	}

	if updates != nil {
		var drv, sw int
		for _, u := range updates.Updates {
			if u.Kind == "Driver" {
				drv++
			} else {
				sw++
			}
		}
		add(Finding{Key: "wu-drivers", Group: GroupDrivers, Severity: countSev(drv, Warn), Params: map[string]string{"count": fmt.Sprint(drv)},
			Links: []Link{{Kind: "settings", Label: "Windows Update", URL: "ms-settings:windowsupdate-optionalupdates"}}})
		add(Finding{Key: "wu-software", Group: GroupWindows, Severity: countSev(sw, Info), Params: map[string]string{"count": fmt.Sprint(sw)},
			Links: []Link{{Kind: "settings", Label: "Windows Update", URL: "ms-settings:windowsupdate"}}})
	}

	// ---- storage
	var sick []string
	for _, d := range r.Disks {
		if h := strings.ToLower(d.Health); h != "" && h != "healthy" {
			sick = append(sick, fmt.Sprintf("%s (%s)", d.Name, d.Health))
		}
	}
	add(Finding{Key: "disk-health", Group: GroupStorage, Severity: countSev(len(sick), Bad), Params: map[string]string{"disks": strings.Join(sick, "; ")}})
	var low []string
	for _, v := range r.Volumes {
		if v.SizeGB > 0 && (v.FreeGB/v.SizeGB < 0.10 || v.FreeGB < 20) {
			low = append(low, fmt.Sprintf("%s: %.0f GB (%.0f%%)", v.Letter, v.FreeGB, 100*v.FreeGB/v.SizeGB))
		}
	}
	add(Finding{Key: "disk-space", Group: GroupStorage, Severity: countSev(len(low), Warn), Params: map[string]string{"volumes": strings.Join(low, "; ")}})

	// ---- security
	enabled := 0
	var names []string
	for _, a := range r.Security.AntiVirus {
		names = append(names, a.Name)
		if a.Enabled {
			enabled++
		}
	}
	switch {
	case len(r.Security.AntiVirus) == 0 && r.Security.Defender == nil:
		add(Finding{Key: "antivirus", Group: GroupSecurity, Severity: Unknown})
	default:
		sev := OK
		if enabled == 0 && !(r.Security.Defender != nil && r.Security.Defender.Enabled && r.Security.Defender.RealTime) {
			sev = Bad
		}
		add(Finding{Key: "antivirus", Group: GroupSecurity, Severity: sev, Params: map[string]string{"products": strings.Join(names, ", ")},
			Links: []Link{{Kind: "settings", Label: "Windows Security", URL: "windowsdefender:"}}})
	}
	stale := false
	for _, a := range r.Security.AntiVirus {
		if a.Enabled && !a.UpToDate {
			stale = true
		}
	}
	age := 0.0
	if d := r.Security.Defender; d != nil {
		age = d.SignatureAgeDays
		if d.Enabled && d.SignatureAgeDays > 7 {
			stale = true
		}
	}
	if len(r.Security.AntiVirus) > 0 || r.Security.Defender != nil {
		add(Finding{Key: "av-signatures", Group: GroupSecurity, Severity: boolSev(!stale, Warn), Params: map[string]string{"days": fmt.Sprintf("%.0f", age)}})
	}
	var fwOff []string
	for _, f := range r.Security.Firewall {
		if !f.Enabled {
			fwOff = append(fwOff, f.Profile)
		}
	}
	if len(r.Security.Firewall) > 0 {
		add(Finding{Key: "firewall", Group: GroupSecurity, Severity: countSev(len(fwOff), Warn), Params: map[string]string{"profiles": strings.Join(fwOff, ", ")}})
	}
	if b := r.Security.BitLockerOnSystemDrive; b != nil {
		add(Finding{Key: "bitlocker", Group: GroupSecurity, Severity: boolSev(*b, Info)})
	} else {
		add(Finding{Key: "bitlocker", Group: GroupSecurity, Severity: Unknown})
	}

	// ---- windows
	caption := r.OS.Caption
	eol := strings.Contains(caption, "Windows 10") && now.After(windows10EOL)
	add(Finding{Key: "windows-eol", Group: GroupWindows, Severity: boolSev(!eol, Bad), Params: map[string]string{"caption": caption},
		Links: []Link{{Kind: "support", Label: "Microsoft", URL: "https://www.microsoft.com/windows/windows-11"}}})
	add(Finding{Key: "pending-reboot", Group: GroupWindows, Severity: boolSev(!r.OS.PendingReboot, Warn)})
	add(Finding{Key: "uptime", Group: GroupWindows, Severity: boolSev(r.OS.UptimeDays <= 30, Info), Params: map[string]string{"days": fmt.Sprintf("%.0f", r.OS.UptimeDays)}})

	return out
}

// Tally counts checks that passed and the checks that gave a definite answer
// (unknown results are excluded from the denominator).
func Tally(fs []Finding) (ok, total int) {
	for _, f := range fs {
		if f.Severity == Unknown {
			continue
		}
		total++
		if f.Severity == OK {
			ok++
		}
	}
	return ok, total
}

// IsThirdParty reports whether a driver came from a hardware vendor rather
// than being part of Windows. In-box drivers carry the OS's own 10.0.x version
// or "(Standard ...)" as manufacturer, and their dates are meaningless (2006).
func IsThirdParty(d Driver) bool {
	m := strings.ToLower(strings.TrimSpace(d.Manufacturer))
	if strings.HasPrefix(m, "(standard") || m == "microsoft" || strings.HasPrefix(m, "microsoft ") {
		return false
	}
	return !strings.HasPrefix(d.Version, "10.0.")
}

// FriendlyGPUVersion shows NVIDIA's familiar number (560.94) instead of
// Windows' internal 32.0.15.6094.
func FriendlyGPUVersion(g GPU) string {
	if g.Vendor == "NVIDIA" {
		digits := strings.ReplaceAll(g.DriverVersion, ".", "")
		if len(digits) >= 5 {
			tail := digits[len(digits)-5:]
			return strings.TrimLeft(tail[:3], "0") + "." + tail[3:]
		}
	}
	return g.DriverVersion
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", s)
	return t, err == nil
}

func boolSev(good bool, whenBad Severity) Severity {
	if good {
		return OK
	}
	return whenBad
}

func countSev(n int, whenAny Severity) Severity {
	if n == 0 {
		return OK
	}
	return whenAny
}

func first(s []string, n int) []string {
	if len(s) > n {
		return append(append([]string{}, s[:n]...), fmt.Sprintf("+%d", len(s)-n))
	}
	return s
}

// Package winfeatures reports whether the Windows optional features people ask about (WSL, Virtual
// Machine Platform, Hyper-V, Windows Sandbox, .NET Framework 3.5) are on, and what WSL has installed.
// It only reads. Turning a feature on or off goes through the recipes in catalogdata/recipes/features.yml,
// run by the normal install runner.
//
// The logic here is pure; reading the machine goes through System, which a Windows file implements and
// tests replace with an in-memory fake.
package winfeatures

import "errors"

// State of a feature as shown to the person.
const (
	On          = "on"
	Off         = "off"
	Unavailable = "unavailable" // this Windows edition does not have the feature (Home has no Hyper-V or Sandbox)
	Unknown     = "unknown"     // it could not be read
)

// Win32_OptionalFeature.InstallState values.
const (
	installEnabled  = 1
	installDisabled = 2
	installAbsent   = 3
)

// Pending changes reported by Windows for a feature that waits for a restart.
const (
	PendingEnable  = "enable"
	PendingDisable = "disable"
)

// ErrWSLMissing is what System.WSL returns when wsl.exe is not on this PC.
var ErrWSLMissing = errors.New("wsl.exe is not installed")

// System is everything the package needs from the machine. Every method only reads.
type System interface {
	// OptionalFeatures returns the InstallState (1 enabled, 2 disabled, 3 absent) of every optional
	// feature Windows lists, by feature name. It needs no administrator rights.
	OptionalFeatures() (map[string]int, error)
	// PendingFeatures returns, by feature name, "enable" or "disable" for features that wait for a
	// restart. Windows only tells administrators, so callers ask it only when elevated.
	PendingFeatures() (map[string]string, error)
	// WSL runs wsl.exe with the given arguments (only listing ones: it never starts a distribution)
	// and returns its raw output and exit code. ErrWSLMissing when wsl.exe is not there.
	WSL(args ...string) (out []byte, code int, err error)
	// WSLDefaultVersion is the default WSL version the person chose; ok is false when none is set.
	WSLDefaultVersion() (version int, ok bool)
}

// Feature is one Windows optional feature.
type Feature struct {
	ID             string `json:"id"`             // wsl, vmp, hyperv, sandbox, netfx3
	State          string `json:"state"`          // on, off, unavailable or unknown
	RestartPending bool   `json:"restartPending"` // Windows needs a restart to finish a change
	Enable         string `json:"enable"`         // recipe id that turns it on
	Disable        string `json:"disable"`        // recipe id that turns it off
	NeedsAdmin     bool   `json:"needsAdmin"`     // changing it needs administrator rights WinForge does not have now
}

// known lists the features in the order the page shows them.
var known = []struct {
	id      string
	windows string // the name Windows gives the feature
}{
	{"wsl", "Microsoft-Windows-Subsystem-Linux"},
	{"vmp", "VirtualMachinePlatform"},
	{"hyperv", "Microsoft-Hyper-V-All"},
	{"sandbox", "Containers-DisposableClientVM"},
	{"netfx3", "NetFx3"},
}

// WindowsName is the name Windows gives the feature with the given id, "" for an unknown id.
func WindowsName(id string) string {
	for _, k := range known {
		if k.id == id {
			return k.windows
		}
	}
	return ""
}

// List reports every feature. elevated says whether WinForge runs as administrator: it decides
// needsAdmin, and only then does Windows tell which features wait for a restart. A feature whose
// state cannot be read is "unknown"; the list itself never fails.
func List(sys System, elevated bool) []Feature {
	states, err := sys.OptionalFeatures()
	var pending map[string]string
	if elevated {
		if p, perr := sys.PendingFeatures(); perr == nil {
			pending = p
		}
	}
	out := make([]Feature, 0, len(known))
	for _, k := range known {
		f := Feature{
			ID:         k.id,
			State:      stateOf(states, err, k.windows),
			Enable:     "feature-" + k.id + "-on",
			Disable:    "feature-" + k.id + "-off",
			NeedsAdmin: !elevated,
		}
		switch pending[k.windows] {
		case PendingEnable:
			f.State, f.RestartPending = On, true
		case PendingDisable:
			f.State, f.RestartPending = Off, true
		}
		out = append(out, f)
	}
	return out
}

// stateOf maps Win32_OptionalFeature to a state. A list that could not be read or came back empty
// says nothing about any feature, while a feature missing from a real list does not exist on this
// edition of Windows.
func stateOf(states map[string]int, err error, name string) string {
	if err != nil || len(states) == 0 {
		return Unknown
	}
	v, ok := states[name]
	if !ok {
		return Unavailable
	}
	switch v {
	case installEnabled:
		return On
	case installDisabled:
		return Off
	case installAbsent:
		return Unavailable
	}
	return Unknown
}

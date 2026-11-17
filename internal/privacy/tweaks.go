// Package privacy offers a small, curated set of per-user Windows privacy and noise settings.
// Every tweak is plain data: the exact HKCU registry values it writes. Nothing here needs
// administrator rights, and every write is backed up first so it can be undone exactly.
//
// Deliberately out of scope: removing built-in apps, Defender, UAC, Windows Update, the telemetry
// level, any machine-wide (HKLM) policy, activity history, and services.
package privacy

// Registry value types, as Windows numbers them.
const (
	TypeNone    uint32 = 0
	TypeString  uint32 = 1
	TypeExpand  uint32 = 2
	TypeBinary  uint32 = 3
	TypeDWord   uint32 = 4
	TypeMultiSZ uint32 = 7
	TypeQWord   uint32 = 11
)

const (
	areaAds      = "ads"
	areaSuggest  = "suggestions"
	areaStart    = "start"
	areaWeb      = "web"
	areaFeedback = "feedback"
)

// Change is one registry value of the current user (under HKEY_CURRENT_USER). The tweak writes
// Private as a REG_DWORD. Defaults lists the values that are Windows' own setting: a value that is
// absent is the default too, and "Reset to Windows default" removes the value.
type Change struct {
	Path     string // below HKEY_CURRENT_USER, with backslashes
	Name     string
	Private  uint32
	Defaults []uint32
}

// Tweak is one setting a person can switch. The texts live in the interface under NameKey/WhyKey.
type Tweak struct {
	ID      string
	Area    string
	Changes []Change
}

// NameKey and WhyKey are the interface text keys for the tweak's name and for why it is worth it.
func (t Tweak) NameKey() string { return "privacy.t." + t.ID + ".name" }
func (t Tweak) WhyKey() string  { return "privacy.t." + t.ID + ".why" }

const (
	keyCDM      = `Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager`
	keyAdv      = `Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo`
	keyPrivacy  = `Software\Microsoft\Windows\CurrentVersion\Privacy`
	keyExplorer = `Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`
	keyPolicy   = `Software\Policies\Microsoft\Windows\Explorer`
	keySiuf     = `Software\Microsoft\Siuf\Rules`
	keyLang     = `Control Panel\International\User Profile`
)

var on = []uint32{1}

// cdm builds the ContentDeliveryManager values Windows reads to decide what to suggest: 0 turns
// the suggestion off and 1 (or no value) leaves it on.
func cdm(names ...string) []Change {
	out := make([]Change, 0, len(names))
	for _, n := range names {
		out = append(out, Change{Path: keyCDM, Name: n, Private: 0, Defaults: on})
	}
	return out
}

// tweaks is the whole curated list, in the order the page shows it. Each entry was checked one by
// one: the value names are the ones the Settings app itself writes.
var tweaks = []Tweak{
	{ID: "advertising-id", Area: areaAds, Changes: []Change{
		{Path: keyAdv, Name: "Enabled", Private: 0, Defaults: on}}},
	{ID: "tailored-experiences", Area: areaAds, Changes: []Change{
		{Path: keyPrivacy, Name: "TailoredExperiencesWithDiagnosticDataEnabled", Private: 0, Defaults: on}}},
	{ID: "language-list", Area: areaWeb, Changes: []Change{
		{Path: keyLang, Name: "HttpAcceptLanguageOptOut", Private: 1, Defaults: []uint32{0}}}},
	{ID: "start-suggestions", Area: areaSuggest, Changes: cdm("SystemPaneSuggestionsEnabled", "SubscribedContent-338388Enabled")},
	{ID: "settings-suggestions", Area: areaSuggest, Changes: cdm("SubscribedContent-338393Enabled", "SubscribedContent-353694Enabled", "SubscribedContent-353696Enabled")},
	{ID: "tips-welcome", Area: areaSuggest, Changes: cdm("SoftLandingEnabled", "SubscribedContent-338389Enabled", "SubscribedContent-310093Enabled")},
	{ID: "lockscreen-tips", Area: areaSuggest, Changes: cdm("RotatingLockScreenOverlayEnabled", "SubscribedContent-338387Enabled")},
	{ID: "start-most-used", Area: areaStart, Changes: []Change{
		{Path: keyExplorer, Name: "Start_TrackProgs", Private: 0, Defaults: on}}},
	{ID: "start-recent-items", Area: areaStart, Changes: []Change{
		{Path: keyExplorer, Name: "Start_TrackDocs", Private: 0, Defaults: on}}},
	{ID: "start-web-search", Area: areaStart, Changes: []Change{
		{Path: keyPolicy, Name: "DisableSearchBoxSuggestions", Private: 1, Defaults: []uint32{0}}}},
	{ID: "feedback-prompts", Area: areaFeedback, Changes: []Change{
		{Path: keySiuf, Name: "NumberOfSIUFInPeriod", Private: 0}}},
}

// Tweaks returns a copy of the curated list.
func Tweaks() []Tweak {
	out := make([]Tweak, len(tweaks))
	copy(out, tweaks)
	return out
}

func tweakByID(id string) (Tweak, bool) {
	for _, t := range tweaks {
		if t.ID == id {
			return t, true
		}
	}
	return Tweak{}, false
}

// known reports whether path\name is one of the values a tweak owns. A backup can only restore
// those, so a damaged or edited backup file cannot point a restore at anything else.
func known(path, name string) bool {
	for _, t := range tweaks {
		for _, c := range t.Changes {
			if equalFold(c.Path, path) && equalFold(c.Name, name) {
				return true
			}
		}
	}
	return false
}

// Package catalog defines the WinForge data model (apps, profiles, recipes)
// and loads and validates the embedded catalog.
package catalog

// Trust says where an installer ultimately comes from. The catalog only
// accepts sources a person could reasonably have downloaded by hand.
type Trust string

const (
	// TrustVendor: the package manifest points at the vendor's own download.
	TrustVendor Trust = "vendor"
	// TrustStore: distributed through the Microsoft Store.
	TrustStore Trust = "store"
)

// App is one installable application.
type App struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Category    string   `yaml:"category" json:"category"`
	Description string   `yaml:"description" json:"description"`
	Homepage    string   `yaml:"homepage" json:"homepage"`
	License     string   `yaml:"license,omitempty" json:"license,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// Winget is the winget PackageIdentifier. It is the primary source: the
	// winget-pkgs repository pins installer URLs and SHA-256 hashes and is
	// moderated, which is exactly the reliability the catalog wants.
	Winget string `yaml:"winget" json:"winget"`
	// Publisher is the expected publisher of the winget package. The
	// verifier fails when the manifest says otherwise, which catches
	// hijacked or typosquatted identifiers.
	Publisher string `yaml:"publisher" json:"publisher"`
	// Trust defaults to vendor.
	Trust Trust `yaml:"trust,omitempty" json:"trust,omitempty"`
	// Scope is "user", "machine" or empty (winget's default).
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty"`
	// Override replaces the installer's own switches (winget --override),
	// used for installers that need workload selection.
	Override string `yaml:"override,omitempty" json:"override,omitempty"`
	// Admin marks apps whose installer needs elevation.
	Admin bool `yaml:"admin,omitempty" json:"admin,omitempty"`

	Detect   Detect   `yaml:"detect,omitempty" json:"detect,omitempty"`
	Requires []string `yaml:"requires,omitempty" json:"requires,omitempty"`
	// Recipes run after the app is installed (see Recipe).
	Recipes []string `yaml:"recipes,omitempty" json:"recipes,omitempty"`
	Notes   string   `yaml:"notes,omitempty" json:"notes,omitempty"`

	// Tagline is a short plain-language description in each interface language,
	// set at load time for the apps listed in featured.yml.
	Tagline *Tagline `yaml:"-" json:"tagline,omitempty"`

	// OpenSource is derived from License at load time, never written in YAML.
	OpenSource bool `yaml:"-" json:"openSource"`
}

// Detect lists extra ways to notice an app that winget does not know is
// installed (portable installs, tools installed by a version manager, …).
type Detect struct {
	// Commands are executables looked up on PATH.
	Commands []string `yaml:"commands,omitempty" json:"commands,omitempty"`
	// Registry are case-insensitive regular expressions matched against the
	// DisplayName of Uninstall registry entries.
	Registry []string `yaml:"registry,omitempty" json:"registry,omitempty"`
	// Paths are files or folders whose existence means "installed";
	// environment variables such as %ProgramFiles% are expanded.
	Paths []string `yaml:"paths,omitempty" json:"paths,omitempty"`
}

// ProfileApp is one entry of a profile.
type ProfileApp struct {
	ID      string `yaml:"id" json:"id"`
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

// Profile is a named set of apps plus the recipes to run afterwards. A
// profile file is data only: it can reference catalog ids and recipe ids,
// never carry commands, so importing one from someone else cannot run code
// that the catalog has not vetted.
type Profile struct {
	ID          string       `yaml:"id" json:"id"`
	Name        string       `yaml:"name" json:"name"`
	Description string       `yaml:"description,omitempty" json:"description,omitempty"`
	Kind        string       `yaml:"kind,omitempty" json:"kind,omitempty"` // base | dev | custom
	Extends     []string     `yaml:"extends,omitempty" json:"extends,omitempty"`
	Apps        []ProfileApp `yaml:"apps,omitempty" json:"apps,omitempty"`
	Recipes     []string     `yaml:"recipes,omitempty" json:"recipes,omitempty"`
}

// Recipe is a vetted post-install step. Recipes live only in the embedded
// catalog.
type Recipe struct {
	ID          string `yaml:"id" json:"id"`
	Description string `yaml:"description" json:"description"`
	// PowerShell is the script body. It must be idempotent.
	PowerShell string `yaml:"powershell" json:"powershell"`
	// Check, when it exits 0, means the recipe has nothing to do.
	Check string `yaml:"check,omitempty" json:"check,omitempty"`
	Admin bool   `yaml:"admin,omitempty" json:"admin,omitempty"`
	// After lists app ids that must be installed first.
	After []string `yaml:"after,omitempty" json:"after,omitempty"`
}

// DetectOverlay adds registry rules to an app without touching its
// hand-written entry. The files under detect/ are generated by winforge-import
// from the Apps & features names in the winget manifests.
type DetectOverlay struct {
	ID       string   `yaml:"id"`
	Registry []string `yaml:"registry"`
}

// Tagline is one line about an app, per interface language.
type Tagline struct {
	EN string `json:"en"`
	ES string `json:"es"`
}

// FeaturedEntry is a line of featured.yml.
type FeaturedEntry struct {
	ID string `yaml:"id"`
	EN string `yaml:"en"`
	ES string `yaml:"es"`
}

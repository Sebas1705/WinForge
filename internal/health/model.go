// Package health inspects the PC the way a technician would: firmware, drivers,
// storage, security and Windows Update. Collection is read-only and runs one
// PowerShell script; analysis is pure Go so every rule is testable.
//
// No serial numbers, user names or network addresses are collected, so a report
// can be shared when asking for help.
package health

import "time"

// Report is what one scan found.
type Report struct {
	CollectedAt time.Time `json:"collectedAt"`
	// Admin is whether the scan ran elevated; some checks (Secure Boot state,
	// BitLocker) are unknown without it.
	Admin    bool      `json:"admin"`
	Machine  Machine   `json:"machine"`
	OS       OS        `json:"os"`
	CPU      CPU       `json:"cpu"`
	Memory   Memory    `json:"memory"`
	GPUs     []GPU     `json:"gpus"`
	Board    Board     `json:"board"`
	BIOS     BIOS      `json:"bios"`
	Disks    []Disk    `json:"disks"`
	Volumes  []Volume  `json:"volumes"`
	Security Security  `json:"security"`
	Drivers  []Driver  `json:"drivers"`
	Problems []Problem `json:"problems"`
	// Errors lists sections the script could not read.
	Errors []string `json:"errors"`
}

type Machine struct {
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Type         string `json:"type"` // Desktop, Laptop, ...
}

type OS struct {
	Caption       string  `json:"caption"`
	Version       string  `json:"version"`
	Build         string  `json:"build"`
	Arch          string  `json:"arch"`
	UptimeDays    float64 `json:"uptimeDays"`
	PendingReboot bool    `json:"pendingReboot"`
}

type CPU struct {
	Name              string `json:"name"`
	Cores             int    `json:"cores"`
	Threads           int    `json:"threads"`
	VirtualizationOn  *bool  `json:"virtualizationOn"` // firmware flag; nil when unknown
	HypervisorPresent bool   `json:"hypervisorPresent"`
}

type Memory struct {
	TotalGB float64  `json:"totalGB"`
	Modules []Module `json:"modules"`
}

type Module struct {
	CapacityGB    float64 `json:"capacityGB"`
	SpeedMHz      int     `json:"speedMHz"`
	ConfiguredMHz int     `json:"configuredMHz"`
	Manufacturer  string  `json:"manufacturer"`
	Part          string  `json:"part"`
}

type GPU struct {
	Name          string `json:"name"`
	Vendor        string `json:"vendor"` // normalised: NVIDIA, AMD, Intel, Other
	DriverVersion string `json:"driverVersion"`
	DriverDate    string `json:"driverDate"` // yyyy-mm-dd
}

type Board struct {
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
}

type BIOS struct {
	Vendor  string `json:"vendor"`
	Version string `json:"version"`
	Date    string `json:"date"` // yyyy-mm-dd
	UEFI    *bool  `json:"uefi"` // nil when unknown
}

type Disk struct {
	Name        string  `json:"name"`
	Media       string  `json:"media"` // SSD, HDD, Unspecified
	Bus         string  `json:"bus"`
	SizeGB      float64 `json:"sizeGB"`
	Health      string  `json:"health"`
	Operational string  `json:"operational"`
}

type Volume struct {
	Letter     string  `json:"letter"`
	SizeGB     float64 `json:"sizeGB"`
	FreeGB     float64 `json:"freeGB"`
	FileSystem string  `json:"fileSystem"`
}

type Security struct {
	// AntiVirus lists products Windows Security Center knows about.
	AntiVirus              []AV       `json:"antivirus"`
	Defender               *Defender  `json:"defender"`
	Firewall               []Firewall `json:"firewall"`
	SecureBoot             *bool      `json:"secureBoot"` // nil: unknown (needs admin or not UEFI)
	TPM                    *TPM       `json:"tpm"`
	BitLockerOnSystemDrive *bool      `json:"bitlockerSystem"` // nil: unknown (needs admin)
}

type AV struct {
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	UpToDate bool   `json:"upToDate"`
}

type Defender struct {
	Enabled          bool    `json:"enabled"`
	RealTime         bool    `json:"realTime"`
	SignatureAgeDays float64 `json:"signatureAgeDays"`
}

type Firewall struct {
	Profile string `json:"profile"`
	Enabled bool   `json:"enabled"`
}

type TPM struct {
	Present bool `json:"present"`
	Ready   bool `json:"ready"`
}

type Driver struct {
	Device       string `json:"device"`
	Class        string `json:"class"`
	Manufacturer string `json:"manufacturer"`
	Version      string `json:"version"`
	Date         string `json:"date"` // yyyy-mm-dd
	Signed       bool   `json:"signed"`
	Inf          string `json:"inf"`
}

type Problem struct {
	Device     string `json:"device"`
	Class      string `json:"class"`
	HardwareID string `json:"hardwareId"` // e.g. ACPI\RTK5452\1; names are often empty for unknown devices
	Code       int    `json:"code"`
}

// Update is one pending Windows Update item.
type Update struct {
	Title    string  `json:"title"`
	Kind     string  `json:"kind"` // Driver, Software
	Category string  `json:"category"`
	SizeMB   float64 `json:"sizeMB"`
	Reboot   bool    `json:"reboot"`
}

// UpdateScan is the result of searching Windows Update.
type UpdateScan struct {
	CheckedAt time.Time `json:"checkedAt"`
	Updates   []Update  `json:"updates"`
}

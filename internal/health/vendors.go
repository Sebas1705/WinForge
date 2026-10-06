package health

import (
	"net/url"
	"sort"
	"strings"
)

// Link is an action offered next to a finding. Kind lets the UI label it in the
// user's language; Label is a proper name (a vendor) and is never translated.
type Link struct {
	Kind  string `json:"kind"` // support, download, search, settings
	Label string `json:"label"`
	URL   string `json:"url"`
}

type vendor struct {
	match   []string
	name    string
	support string
}

// Support pages are the vendors' own landing pages: stable, and they lead to
// the model search. Deep links to a model's BIOS page change too often to
// hard-code, so the model is offered as a search instead.
var vendors = []vendor{
	{[]string{"asustek", "asus"}, "ASUS", "https://www.asus.com/support/"},
	{[]string{"micro-star", "msi"}, "MSI", "https://www.msi.com/support"},
	{[]string{"gigabyte", "aorus"}, "Gigabyte", "https://www.gigabyte.com/Support"},
	{[]string{"asrock"}, "ASRock", "https://www.asrock.com/support/index.asp"},
	{[]string{"dell", "alienware"}, "Dell", "https://www.dell.com/support/home"},
	{[]string{"hewlett", "hp inc", "omen"}, "HP", "https://support.hp.com/"},
	{[]string{"lenovo"}, "Lenovo", "https://support.lenovo.com/"},
	{[]string{"acer", "predator"}, "Acer", "https://www.acer.com/support"},
	{[]string{"samsung"}, "Samsung", "https://www.samsung.com/support/"},
	{[]string{"razer"}, "Razer", "https://support.razer.com/"},
	{[]string{"framework"}, "Framework", "https://knowledgebase.frame.work/"},
	{[]string{"microsoft"}, "Microsoft Surface", "https://support.microsoft.com/surface"},
	{[]string{"intel"}, "Intel", "https://www.intel.com/content/www/us/en/support.html"},
}

// placeholders are what OEM-less boards report instead of a real name.
var placeholders = []string{"system manufacturer", "system product name", "to be filled", "default string", "o.e.m", "not applicable", "unknown", "none"}

func isPlaceholder(s string) bool {
	l := strings.ToLower(strings.TrimSpace(s))
	if l == "" {
		return true
	}
	for _, p := range placeholders {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

// SupportLink returns the vendor's support landing page for a manufacturer
// string such as "ASUSTeK COMPUTER INC.".
func SupportLink(manufacturer string) (Link, bool) {
	l := strings.ToLower(manufacturer)
	for _, v := range vendors {
		for _, m := range v.match {
			if strings.Contains(l, m) {
				return Link{Kind: "support", Label: v.name, URL: v.support}, true
			}
		}
	}
	return Link{}, false
}

// SearchLink is a web search for a model's firmware or drivers. Opening it is
// the user's choice; nothing is sent until they click.
func SearchLink(query string) Link {
	return Link{Kind: "search", Label: "DuckDuckGo", URL: "https://duckduckgo.com/?q=" + url.QueryEscape(query)}
}

// boardIdentity picks the manufacturer and model that name the firmware's
// owner: the system's when it is a real OEM (laptops, prebuilts), otherwise the
// motherboard's.
func boardIdentity(r *Report) (manufacturer, model string) {
	if !isPlaceholder(r.Machine.Manufacturer) && !isPlaceholder(r.Machine.Model) {
		return r.Machine.Manufacturer, r.Machine.Model
	}
	m := r.Board.Manufacturer
	if isPlaceholder(m) {
		m = r.Machine.Manufacturer
	}
	return m, r.Board.Product
}

// gpuDownload is where each GPU vendor publishes drivers.
func gpuDownload(vendor string) (Link, string) {
	switch vendor {
	case "NVIDIA":
		return Link{Kind: "download", Label: "NVIDIA", URL: "https://www.nvidia.com/Download/index.aspx"}, ""
	case "AMD":
		return Link{Kind: "download", Label: "AMD", URL: "https://www.amd.com/en/support/download/drivers.html"}, ""
	case "Intel":
		return Link{Kind: "download", Label: "Intel", URL: "https://www.intel.com/content/www/us/en/download-center/home.html"}, "Intel.IntelDriverAndSupportAssistant"
	}
	return Link{}, ""
}

// driverPages are the vendors' own driver download pages, for the chip makers
// whose drivers a PC most often lacks.
var driverPages = map[string]Link{
	"AMD":     {Kind: "download", Label: "AMD", URL: "https://www.amd.com/en/support/download/drivers.html"},
	"Intel":   {Kind: "download", Label: "Intel", URL: "https://www.intel.com/content/www/us/en/download-center/home.html"},
	"NVIDIA":  {Kind: "download", Label: "NVIDIA", URL: "https://www.nvidia.com/Download/index.aspx"},
	"Realtek": {Kind: "download", Label: "Realtek", URL: "https://www.realtek.com/Download"},
}

func appendUnique(ls []Link, more ...Link) []Link {
next:
	for _, m := range more {
		for _, l := range ls {
			if l.URL == m.URL {
				continue next
			}
		}
		ls = append(ls, m)
	}
	return ls
}

// vendorDriverLinks collects the download pages of the makers behind the
// drivers that match, so an outdated or unsigned driver is one click from its
// replacement.
func vendorDriverLinks(drivers []Driver, match func(Driver) bool) []Link {
	var ls []Link
	for _, d := range drivers {
		if !match(d) {
			continue
		}
		m := strings.ToLower(d.Manufacturer)
		for name, l := range driverPages {
			if strings.Contains(m, strings.ToLower(name)) || (name == "AMD" && strings.Contains(m, "advanced micro")) {
				ls = appendUnique(ls, l)
			}
		}
	}
	sort.Slice(ls, func(i, j int) bool { return ls[i].Label < ls[j].Label })
	return ls
}

// CatalogLink searches the Microsoft Update Catalog, where Windows drivers are
// published as plain downloads, for a hardware id such as ACPI\RTK5452\1 or
// PCI\VEN_10EC&DEV_8168&SUBSYS_...; only the part that names the chip is used.
func CatalogLink(label, hardwareID string) Link {
	return Link{Kind: "catalog", Label: label, URL: "https://www.catalog.update.microsoft.com/Search.aspx?q=" + url.QueryEscape(chipID(hardwareID))}
}

func chipID(id string) string {
	parts := strings.Split(id, "\\")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	s := strings.Join(parts, "\\")
	if i := strings.Index(strings.ToUpper(s), "&SUBSYS"); i > 0 {
		s = s[:i]
	}
	return s
}

// DriverLinks are the download places for a device that has no working driver:
// the chip maker's driver page when the hardware id names one, then the
// Microsoft Update Catalog entry for that exact id.
func DriverLinks(device, hardwareID string) []Link {
	var ls []Link
	if l, ok := driverPages[HardwareHint(hardwareID)]; ok {
		ls = append(ls, l)
	}
	label := strings.TrimSpace(device)
	if label == "" {
		label = chipID(hardwareID)
	}
	if hardwareID != "" {
		ls = append(ls, CatalogLink(label, hardwareID))
	}
	return ls
}

// HardwareHint names the vendor behind an ACPI or PCI hardware id, so "unknown
// device" can say whose driver is missing.
func HardwareHint(id string) string {
	u := strings.ToUpper(id)
	switch {
	case strings.HasPrefix(u, "ACPI\\AMD"), strings.Contains(u, "VEN_1022"), strings.Contains(u, "VEN_1002"):
		return "AMD"
	case strings.HasPrefix(u, "ACPI\\RTK"), strings.Contains(u, "VEN_10EC"):
		return "Realtek"
	case strings.HasPrefix(u, "ACPI\\INT"), strings.Contains(u, "VEN_8086"):
		return "Intel"
	case strings.HasPrefix(u, "ACPI\\NVDA"), strings.Contains(u, "VEN_10DE"):
		return "NVIDIA"
	case strings.Contains(u, "VEN_14C3"):
		return "MediaTek"
	case strings.Contains(u, "VEN_168C"), strings.Contains(u, "VEN_17CB"):
		return "Qualcomm"
	}
	return ""
}

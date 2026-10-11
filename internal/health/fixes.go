package health

// fixes maps a finding to the repair recipes (catalogdata/recipes/repairs.yml)
// that can fix it by themselves. A finding that is not here can only be fixed by
// hand (BIOS updates, a failing disk, a driver Windows Update does not have):
// it keeps its vendor links instead. Repairs are never started without the
// person pressing the button.
var fixes = map[string][]string{
	"device-problems": {"repair-missing-drivers"},
	"wu-drivers":      {"repair-driver-updates"},
	"wu-software":     {"repair-windows-updates"},
	"av-signatures":   {"repair-defender-signatures"},
	"antivirus":       {"repair-defender-realtime"},
	"firewall":        {"repair-firewall"},
}

// FixesFor returns the repair ids for a finding key (a copy), or nil.
func FixesFor(key string) []string {
	if f, ok := fixes[key]; ok {
		return append([]string(nil), f...)
	}
	return nil
}

// RepairIDs lists every repair a finding can point to.
func RepairIDs() []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range fixes {
		for _, id := range list {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

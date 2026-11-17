// Package mcpserver is the server side of WinForge's MCP support: JSON-RPC 2.0 over stdio, the safety modes,
// the plan store, redaction and the registry of tools. The tools themselves live in internal/mcptools and
// speak only the contract of internal/mcpkit.
package mcpserver

import (
	"fmt"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// Mode is how much the assistant may do. The default is Read.
type Mode int

const (
	// ModeRead lists and runs only Read tools.
	ModeRead Mode = iota
	// ModePlan adds Prepare tools: the assistant can build plans but nothing can be applied.
	ModePlan
	// ModeFull adds Act tools; they only run plans prepared in this process.
	ModeFull
)

// String is the value of the --mode flag.
func (m Mode) String() string {
	switch m {
	case ModeRead:
		return "read"
	case ModePlan:
		return "plan"
	case ModeFull:
		return "full"
	}
	return "unknown"
}

// ParseMode reads the --mode flag.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "read":
		return ModeRead, nil
	case "plan":
		return ModePlan, nil
	case "full":
		return ModeFull, nil
	}
	return ModeRead, fmt.Errorf("unknown mode %q: use read, plan or full", s)
}

// Allows reports whether tools of this class may be listed and called in this mode.
func (m Mode) Allows(c mcpkit.Class) bool {
	switch c {
	case mcpkit.Read:
		return true
	case mcpkit.Prepare:
		return m >= ModePlan
	case mcpkit.Act:
		return m >= ModeFull
	}
	return false
}

// modeFor is the weakest mode that allows a class.
func modeFor(c mcpkit.Class) Mode {
	switch c {
	case mcpkit.Prepare:
		return ModePlan
	case mcpkit.Act:
		return ModeFull
	}
	return ModeRead
}

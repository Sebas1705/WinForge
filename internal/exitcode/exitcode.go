// Package exitcode is the one place that defines the exit codes of winforge-cli. Every subcommand
// (scan, profiles, plan, apply, script, configure, health, export, check, updates) uses the same
// table, so a script can tell "finished with failures" from "another run is busy" from "winget is
// missing" without knowing which command it started.
package exitcode

import (
	"fmt"
	"strings"
)

// The exit codes. They are part of the command-line contract: never renumber one.
const (
	OK       = 0 // finished without problems
	Failures = 1 // finished, but some work failed
	Usage    = 2 // the command line was not understood; nothing was done
	Busy     = 3 // another run is already in progress; nothing was done
	Prereq   = 4 // a prerequisite is missing (winget) or could not be used
	Data     = 5 // a data file (profile, settings, catalog) is damaged or unreadable
	Timeout  = 6 // the run hit its time limit (--timeout)
	Admin    = 7 // administrator rights are needed and this run has none; nothing was done
	Reboot   = 8 // finished fine, but Windows has to restart to complete it
)

// Entry describes one code.
type Entry struct {
	Code int
	// Name is the stable machine word that appears as error.code in --json output.
	Name string
	// Meaning is the sentence shown in every --help.
	Meaning string
}

// Table lists every code in order.
var Table = []Entry{
	{OK, "ok", "finished without problems"},
	{Failures, "failures", "finished, but some work failed"},
	{Usage, "usage", "the command line was not understood"},
	{Busy, "busy", "another run is already in progress; nothing was done"},
	{Prereq, "prerequisite", "a prerequisite is missing, such as winget"},
	{Data, "data", "a data file is damaged or unreadable"},
	{Timeout, "timeout", "the run hit its time limit"},
	{Admin, "admin", "administrator rights are needed; nothing was done"},
	{Reboot, "reboot", "finished fine, but Windows has to restart to complete it"},
}

// Name returns the machine word of a code ("" for an unknown one).
func Name(code int) string {
	for _, e := range Table {
		if e.Code == code {
			return e.Name
		}
	}
	return ""
}

// Succeeded is true for the two codes that mean the work was done: OK and Reboot.
func Succeeded(code int) bool { return code == OK || code == Reboot }

// HelpBlock is the "Exit codes" section every --help ends with.
func HelpBlock() string {
	var b strings.Builder
	b.WriteString("Exit codes (the same for every command):\n")
	for _, e := range Table {
		fmt.Fprintf(&b, "  %d  %s\n", e.Code, e.Meaning)
	}
	return b.String()
}

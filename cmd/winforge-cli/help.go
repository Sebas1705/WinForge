package main

import (
	"github.com/Sebas1705/WinForge/internal/cliout"
	"github.com/Sebas1705/WinForge/internal/exitcode"
	"github.com/Sebas1705/WinForge/internal/sched"
)

// commands lists every subcommand, in the order of the usage text. The help test walks it.
var commands = []string{"scan", "profiles", "plan", "apply", "script", "configure", "health", "updates", "export", "check"}

type helpEntry struct {
	usage string // after "usage: winforge-cli "
	text  string // what it does, what --json adds, command-specific exit code notes
}

var helpTexts = map[string]helpEntry{
	"scan": {"scan [--json] [--quiet] [--timeout <time>]", `Lists the catalog apps installed on this PC, with their version and how they were found.

With --json: installed[] (id, name, version, sources), installedCount, catalogCount, warnings[].
Exit 4 also means winget is missing: the list is still printed, but it comes from the registry
and PATH only.
`},
	"profiles": {"profiles [--json]", `Lists the profiles of the catalog.

With --json: profiles[] (id, kind, name).
`},
	"plan": {"plan <profile> [--json] [--quiet] [--timeout <time>]", `Shows what applying a profile would do on this PC. Nothing is changed.

With --json: profile, alreadyInstalled[], steps[] (kind, id, name, version, admin), skippedRecipes[],
needsAdmin.
`},
	"apply": {"apply <profile> [--json] [--quiet] [--yes] [--timeout <time>]", `Installs what is missing from a profile, then runs its recipes. A failed app does not stop the
others; steps that depend on it are skipped.

Only one run happens at a time (it shares its lock with "updates"). With --yes (or
--non-interactive), a plan that needs administrator rights is refused up front when this process
is not elevated (exit 7): an unattended run never raises a permission prompt. Without winget the
run is refused (exit 4). WinForge never restarts the PC: a step that asks for one is only reported
(exit 8 when everything else worked).

With --json: profile, alreadyInstalled[], skippedRecipes[], needsAdmin, rebootRequired, counts
{ok, failed, skipped} and steps[] (id, kind, name, result ok|failed|skipped, reason, message,
from, to, reboot, admin). from and to are the version installed before and after.
`},
	"script": {"script <profile> [--json] [--quiet] [--timeout <time>]", `Prints a PowerShell script that installs what is missing from a profile. Nothing is run.

With --json: profile, script.
`},
	"configure": {"configure <profile> [--json]", `Prints the profile as a winget configure file (apps only).

With --json: profile, format ("winget-configure"), content.
`},
	"health": {"health [updates] [--json] [--quiet] [--timeout <time>]", `Checks firmware, drivers, storage and security. Read-only. "updates" also looks for
Windows and driver updates. Findings are keys and parameters, not sentences.

With --json: machine, findings[] (key, group, severity, params, winget), okCount, total.
`},
	"updates": {"updates [--mode notify|apply] [--scheduled] [--json] [--quiet] [--timeout <time>]", ""},
	"export": {"export <id> [pin] [--json] [--quiet] [--timeout <time>]", `Writes this PC's catalog apps as a profile with the given id, to stdout. "pin" records the
installed versions.

With --json: profile (the profile document itself).
`},
	"check": {"check <file.json> [--json]", `Validates a profile file against the catalog. Unknown apps and recipes are reported, not fatal.

With --json: file, name, apps, unknownApps[], unknownRecipes[].
Exit 5 when the file cannot be read or is not a valid profile.
`},
}

// helpFor is the --help text of one command.
func helpFor(cmd string) string {
	if cmd == "updates" {
		return sched.Help
	}
	h := helpTexts[cmd]
	return "usage: winforge-cli " + h.usage + "\n\n" + h.text + "\n" + cliout.FlagHelp + "\n" + exitcode.HelpBlock()
}

// usageFor is the one-line usage printed after a usage mistake.
func usageFor(cmd string) string { return "usage: winforge-cli " + helpTexts[cmd].usage }

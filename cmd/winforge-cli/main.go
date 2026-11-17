// Command winforge-cli is the headless face of WinForge: scan this PC, plan or
// apply a profile, export what is installed. The desktop app uses the same
// packages.
//
// It is built for unattended use: --json turns stdout into one JSON document, every command ends
// with one of the shared exit codes (internal/exitcode) and nothing ever waits for input.
package main

import (
	"context"
	"os"
	"os/signal"
)

const usage = `usage:
  winforge-cli scan                     list catalog apps installed on this PC
  winforge-cli profiles                 list catalog profiles
  winforge-cli plan <profile>           show what applying a profile would do
  winforge-cli apply <profile>          install what is missing, then run recipes
  winforge-cli script <profile>         print a PowerShell script for what is missing
  winforge-cli configure <profile>      print the profile as a winget configure file (apps only)
  winforge-cli health [updates]         check firmware, drivers, storage and security (read-only)
  winforge-cli updates [--mode notify|apply] [--scheduled]
                                        check for updates, or update what may be updated unattended
                                        (see: winforge-cli updates --help)
  winforge-cli export <id> [pin]        write this PC's catalog apps as a profile (stdout)
  winforge-cli check <file.json>        validate a profile file against the catalog

global options: --json --quiet --no-color --yes --timeout <time>
  winforge-cli <command> --help         describes a command, its --json document and its exit codes`

func main() {
	// run answers `updates` and every --help before the catalog loads.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, realDeps())
	stop()
	os.Exit(code)
}

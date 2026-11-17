package install

import (
	"context"
	"errors"
	"fmt"
)

// StepKit installs an app from an offline kit: a local installer that was
// verified against the kit's hashes, run with the silent switches its winget
// manifest declares. See package offlinekit.
const StepKit StepKind = "kit"

// Reasons only a kit step can fail with, as keys the UI translates. A full
// disk reuses ReasonDisk.
const (
	// ReasonKitHash means a file of the kit is not what kit.json recorded: it was modified, replaced or is a link.
	ReasonKitHash = "kithash"
	// ReasonKitClosed means no kit is open, so there is nothing to run.
	ReasonKitClosed = "kitclosed"
	// ReasonKitFile means the kit's files cannot be read any more (the USB stick was removed, say).
	ReasonKitFile = "kitfile"
	// ReasonKitUnsupported means the installer's type cannot be run safely offline.
	ReasonKitUnsupported = "kitunsupported"
)

// KitCommand is how to run one app's local installer. The program and every
// argument are separate strings: nothing is ever joined into a command line.
type KitCommand struct {
	Exe  string
	Args []string
	// OKCodes are exit codes, besides 0 and the restart codes, that the manifest calls success.
	OKCodes []int
}

// KitResolver turns a kit step into a command. Prepare makes sure the files are
// exactly what the kit says (it copies them to a private folder and checks the
// copy), Done removes that copy. Only a resolver that was handed a verified kit
// can answer, which is what keeps a step from naming a path of its own.
type KitResolver interface {
	Prepare(ctx context.Context, appID string, emit func(line string)) (KitCommand, error)
	Done(appID string)
}

// KitError is a kit step that failed for a reason the UI can explain.
type KitError struct {
	Reason string
	Err    error
}

func (e *KitError) Error() string { return e.Err.Error() }
func (e *KitError) Unwrap() error { return e.Err }

// HasKitSteps reports whether a plan installs from a kit. Such a plan is not
// remembered as an unfinished run: its files may be on a stick that is gone.
func HasKitSteps(p Plan) bool {
	for _, s := range p.Steps {
		if s.Kind == StepKit {
			return true
		}
	}
	return false
}

// exitRestartStarted is ERROR_SUCCESS_REBOOT_INITIATED: done, and a restart was started.
const exitRestartStarted = 1641

func runKit(ctx context.Context, kit KitResolver, s Step, ex Executor, emit func(Event)) (int, error) {
	if kit == nil {
		return -1, &KitError{Reason: ReasonKitClosed, Err: errors.New("no offline kit is open")}
	}
	onLine := func(l string) { emit(Event{Step: s, Status: StatusOutput, Line: l}) }
	cmd, err := kit.Prepare(ctx, s.ID, onLine)
	defer kit.Done(s.ID)
	if err != nil {
		return -1, err
	}
	code, err := ex.Run(ctx, cmd.Exe, cmd.Args, onLine)
	if err != nil {
		return code, &KitError{Reason: ReasonKitFile, Err: fmt.Errorf("could not run the installer: %w", err)}
	}
	switch code {
	case 0, exitRebootRequired:
		return code, nil
	case exitRestartStarted:
		return exitRebootRequired, nil
	}
	for _, ok := range cmd.OKCodes {
		if ok == code {
			return 0, nil
		}
	}
	return code, &KitError{Reason: kitExitReason(code), Err: fmt.Errorf("the installer exited with code %d", code)}
}

// kitExitReason explains an installer's own exit code (Windows Installer or
// the installer's convention) as a Reason.
func kitExitReason(code int) string {
	switch code {
	case 740, 5, 1925: // elevation required, access denied, cannot run as this user
		return ReasonAdmin
	case 1618:
		return ReasonBusy
	case 1602, 1223:
		return ReasonCancelled
	case 112:
		return ReasonDisk
	case 1603:
		// A generic MSI failure; without administrator rights it is the usual cause.
		return ReasonAdmin
	case 1614: // the product is uninstalled; restart pending
		return ReasonReboot
	}
	return ReasonInstaller
}

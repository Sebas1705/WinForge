package history

import (
	"errors"
	"regexp"

	"github.com/Sebas1705/WinForge/internal/catalog"
	"github.com/Sebas1705/WinForge/internal/install"
)

// Errors a rollback can be refused with. Their text is a stable code the interface translates.
var (
	ErrUnknownEntry    = errors.New("history.unknown")
	ErrNotRollbackable = errors.New("history.notRollbackable")
	ErrMoved           = errors.New("history.moved")
)

// versionShape is what a winget version looks like. It is checked even though the version came
// from the history file: the file is on disk and could have been edited, and a value that starts
// with "-" would be read by winget as an option.
var versionShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_~-]{0,63}$`)

// RollbackPlan builds the plan that puts the app of an update entry back on the version it had.
// It is one pinned-version step (install.StepRollback); the entry must be an update that
// succeeded with a known previous version, and its app must be in the catalog.
func RollbackPlan(cat *catalog.Catalog, e Entry) (install.Plan, error) {
	app := cat.Apps[e.App]
	if app == nil || e.Action != Update || e.Result == Failed || !versionShape.MatchString(e.From) {
		return install.Plan{}, ErrNotRollbackable
	}
	return install.Plan{
		Steps:            []install.Step{{Kind: install.StepRollback, ID: e.App, Name: app.Name, Version: e.From, Admin: app.Admin}},
		AlreadyInstalled: []string{},
		NeedsAdmin:       app.Admin,
	}, nil
}

// Find returns the entry with the given id. Only ids that are in the history are accepted: the
// interface cannot name an app or a version of its own.
func Find(entries []Entry, id string) (Entry, error) {
	for _, e := range entries {
		if e.ID == id && id != "" {
			return e, nil
		}
	}
	return Entry{}, ErrUnknownEntry
}

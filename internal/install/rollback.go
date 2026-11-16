package install

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

// Reasons only a rollback can fail with.
const (
	// ReasonDowngrade means the installer or winget refused to put an older version over a newer one.
	ReasonDowngrade = "downgrade"
	// ReasonVersion means winget has no such version of the package.
	ReasonVersion = "version"
)

// msiNewerVersion is ERROR_PRODUCT_VERSION (1638): another version of the product is already installed.
const msiNewerVersion = 1638

// WingetRollbackArgs builds the command line that installs exactly one older version over a newer
// one. It is the install command pinned to a version, plus --force: without it winget answers "a
// newer version is already installed" and does nothing.
func WingetRollbackArgs(app *catalog.App, version string) []string {
	return append(WingetArgs(app, version), "--force")
}

// runRollback runs a rollback step. Unlike an install, "already installed" and "no applicable
// update" are failures here: they mean nothing was changed, and a rollback that changed nothing
// must not look like it worked.
func runRollback(ctx context.Context, app *catalog.App, s Step, ex Executor, emit func(Event)) (int, error) {
	if strings.TrimSpace(s.Version) == "" {
		return -1, errors.New("a rollback needs the version to go back to")
	}
	onLine := func(l string) { emit(Event{Step: s, Status: StatusOutput, Line: l}) }
	code, err := ex.Run(ctx, "winget", WingetRollbackArgs(app, s.Version), onLine)
	if err != nil {
		return code, fmt.Errorf("could not run winget: %w", err)
	}
	if code != 0 && code != exitRebootRequired {
		return code, fmt.Errorf("winget exited with code %#x", uint32(code))
	}
	return code, nil
}

// explainStep is Explain with what only a rollback can run into.
func explainStep(s Step, code int, lines []string) string {
	if s.Kind != StepRollback {
		return Explain(code, lines)
	}
	text := strings.ToLower(strings.Join(lines, "\n"))
	switch {
	case uint32(code) == wingetAlreadyInstalled || uint32(code) == wingetNoUpgrade:
		return ReasonDowngrade
	case strings.Contains(text, "no version found") || (strings.Contains(text, "no se encontr") && strings.Contains(text, "versi")):
		return ReasonVersion
	case strings.Contains(text, "downgrade") || strings.Contains(text, "newer version") || strings.Contains(text, "versión más reciente") || strings.Contains(text, "version mas reciente"):
		return ReasonDowngrade
	}
	for _, l := range lines {
		if m := innerCode.FindStringSubmatch(l); m != nil && m[1] == fmt.Sprint(msiNewerVersion) {
			return ReasonDowngrade
		}
	}
	return Explain(code, lines)
}

package install

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf16"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

// Status is the lifecycle of a step as reported to the UI.
type Status string

const (
	StatusStart   Status = "start"
	StatusOutput  Status = "output"
	StatusOK      Status = "ok"
	StatusSkipped Status = "skipped"
	StatusFailed  Status = "failed"
)

// Event is emitted while a plan runs.
type Event struct {
	Step   Step   `json:"step"`
	Status Status `json:"status"`
	Line   string `json:"line,omitempty"`
	Err    string `json:"error,omitempty"`
	// Reason is why a step failed, or "reboot" on a step that needs a restart; see Explain.
	Reason string `json:"reason,omitempty"`
	// Percent is download progress (1-100) on output lines that carry it.
	Percent int `json:"percent,omitempty"`
}

// Executor runs a command and streams its output lines. It exists so the
// runner can be tested without touching the machine.
type Executor interface {
	Run(ctx context.Context, name string, args []string, onLine func(string)) (exitCode int, err error)
}

// winget exit codes that mean "nothing to do".
const (
	wingetAlreadyInstalled = -1978335135 // 0x8A150061
	wingetNoUpgrade        = -1978335189 // 0x8A15002B: no newer version applies
)

// WingetArgs builds the install command line for an app.
func WingetArgs(app *catalog.App, version string) []string {
	args := []string{"install", "--id", app.Winget, "--exact", "--source", "winget",
		"--silent", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}
	if version != "" {
		args = append(args, "--version", version)
	}
	if app.Scope != "" {
		args = append(args, "--scope", app.Scope)
	}
	if app.Override != "" {
		args = append(args, "--override", app.Override)
	}
	return args
}

// WingetUninstallArgs builds the uninstall command line for an installed app.
func WingetUninstallArgs(app *catalog.App) []string {
	return []string{"uninstall", "--id", app.Winget, "--exact", "--source", "winget",
		"--silent", "--accept-source-agreements", "--disable-interactivity"}
}

// WingetUpgradeArgs builds the upgrade command line for an installed app.
func WingetUpgradeArgs(app *catalog.App) []string {
	args := []string{"upgrade", "--id", app.Winget, "--exact", "--source", "winget",
		"--silent", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}
	if app.Override != "" {
		args = append(args, "--override", app.Override)
	}
	return args
}

// Run executes the plan sequentially. A failed app does not stop the plan:
// independent apps still install, but steps depending on the failed app are
// skipped (dependencies and recipes `after`).
func Run(ctx context.Context, cat *catalog.Catalog, plan Plan, ex Executor, emit func(Event)) (failed []string) {
	broken := map[string]bool{}
	for _, s := range plan.Steps {
		if ctx.Err() != nil {
			emit(Event{Step: s, Status: StatusSkipped, Err: "cancelled"})
			continue
		}
		if dep := brokenDependency(cat, s, broken); dep != "" {
			broken[s.ID] = true
			emit(Event{Step: s, Status: StatusSkipped, Err: "depends on " + dep + ", which failed"})
			continue
		}
		emit(Event{Step: s, Status: StatusStart})
		// The last lines are kept so a failure can be explained from what the installer said.
		var tail []string
		track := func(e Event) {
			if e.Status == StatusOutput {
				if tail = append(tail, e.Line); len(tail) > 40 {
					tail = tail[1:]
				}
				e.Percent = max(ParsePercent(e.Line), 0)
			}
			emit(e)
		}
		var err error
		var code int
		switch s.Kind {
		case StepApp:
			code, err = runWinget(ctx, WingetArgs(cat.Apps[s.ID], s.Version), s, ex, track)
		case StepUpgrade:
			code, err = runWinget(ctx, WingetUpgradeArgs(cat.Apps[s.ID]), s, ex, track)
		case StepUninstall:
			code, err = runWinget(ctx, WingetUninstallArgs(cat.Apps[s.ID]), s, ex, track)
		case StepRecipe:
			code, err = runRecipe(ctx, cat.Recipes[s.ID], s, ex, track)
		}
		if err == nil && code == exitRebootRequired {
			emit(Event{Step: s, Status: StatusOK, Reason: ReasonReboot})
			continue
		}
		if err != nil {
			broken[s.ID] = true
			failed = append(failed, s.ID)
			emit(Event{Step: s, Status: StatusFailed, Err: err.Error(), Reason: Explain(code, tail)})
			continue
		}
		emit(Event{Step: s, Status: StatusOK})
	}
	return failed
}

func brokenDependency(cat *catalog.Catalog, s Step, broken map[string]bool) string {
	var deps []string
	switch s.Kind {
	case StepApp:
		deps = cat.Apps[s.ID].Requires
	case StepUpgrade:
		deps = nil
	case StepRecipe:
		deps = cat.Recipes[s.ID].After
	}
	for _, d := range deps {
		if broken[d] {
			return d
		}
	}
	return ""
}

// exitRebootRequired is the Windows Installer's "done, restart to finish".
const exitRebootRequired = 3010

func runWinget(ctx context.Context, args []string, s Step, ex Executor, emit func(Event)) (int, error) {
	onLine := func(l string) { emit(Event{Step: s, Status: StatusOutput, Line: l}) }
	code, err := ex.Run(ctx, "winget", args, onLine)
	if err != nil {
		return code, fmt.Errorf("could not run winget: %w", err)
	}
	if code != 0 && code != wingetAlreadyInstalled && code != wingetNoUpgrade && code != exitRebootRequired {
		return code, fmt.Errorf("winget exited with code %#x", uint32(code))
	}
	return code, nil
}

func runRecipe(ctx context.Context, r *catalog.Recipe, s Step, ex Executor, emit func(Event)) (int, error) {
	onLine := func(l string) { emit(Event{Step: s, Status: StatusOutput, Line: l}) }
	if strings.TrimSpace(r.Check) != "" {
		code, err := ex.Run(ctx, "powershell", PowerShellArgs(r.Check), func(string) {})
		if err == nil && code == 0 {
			onLine("already satisfied")
			return 0, nil
		}
	}
	code, err := ex.Run(ctx, "powershell", PowerShellArgs(r.PowerShell), onLine)
	if err != nil {
		return code, fmt.Errorf("could not run powershell: %w", err)
	}
	if code != 0 && code != exitRebootRequired {
		return code, fmt.Errorf("recipe exited with code %d", code)
	}
	return code, nil
}

// refreshPath makes tools installed earlier in this run visible: a child
// process inherits the environment WinForge started with, which predates them.
const refreshPath = `$env:Path = [Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User'); $ErrorActionPreference = 'Stop'; $global:LASTEXITCODE = 0;`

// PowerShellArgs wraps a script for powershell.exe. The script is passed as
// -EncodedCommand (UTF-16LE base64), which sidesteps every quoting problem.
func PowerShellArgs(script string) []string {
	full := refreshPath + "\n& {\n" + script + "\n}\nif ($LASTEXITCODE) { exit $LASTEXITCODE }"
	u := utf16.Encode([]rune(full))
	buf := make([]byte, 0, len(u)*2)
	for _, c := range u {
		buf = append(buf, byte(c), byte(c>>8))
	}
	return []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(buf)}
}

// OSExecutor runs real processes.
type OSExecutor struct{}

// Run implements Executor. winget draws progress with carriage returns, so
// output is split on both \r and \n.
func (OSExecutor) Run(ctx context.Context, name string, args []string, onLine func(string)) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return -1, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	sc.Split(splitCRLF)
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			onLine(l)
		}
	}
	err = cmd.Wait()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

func splitCRLF(data []byte, atEOF bool) (int, []byte, error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

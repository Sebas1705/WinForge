package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"unicode/utf16"
)

// maxBatch keeps one PowerShell command line well under Windows' length limit.
const maxBatch = 40

// Recycle moves files to the Recycle Bin, so a mistake can be undone. A file
// counts as moved only if it is gone afterwards; anything else (in use, no
// rights, not a plain file) is reported in failed and left where it was.
func Recycle(ctx context.Context, run Runner, paths []string) (moved int, freed int64, failed []string) {
	if run == nil {
		return 0, 0, append([]string(nil), paths...)
	}
	type item struct {
		path string
		size int64
	}
	var todo []item
	for _, p := range paths {
		st, err := os.Lstat(p)
		if err != nil || !st.Mode().IsRegular() {
			failed = append(failed, p)
			continue
		}
		todo = append(todo, item{p, st.Size()})
	}
	for start := 0; start < len(todo); start += maxBatch {
		end := min(start+maxBatch, len(todo))
		batch := make([]string, 0, end-start)
		for _, it := range todo[start:end] {
			batch = append(batch, it.path)
		}
		if ctx.Err() != nil {
			for _, it := range todo[start:] {
				failed = append(failed, it.path)
			}
			break
		}
		_, _ = run.Run(ctx, "powershell.exe", recycleArgs(batch)...)
		for _, it := range todo[start:end] {
			if _, err := os.Lstat(it.path); errors.Is(err, os.ErrNotExist) {
				moved++
				freed += it.size
			} else {
				failed = append(failed, it.path)
			}
		}
	}
	return moved, freed, failed
}

// recycleArgs builds the PowerShell command that sends the paths to the
// Recycle Bin. The paths travel as base64 JSON inside an encoded command, so no
// character in a file name can be read as part of the command.
func recycleArgs(paths []string) []string {
	j, _ := json.Marshal(paths)
	script := `Add-Type -AssemblyName Microsoft.VisualBasic
$paths = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString(j) + `')) | ConvertFrom-Json
foreach ($p in $paths) {
    try { [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin') } catch { }
}`
	u := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(u)*2)
	for _, c := range u {
		buf = append(buf, byte(c), byte(c>>8))
	}
	return []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", base64.StdEncoding.EncodeToString(buf)}
}

// OSRunner runs real commands, hidden, and stops them when ctx is cancelled.
type OSRunner struct{}

// Run implements Runner.
func (OSRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	return cmd.CombinedOutput()
}

// RecycleResult is what moving files to the Recycle Bin did, as the interface sees it.
type RecycleResult struct {
	Moved      int      `json:"moved"`
	FreedBytes int64    `json:"freedBytes"`
	Failed     []string `json:"failed"`
}

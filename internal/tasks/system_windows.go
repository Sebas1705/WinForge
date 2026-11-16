//go:build windows

package tasks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

type osSystem struct{}

// NewSystem is the real machine: PowerShell's ScheduledTasks module to read, and schtasks.exe for
// the one change WinForge makes.
func NewSystem() System { return osSystem{} }

func (osSystem) Getenv(name string) string { return os.Getenv(name) }

func (osSystem) CurrentUser() []string {
	u, err := user.Current()
	if err != nil {
		return nil
	}
	out := []string{u.Username, u.Uid}
	if i := strings.LastIndexByte(u.Username, '\\'); i >= 0 {
		out = append(out, u.Username[i+1:])
	}
	return out
}

// listScript reads every task outside \Microsoft\ with its state and XML definition. Output is
// base64 UTF-8 JSON so no character is lost to the console code page.
const listScript = `$ErrorActionPreference='SilentlyContinue'
$ProgressPreference='SilentlyContinue'
$out=@()
foreach($t in @(Get-ScheduledTask)){
  if($t.TaskPath -like '\Microsoft\*'){continue}
  $x=''
  try{$x=[string](Export-ScheduledTask -TaskName $t.TaskName -TaskPath $t.TaskPath)}catch{}
  $out+=[pscustomobject]@{f=[string]$t.TaskPath;n=[string]$t.TaskName;s=[int]$t.State;x=$x}
}
$j=ConvertTo-Json -InputObject @($out) -Compress -Depth 3
[Console]::Out.Write([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($j)))
`

// resultScript reads the last run result of the tasks given on stdin as base64 JSON [{f,n}].
const resultScript = `$ErrorActionPreference='SilentlyContinue'
$ProgressPreference='SilentlyContinue'
$in=[Console]::In.ReadToEnd().Trim()
$items=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($in))|ConvertFrom-Json
$out=@()
foreach($i in @($items)){
  $r=Get-ScheduledTaskInfo -TaskPath $i.f -TaskName $i.n
  if($r){$out+=[pscustomobject]@{f=[string]$i.f;n=[string]$i.n;r=[int64]$r.LastTaskResult}}
}
$j=ConvertTo-Json -InputObject @($out) -Compress
[Console]::Out.Write([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($j)))
`

func powershell(script, stdin string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	u := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(u)*2)
	for _, c := range u {
		buf = append(buf, byte(c), byte(c>>8))
	}
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(buf))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stdout.String()))
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (osSystem) List() ([]Raw, error) {
	raw, err := powershell(listScript, "", 90*time.Second)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		F string `json:"f"`
		N string `json:"n"`
		S int    `json:"s"`
		X string `json:"x"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]Raw, 0, len(rows))
	for _, r := range rows {
		out = append(out, Raw{Folder: r.F, Name: r.N, State: r.S, XML: r.X})
	}
	return out, nil
}

func (osSystem) LastResults(ids []string) map[string]int32 {
	type in struct {
		F string `json:"f"`
		N string `json:"n"`
	}
	list := make([]in, 0, len(ids))
	for _, id := range ids {
		f, n := SplitID(id)
		list = append(list, in{f, n})
	}
	b, _ := json.Marshal(list)
	raw, err := powershell(resultScript, base64.StdEncoding.EncodeToString(b), 90*time.Second)
	if err != nil {
		return nil // results are best effort
	}
	var rows []struct {
		F string `json:"f"`
		N string `json:"n"`
		R int64  `json:"r"`
	}
	if json.Unmarshal(raw, &rows) != nil {
		return nil
	}
	out := make(map[string]int32, len(rows))
	for _, r := range rows {
		out[MakeID(r.F, r.N)] = int32(uint32(r.R))
	}
	return out
}

// SetEnabled runs `schtasks /Change /TN <id> /ENABLE|/DISABLE`: only the enabled flag changes.
// The task is not run, deleted or edited. The id is its own argument, never through a shell.
func (osSystem) SetEnabled(id string, enabled bool) error {
	if !strings.HasPrefix(id, `\`) || strings.TrimSpace(id) == `\` {
		return ErrNotFound
	}
	flag := "/DISABLE"
	if enabled {
		flag = "/ENABLE"
	}
	cmd := exec.Command("schtasks.exe", "/Change", "/TN", id, flag)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New("task:failed: " + strings.TrimSpace(string(out)))
	}
	return nil
}

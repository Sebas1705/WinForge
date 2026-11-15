//go:build windows

package startup

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

const (
	runPath      = `Software\Microsoft\Windows\CurrentVersion\Run`
	runPath32    = `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`
	approvedBase = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\`
)

type osSystem struct{}

// NewSystem is the real machine: the registry, the Startup folders and PowerShell for file details.
func NewSystem() System { return osSystem{} }

func root(o Origin) registry.Key {
	if o.Machine() {
		return registry.LOCAL_MACHINE
	}
	return registry.CURRENT_USER
}

func (osSystem) Getenv(name string) string { return os.Getenv(name) }

func (osSystem) Exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func folderOf(o Origin) string {
	switch o {
	case UserFolder:
		if v := os.Getenv("APPDATA"); v != "" {
			return filepath.Join(v, `Microsoft\Windows\Start Menu\Programs\Startup`)
		}
	case CommonFolder:
		if v := os.Getenv("ProgramData"); v != "" {
			return filepath.Join(v, `Microsoft\Windows\Start Menu\Programs\StartUp`)
		}
	}
	return ""
}

func (osSystem) Entries(o Origin) ([]Raw, error) {
	if o.Folder() {
		dir := folderOf(o)
		if dir == "" {
			return nil, nil
		}
		list, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, nil
			}
			return nil, err
		}
		var out []Raw
		for _, e := range list {
			// desktop.ini only decorates the folder; Windows does not start it.
			if e.IsDir() || strings.EqualFold(e.Name(), "desktop.ini") {
				continue
			}
			out = append(out, Raw{Name: e.Name(), Command: filepath.Join(dir, e.Name())})
		}
		return out, nil
	}
	path := runPath
	if o == MachineRun32 {
		path = runPath32
	}
	k, err := registry.OpenKey(root(o), path, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	var out []Raw
	for _, n := range names {
		if n == "" {
			continue // the key's default value is not a startup item
		}
		v, _, err := k.GetStringValue(n)
		if err != nil {
			// Not a string (or unreadable): not something Windows starts, so not listed.
			continue
		}
		out = append(out, Raw{Name: n, Command: v})
	}
	return out, nil
}

func (osSystem) Approved(o Origin) (map[string][]byte, error) {
	k, err := registry.OpenKey(root(o), approvedBase+o.ApprovedKey(), registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, n := range names {
		b, typ, err := k.GetBinaryValue(n)
		if err != nil || typ != registry.BINARY {
			continue
		}
		out[strings.ToLower(n)] = b
	}
	return out, nil
}

func (osSystem) SetApproved(o Origin, name string, data []byte) error {
	k, _, err := registry.CreateKey(root(o), approvedBase+o.ApprovedKey(), registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return ErrNeedsAdmin
		}
		return err
	}
	defer k.Close()
	return k.SetBinaryValue(name, data)
}

// inspectScript reads, for every path, the publisher (CompanyName) and where a shortcut points,
// in one PowerShell run. Input and output travel as base64 UTF-8 JSON so no character is lost.
const inspectScript = `$ErrorActionPreference='SilentlyContinue'
$ProgressPreference='SilentlyContinue'
$in=[Console]::In.ReadToEnd().Trim()
$paths=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($in))|ConvertFrom-Json
$sh=$null
$out=@()
foreach($p in @($paths)){
  $t='';$f=$p
  if($p -like '*.lnk'){
    if(-not $sh){$sh=New-Object -ComObject WScript.Shell}
    try{$t=[string]$sh.CreateShortcut($p).TargetPath}catch{}
    if($t){$f=$t}
  }
  $c=''
  try{if(Test-Path -LiteralPath $f -PathType Leaf){$c=[string](Get-Item -LiteralPath $f).VersionInfo.CompanyName}}catch{}
  $out+=[pscustomobject]@{p=[string]$p;t=$t;c=$c}
}
$j=ConvertTo-Json -InputObject @($out) -Compress
[Console]::Out.Write([Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($j)))
`

func (osSystem) Inspect(paths []string) map[string]FileInfo {
	if len(paths) == 0 {
		return nil
	}
	in, _ := json.Marshal(paths)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	u := utf16.Encode([]rune(inspectScript))
	buf := make([]byte, 0, len(u)*2)
	for _, c := range u {
		buf = append(buf, byte(c), byte(c>>8))
	}
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(buf))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Stdin = strings.NewReader(base64.StdEncoding.EncodeToString(in))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil // publishers are best effort
	}
	return decodeInspect(stdout.Bytes())
}

// decodeInspect reads the script's output. Anything it cannot read gives no details.
func decodeInspect(out []byte) map[string]FileInfo {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil {
		return nil
	}
	var rows []struct{ P, T, C string }
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}
	res := make(map[string]FileInfo, len(rows))
	for _, r := range rows {
		res[r.P] = FileInfo{Publisher: r.C, Target: r.T}
	}
	return res
}

//go:build windows

package sched

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

type winSystem struct{}

// NewSystem is this PC: schtasks.exe for the task, PowerShell for the toast, the current user's
// registry for the URL protocol. Nothing here needs administrator rights.
func NewSystem() System { return winSystem{} }

func system32(parts ...string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(append([]string{root, "System32"}, parts...)...)
}

// run starts a Windows tool hidden, with a time limit, and returns what it printed.
func run(timeout time.Duration, env []string, name string, args ...string) (stdout []byte, exit int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ctx.Err() == nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.Bytes(), ee.ExitCode(), fmt.Errorf("%w (exit %d) %s", ErrTask, ee.ExitCode(), msg)
	}
	if err != nil {
		return nil, -1, fmt.Errorf("%w: %v", ErrTask, err)
	}
	return out.Bytes(), 0, nil
}

func (winSystem) Query() (TaskInfo, error) {
	out, exit, err := run(30*time.Second, nil, system32("schtasks.exe"), "/Query", "/TN", TaskName, "/XML")
	if exit > 0 {
		return TaskInfo{}, nil // schtasks answers a task that is not there with a failure exit code
	}
	if err != nil {
		return TaskInfo{}, err
	}
	return ParseTaskXML(out)
}

func (winSystem) Create(t Task) error {
	doc, err := BuildTaskXML(t, time.Now())
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "winforge-task-*.xml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(EncodeUTF16(doc)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_, _, err = run(60*time.Second, nil, system32("schtasks.exe"), "/Create", "/TN", TaskName, "/XML", f.Name(), "/F")
	return err
}

func (s winSystem) Delete() error {
	info, err := s.Query()
	if err != nil {
		return err
	}
	if !info.Exists {
		return nil
	}
	_, _, err = run(30*time.Second, nil, system32("schtasks.exe"), "/Delete", "/TN", TaskName, "/F")
	return err
}

// toastScript shows one toast. The text and the link arrive in environment variables and are put
// into the notification through the XML DOM (InnerText), so nothing in them is ever parsed as script
// or markup. It registers a per-user application name so the toast says "WinForge", not "PowerShell".
const toastScript = `
$ErrorActionPreference = 'Stop'
$aumid = 'WinForge'
$key = 'HKCU:\Software\Classes\AppUserModelId\' + $aumid
if (-not (Test-Path $key)) { New-Item -Path $key -Force | Out-Null }
New-ItemProperty -Path $key -Name DisplayName -Value 'WinForge' -PropertyType String -Force | Out-Null
[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
[void][Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime]
$doc = New-Object Windows.Data.Xml.Dom.XmlDocument
$doc.LoadXml('<toast><visual><binding template="ToastGeneric"><text/><text/></binding></visual></toast>')
$texts = $doc.GetElementsByTagName('text')
$texts.Item(0).InnerText = $env:WINFORGE_TOAST_TITLE
$texts.Item(1).InnerText = $env:WINFORGE_TOAST_BODY
if ($env:WINFORGE_TOAST_LAUNCH) {
  $doc.DocumentElement.SetAttribute('activationType', 'protocol')
  $doc.DocumentElement.SetAttribute('launch', $env:WINFORGE_TOAST_LAUNCH)
}
$toast = New-Object Windows.UI.Notifications.ToastNotification $doc
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($aumid).Show($toast)
`

// ToastScript is the PowerShell the toast runs (exported for tests that parse it).
func ToastScript() string { return toastScript }

func encodePowerShell(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 0, 2*len(u))
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return base64.StdEncoding.EncodeToString(b)
}

func (winSystem) Toast(t Toast) error {
	_, _, err := run(30*time.Second,
		[]string{"WINFORGE_TOAST_TITLE=" + t.Title, "WINFORGE_TOAST_BODY=" + t.Body, "WINFORGE_TOAST_LAUNCH=" + t.Launch},
		system32("WindowsPowerShell", "v1.0", "powershell.exe"),
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encodePowerShell(toastScript))
	return err
}

const protocolKey = `Software\Classes\winforge`

func (winSystem) RegisterProtocol(exe string) error {
	if exe == "" || strings.ContainsAny(exe, "\"\r\n\x00") {
		return ErrInvalid
	}
	root, _, err := registry.CreateKey(registry.CURRENT_USER, protocolKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.SetStringValue("", "URL:WinForge"); err != nil {
		return err
	}
	if err := root.SetStringValue("URL Protocol", ""); err != nil {
		return err
	}
	cmd, _, err := registry.CreateKey(registry.CURRENT_USER, protocolKey+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer cmd.Close()
	return cmd.SetStringValue("", `"`+exe+`" "%1"`)
}

func (winSystem) UnregisterProtocol() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, protocolKey+`\shell\open\command`, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	v, _, _ := k.GetStringValue("")
	k.Close()
	if !strings.Contains(strings.ToLower(v), "winforge") {
		return nil // somebody else's winforge:// handler: not ours to remove
	}
	for _, sub := range []string{`\shell\open\command`, `\shell\open`, `\shell`, ``} {
		if err := registry.DeleteKey(registry.CURRENT_USER, protocolKey+sub); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
	}
	return nil
}

// SystemLanguage is the Windows display language, such as "es-ES".
func SystemLanguage() string {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")
	buf := make([]uint16, 85)
	n, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

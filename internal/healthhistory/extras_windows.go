//go:build windows

package healthhistory

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf16"
)

func readReliability(ctx context.Context) ([]DiskStat, error) {
	u := utf16.Encode([]rune(reliabilityScript))
	buf := make([]byte, 0, len(u)*2)
	for _, c := range u {
		buf = append(buf, byte(c), byte(c>>8))
	}
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(buf))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if m := strings.TrimSpace(stderr.String()); m != "" {
			return nil, errors.New(m)
		}
		return nil, err
	}
	return ParseReliability(stdout.Bytes()), nil
}

package health

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
)

// CollectSteps lists the collector's stages in the order it reports them.
var CollectSteps = []string{"system", "firmware", "security", "hardware", "storage", "drivers", "devices", "report"}

// Collect scans the PC. It takes 10-40 seconds, mostly reading the driver list.
func Collect(ctx context.Context) (*Report, error) { return CollectWith(ctx, nil) }

// CollectWith is Collect that calls onStep with the name of each stage as the
// collector starts it, so the UI can show what is being read.
func CollectWith(ctx context.Context, onStep func(step string)) (*Report, error) {
	out, err := runPowerShell(ctx, collectScript, 2*time.Minute, onStep)
	if err != nil {
		return nil, err
	}
	return ParseReport(out, time.Now())
}

// stepWriter keeps everything written and reports each "##STEP name" line.
type stepWriter struct {
	buf    bytes.Buffer
	onStep func(string)
	line   []byte
}

func (w *stepWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	if w.onStep == nil {
		return len(p), nil
	}
	for _, c := range p {
		if c != '\n' {
			w.line = append(w.line, c)
			continue
		}
		if s := strings.TrimSpace(string(w.line)); strings.HasPrefix(s, "##STEP ") {
			w.onStep(strings.TrimPrefix(s, "##STEP "))
		}
		w.line = w.line[:0]
	}
	return len(p), nil
}

// ScanUpdates searches Windows Update for pending items, drivers and firmware
// included. It never installs anything.
func ScanUpdates(ctx context.Context) (*UpdateScan, error) {
	out, err := runPowerShell(ctx, updateScript, 4*time.Minute, nil)
	if err != nil {
		return nil, err
	}
	return ParseUpdates(out, time.Now())
}

// ParseReport decodes the collector's JSON and fills derived fields.
func ParseReport(out []byte, now time.Time) (*Report, error) {
	var r Report
	if err := json.Unmarshal(jsonPart(out), &r); err != nil {
		return nil, fmt.Errorf("could not read the scan result: %w", err)
	}
	r.CollectedAt = now
	r.normalize()
	return &r, nil
}

// ParseUpdates decodes the Windows Update search result.
func ParseUpdates(out []byte, now time.Time) (*UpdateScan, error) {
	var s UpdateScan
	if err := json.Unmarshal(jsonPart(out), &s); err != nil {
		return nil, fmt.Errorf("could not read the Windows Update result: %w", err)
	}
	s.CheckedAt = now
	if s.Updates == nil {
		s.Updates = []Update{}
	}
	return &s, nil
}

// jsonPart drops a UTF-8 BOM and anything printed before the JSON object.
func jsonPart(b []byte) []byte {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if i := bytes.IndexByte(b, '{'); i > 0 {
		b = b[i:]
	}
	return bytes.TrimSpace(b)
}

func runPowerShell(ctx context.Context, script string, timeout time.Duration, onStep func(string)) ([]byte, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("PC health is only available on Windows")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	u := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(u)*2)
	for _, c := range u {
		buf = append(buf, byte(c), byte(c>>8))
	}
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-EncodedCommand", base64.StdEncoding.EncodeToString(buf))
	hide(cmd)
	stdout := &stepWriter{onStep: onStep}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("timed out after %s", timeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return stdout.buf.Bytes(), nil
}

// normalize fills derived fields and replaces nil slices with empty ones so the
// UI never receives null.
func (r *Report) normalize() {
	for i := range r.GPUs {
		g := &r.GPUs[i]
		g.Vendor = gpuVendor(g.Vendor, g.Name)
	}
	if r.GPUs == nil {
		r.GPUs = []GPU{}
	}
	if r.Memory.Modules == nil {
		r.Memory.Modules = []Module{}
	}
	if r.Disks == nil {
		r.Disks = []Disk{}
	}
	if r.Volumes == nil {
		r.Volumes = []Volume{}
	}
	if r.Drivers == nil {
		r.Drivers = []Driver{}
	}
	if r.Problems == nil {
		r.Problems = []Problem{}
	}
	if r.Errors == nil {
		r.Errors = []string{}
	}
	if r.Security.AntiVirus == nil {
		r.Security.AntiVirus = []AV{}
	}
	if r.Security.Firewall == nil {
		r.Security.Firewall = []Firewall{}
	}
}

// gpuVendor reduces the adapter's vendor string and name to a known vendor.
func gpuVendor(vendor, name string) string {
	s := strings.ToLower(vendor + " " + name)
	switch {
	case strings.Contains(s, "nvidia"):
		return "NVIDIA"
	case strings.Contains(s, "amd") || strings.Contains(s, "advanced micro") || strings.Contains(s, "radeon") || strings.Contains(s, "ati "):
		return "AMD"
	case strings.Contains(s, "intel"):
		return "Intel"
	case strings.Contains(s, "microsoft"):
		return "Microsoft"
	}
	return "Other"
}

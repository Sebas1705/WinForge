package battery

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// Read asks Windows for the battery report and parses it. The report goes to a temporary
// folder that is removed before returning, whatever happened.
func Read(ctx context.Context) (Report, error) { return ReadWith(ctx, runPowercfg) }

// ReadWith is Read with the program that writes the XML report to path replaced, for tests.
func ReadWith(ctx context.Context, run func(ctx context.Context, path string) error) (Report, error) {
	dir, err := os.MkdirTemp("", "winforge-battery-")
	if err != nil {
		return Report{}, err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	path := filepath.Join(dir, "battery.xml")
	if err := run(ctx, path); err != nil {
		return Report{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	return Parse(b)
}

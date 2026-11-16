//go:build !windows

package battery

import (
	"context"
	"errors"
)

func runPowercfg(context.Context, string) error {
	return errors.New("the battery report is only available on Windows")
}

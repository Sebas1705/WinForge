//go:build !windows

package healthhistory

import "context"

func readReliability(context.Context) ([]DiskStat, error) { return nil, nil }

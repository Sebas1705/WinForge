package pcread

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	defaultLimit = 50
	maxLimit     = 200
	maxRoots     = 8
	maxText      = 260 // longest command line or path kept in a result
)

// limitProp is the schema member every list tool shares.
const limitProp = `"limit":{"type":"integer","minimum":1,"maximum":200,"default":50,"description":"Most items to return (1-200, default 50). The result says truncated=true when more exist."}`

// limitArg checks an optional limit: absent means 50, anything outside 1..200 is refused so the model learns
// the cap instead of silently getting less.
func limitArg(p *int) (int, error) {
	if p == nil {
		return defaultLimit, nil
	}
	if *p < 1 || *p > maxLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d (default %d)", maxLimit, defaultLimit)
	}
	return *p, nil
}

// capped returns at most limit items, never nil, and whether items were left out.
func capped[T any](in []T, limit int) ([]T, bool) {
	if len(in) > limit {
		return append([]T{}, in[:limit]...), true
	}
	return append([]T{}, in...), false
}

// short cuts a long text to n runes.
func short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// runCtx runs a backend that does not take a context and stops waiting when ctx is cancelled. The backend
// goroutine finishes on its own; its result is dropped. A panic in the backend becomes an error.
func runCtx[T any](ctx context.Context, f func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				var zero T
				ch <- result{zero, errors.New("the backend stopped unexpectedly")}
			}
		}()
		v, err := f()
		ch <- result{v, err}
	}()
	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case r := <-ch:
		return r.v, r.err
	}
}

var (
	driveAbs  = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	driveRoot = regexp.MustCompile(`^[A-Za-z]:[\\/]*$`)
	reserved  = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true, "conin$": true, "conout$": true}
)

func isReservedName(seg string) bool {
	base := strings.ToLower(seg)
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, " ")
	if reserved[base] {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '0' && base[3] <= '9'
}

// checkFolder validates a folder path a person named, before anything touches the disk: absolute, no UNC or
// device path, no "..", no stream or wildcard characters, no reserved device names, and a drive root only
// with allowDriveRoot. It returns the cleaned path.
func checkFolder(p string, allowDriveRoot bool) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.New("a folder path is required")
	}
	if strings.ContainsAny(p, "\x00\r\n") {
		return "", errors.New("the path contains control characters")
	}
	if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") {
		return "", errors.New("network (UNC) and device paths are not allowed; name a folder on a local drive")
	}
	abs := driveAbs.MatchString(p)
	if !abs && !onWindows && strings.HasPrefix(p, "/") {
		abs = true
	}
	if !abs {
		return "", errors.New("the path must be absolute, like C:\\Users\\me\\Downloads (relative paths are not allowed)")
	}
	rest := p
	if driveAbs.MatchString(p) {
		rest = p[2:]
	}
	if strings.ContainsAny(rest, ":*?\"<>|") {
		return "", errors.New("the path contains characters that are not allowed")
	}
	for _, seg := range strings.FieldsFunc(rest, func(r rune) bool { return r == '\\' || r == '/' }) {
		if seg == ".." {
			return "", errors.New("paths with .. are not allowed")
		}
		if isReservedName(seg) {
			return "", errors.New("device names are not allowed in the path")
		}
	}
	isRoot := driveRoot.MatchString(p) || (!driveAbs.MatchString(p) && strings.Trim(p, "/") == "")
	if isRoot && !allowDriveRoot {
		return "", errors.New("a whole drive is refused unless allowDriveRoot is true; name a folder, or confirm with the person that scanning the whole drive is wanted")
	}
	return filepath.Clean(p), nil
}

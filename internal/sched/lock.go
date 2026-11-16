package sched

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunTimeout is the hard limit of one run. The scheduled task's own limit (ExecutionLimit) is longer,
// so the run ends itself, writes its log and releases the lock before Windows would have to kill it.
const RunTimeout = 45 * time.Minute

// staleAfter is how old a lock file must be before it is taken to be left by a crashed run.
const staleAfter = RunTimeout + 15*time.Minute

// Lock makes one run at a time: the scheduled task, "Run now" and a command typed in a terminal all
// take it. It is a file created exclusively, so it works across processes.
type Lock struct {
	Path string
	Now  func() time.Time
}

// Acquire takes the lock or returns ErrBusy. The returned function releases it. A lock older than
// staleAfter belongs to a run that died, and is taken over.
func (l Lock) Acquire() (release func(), err error) {
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d %s\n", os.Getpid(), now().UTC().Format(time.RFC3339))
			f.Close()
			return func() { _ = os.Remove(l.Path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		st, serr := os.Stat(l.Path)
		if serr != nil {
			continue // released between the two calls: try again
		}
		if now().Sub(st.ModTime()) < staleAfter {
			return nil, ErrBusy
		}
		_ = os.Remove(l.Path)
	}
	return nil, ErrBusy
}

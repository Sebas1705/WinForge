package wingetpkgs

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultRepoURL is the winget-pkgs repository a Mirror clones.
const DefaultRepoURL = "https://github.com/microsoft/winget-pkgs.git"

// Mirror is a local, shallow, blobless, sparse clone of winget-pkgs. One clone
// lists every manifest path (about 40 MB of trees, seconds to fetch), and
// Prefetch downloads only the version folders a run needs in a single batch,
// so a bulk run makes a handful of git requests instead of thousands of
// throttled HTTP ones.
type Mirror struct {
	Dir   string
	env   []string
	paths []string // every file under manifests/, sorted
}

// OpenMirror clones repoURL into dir (shallow, no blobs, nothing checked out)
// or reuses a clone already there, and indexes its manifest paths. token, if
// set, authenticates the git traffic, which GitHub otherwise throttles per IP.
func OpenMirror(ctx context.Context, dir, repoURL, token string) (*Mirror, error) {
	if repoURL == "" {
		repoURL = DefaultRepoURL
	}
	m := &Mirror{Dir: dir, env: append(os.Environ(), "GIT_TERMINAL_PROMPT=0")}
	if token != "" && strings.HasPrefix(repoURL, "https://") {
		// Passed through the environment, so the token is neither on a
		// command line nor written to the clone's config.
		basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		m.env = append(m.env, "GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic "+basic)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return nil, err
		}
		err := retryGit(ctx, func() error {
			_ = os.RemoveAll(dir) // a failed attempt may leave a partial clone
			_, err := m.git(ctx, "", filepath.Dir(dir), "clone", "--quiet", "--depth=1", "--filter=blob:none",
				"--no-checkout", "--sparse", repoURL, filepath.Base(dir))
			return err
		})
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	out, err := m.git(ctx, "", dir, "ls-tree", "-r", "-z", "--name-only", "HEAD", "manifests/")
	if err != nil {
		return nil, err
	}
	for p := range bytes.SplitSeq(out, []byte{0}) {
		if len(p) > 0 {
			m.paths = append(m.paths, string(p))
		}
	}
	sort.Strings(m.paths)
	if len(m.paths) == 0 {
		return nil, fmt.Errorf("%s: no manifests in the clone", dir)
	}
	return m, nil
}

// gitThrottled matches git's report of GitHub throttling or a transient
// server error.
func gitThrottled(err error) bool {
	s := err.Error()
	for _, k := range []string{" 429", " 500", " 502", " 503", " 504", "RPC failed", "rate-limited", "rate limit"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// retryGit retries a git network operation GitHub throttled, with
// exponential backoff and jitter, for at most DefaultMaxWait in total.
func retryGit(ctx context.Context, op func() error) error {
	var waited time.Duration
	for attempt := 0; ; attempt++ {
		err := op()
		if err == nil || !gitThrottled(err) {
			return err
		}
		wait := 5*time.Second<<min(attempt, 4) + time.Duration(rand.Int64N(int64(2*time.Second)))
		if waited+wait > DefaultMaxWait {
			return err
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
		waited += wait
	}
}

func (m *Mirror) git(ctx context.Context, stdin, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir, cmd.Env = dir, m.env
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// has reports whether the file exists in the repository.
func (m *Mirror) has(path string) bool {
	i := sort.SearchStrings(m.paths, path)
	return i < len(m.paths) && m.paths[i] == path
}

// subdirs lists the folders directly under dir; ok is false when dir does not
// exist at all.
func (m *Mirror) subdirs(dir string) (names []string, ok bool) {
	prefix := dir + "/"
	seen := map[string]bool{}
	for i := sort.SearchStrings(m.paths, prefix); i < len(m.paths) && strings.HasPrefix(m.paths[i], prefix); i++ {
		ok = true
		rest := m.paths[i][len(prefix):]
		if name, _, nested := strings.Cut(rest, "/"); nested && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, ok
}

// read returns a file's content; ok is false when the repository has no such
// file. Files Prefetch did not check out are fetched one by one.
func (m *Mirror) read(ctx context.Context, path string) ([]byte, bool, error) {
	if !m.has(path) {
		return nil, false, nil
	}
	b, err := os.ReadFile(filepath.Join(m.Dir, filepath.FromSlash(path)))
	if err == nil {
		return b, true, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, false, err
	}
	err = retryGit(ctx, func() error {
		var gerr error
		b, gerr = m.git(ctx, "", m.Dir, "cat-file", "blob", "HEAD:"+path)
		return gerr
	})
	return b, err == nil, err
}

// Prefetch checks out, in one batch, the newest version folder holding an
// installer manifest of each package, which is what Client.Package reads.
func (m *Mirror) Prefetch(ctx context.Context, ids []string) error {
	c := &Client{Mirror: m}
	var dirs []string
	for _, id := range ids {
		versions, err := c.Versions(ctx, id)
		if err != nil {
			continue // Package reports it
		}
		for i, v := range versions {
			if i == 6 {
				break
			}
			d := ManifestDir(id) + "/" + v
			if m.has(d + "/" + id + ".installer.yaml") {
				dirs = append(dirs, d)
				break
			}
		}
	}
	if len(dirs) == 0 {
		return nil
	}
	if _, err := m.git(ctx, strings.Join(dirs, "\n")+"\n", m.Dir, "sparse-checkout", "set", "--cone", "--stdin"); err != nil {
		return err
	}
	return retryGit(ctx, func() error {
		_, err := m.git(ctx, "", m.Dir, "checkout", "--quiet")
		return err
	})
}

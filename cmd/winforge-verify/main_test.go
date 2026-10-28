package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Sebas1705/WinForge/internal/wingetpkgs"
)

func TestClassify(t *testing.T) {
	limited := fmt.Errorf("lookup: %w", &wingetpkgs.RateLimitError{URL: "u", Status: 429})
	cases := map[string]struct {
		err  error
		want outcome
	}{
		"ok":            {nil, verified},
		"rate limited":  {limited, unverified},
		"bare sentinel": {wingetpkgs.ErrRateLimited, unverified},
		"publisher":     {errors.New(`publisher is "X", catalog expects "Y"`), failed},
		"not found":     {errors.New("package A.B not found in winget-pkgs"), failed},
		"http":          {errors.New(`installer "http://x" is not an https URL`), failed},
	}
	for name, c := range cases {
		if got := classify(c.err); got != c.want {
			t.Errorf("%s: classify = %d, want %d", name, got, c.want)
		}
	}
}

func TestMergeLockKeepsUnverifiedEntries(t *testing.T) {
	prev := map[string]*LockEntry{
		"a": {Version: "1"}, "b": {Version: "1"}, "gone": {Version: "1"},
	}
	ids := []string{"a", "b", "new"}
	entries := []*LockEntry{{Version: "2"}, nil, nil}
	got := mergeLock(prev, ids, entries, []outcome{verified, unverified, unverified})
	if got["a"].Version != "2" {
		t.Errorf("verified entry not refreshed: %+v", got["a"])
	}
	if got["b"] == nil || got["b"].Version != "1" {
		t.Errorf("unverified entry lost its previous data: %+v", got["b"])
	}
	if _, ok := got["new"]; ok {
		t.Error("unverified app without previous data got an entry")
	}
	if _, ok := got["gone"]; ok {
		t.Error("app no longer in the catalog kept its entry")
	}
}

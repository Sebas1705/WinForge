package install_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebas1705/WinForge/internal/install"
)

// TestMain doubles as the helper processes used by the OSExecutor tests: the
// test binary re-runs itself as a "parent" that spawns a long-lived "child"
// (the grandchild of the executor) which inherits the output pipe.
func TestMain(m *testing.M) {
	switch os.Getenv("WF_HELPER") {
	case "parent":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "WF_HELPER=child")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		os.Stdout.WriteString("started\n")
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "child":
		time.Sleep(20 * time.Second)
		os.Exit(0)
	case "lines":
		os.Stdout.WriteString("one\rtwo\r\nthree\n\n  four  ")
		os.Stderr.WriteString("\nerr")
		os.Exit(7)
	}
	os.Exit(m.Run())
}

func helper(mode string) func() {
	old, had := os.LookupEnv("WF_HELPER")
	os.Setenv("WF_HELPER", mode)
	return func() {
		if had {
			os.Setenv("WF_HELPER", old)
		} else {
			os.Unsetenv("WF_HELPER")
		}
	}
}

func TestCancelReturnsPromptlyWhileAGrandchildLives(t *testing.T) {
	defer helper("parent")()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	var once sync.Once
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = install.OSExecutor{}.Run(ctx, os.Args[0], nil, func(l string) {
			if l == "started" {
				once.Do(func() { close(started) })
			}
		})
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("helper never started")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("Run still blocked 8s after cancel: a grandchild holds the output pipe")
	}
}

func TestOSExecutorSplitsLinesAndReturnsExitCode(t *testing.T) {
	defer helper("lines")()
	var got []string
	code, err := install.OSExecutor{}.Run(context.Background(), os.Args[0], nil, func(l string) { got = append(got, l) })
	if err != nil || code != 7 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if want := "one|two|three|four|err"; strings.Join(got, "|") != want {
		t.Fatalf("lines = %q, want %q", strings.Join(got, "|"), want)
	}
}

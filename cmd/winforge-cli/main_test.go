package main

import (
	"strings"
	"testing"

	"github.com/Sebas1705/WinForge/internal/sched"
)

func TestUsageListsTheUpdatesCommand(t *testing.T) {
	if !strings.Contains(usage, "winforge-cli updates [--mode notify|apply] [--scheduled]") || !strings.Contains(usage, "updates --help") {
		t.Errorf("the usage text does not describe `updates`:\n%s", usage)
	}
}

func TestUpdatesIsAnsweredBeforeTheCatalogLoads(t *testing.T) {
	// main() hands `updates ...` to the shared runner; its --help is the long text and needs nothing from this PC.
	if !sched.IsUpdatesCommand([]string{"winforge-cli", "updates", "--help"}) {
		t.Fatal("`winforge-cli updates` must reach the shared runner")
	}
	if sched.IsUpdatesCommand([]string{"winforge-cli", "health", "updates"}) {
		t.Fatal("`health updates` is another command")
	}
}

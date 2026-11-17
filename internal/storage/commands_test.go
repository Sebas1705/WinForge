package storage_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/Sebas1705/WinForge/internal/storage"
)

// CommandsFor is what a plan shows the person, so it must be what Clean really runs.
func TestCommandsForMatchesWhatCleanRuns(t *testing.T) {
	env := fakeEnv(t)
	env.SystemDrive = "X:"
	for _, id := range []string{"recycle-bin", "docker-build-cache", "docker-dangling", "windows-old"} {
		run := &fakeRunner{}
		o := opts(env)
		o.Runner = run
		o.Admin = true
		o.RecycleRoots = []string{}
		storage.Clean(context.Background(), []string{id}, o)
		got := storage.CommandsFor(id, env)
		// Docker is also asked for its sizes before and after; those are reads and are not listed.
		var runs [][]string
		for _, c := range run.calls {
			if c[0] == "docker" && len(c) > 1 && (c[1] == "system" || c[1] == "images") {
				continue
			}
			runs = append(runs, c)
		}
		if !reflect.DeepEqual(got, runs) {
			t.Errorf("%s: CommandsFor = %v, Clean ran %v", id, got, runs)
		}
	}
	if storage.CommandsFor("temp-user", env) != nil || storage.CommandsFor("nope", env) != nil {
		t.Error("folder categories and unknown ids have no commands")
	}
}

func TestFoldersOfAFolderCategory(t *testing.T) {
	env := fakeEnv(t)
	if len(storage.Folders("temp-user", env)) != 1 {
		t.Errorf("%v", storage.Folders("temp-user", env))
	}
	if storage.Folders("docker-dangling", env) != nil || storage.Folders("nope", env) != nil {
		t.Error("command categories and unknown ids have no folders")
	}
}

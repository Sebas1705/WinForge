package storage_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Sebas1705/WinForge/internal/storage"
)

func TestDeveloperCachesAreScannedAndCleanedButNothingElseInTheirTools(t *testing.T) {
	e := fakeEnv(t)
	home := e.Home
	// what is cleaned
	put(t, filepath.Join(home, ".cargo", "registry", "cache", "index.crates.io-1", "serde-1.0.crate"), 4000, 0)
	put(t, filepath.Join(home, ".cargo", "registry", "src", "index.crates.io-1", "serde-1.0", "lib.rs"), 1000, 0)
	put(t, filepath.Join(home, ".gradle", "wrapper", "dists", "gradle-8.5-bin", "abc", "gradle.zip"), 6000, 0)
	put(t, filepath.Join(home, ".android", "cache", "sdk-meta.xml"), 500, 0)
	put(t, filepath.Join(e.LocalAppData, "Yarn", "Cache", "v6", "pkg.tgz"), 700, 0)
	put(t, filepath.Join(e.LocalAppData, "pnpm", "store", "v3", "files", "00", "ab"), 900, 0)
	// what must stay: credentials, binaries, the package index, virtual devices, the tool itself
	keep := []string{
		filepath.Join(home, ".cargo", "bin", "cargo.exe"),
		filepath.Join(home, ".cargo", "credentials.toml"),
		filepath.Join(home, ".cargo", "registry", "index", "index.crates.io-1", "config.json"),
		filepath.Join(home, ".gradle", "gradle.properties"),
		filepath.Join(home, ".android", "avd", "Pixel.avd", "userdata.img"),
		filepath.Join(home, ".android", "adbkey"),
		filepath.Join(e.LocalAppData, "pnpm", "pnpm.exe"),
		filepath.Join(e.LocalAppData, "Yarn", "Data", "global", "package.json"),
	}
	for _, k := range keep {
		put(t, k, 10, 0)
	}

	cats, err := storage.Scan(context.Background(), opts(e))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"dev-cargo": 5000, "dev-gradle-wrapper": 6000, "dev-android": 500, "dev-yarn": 700, "dev-pnpm": 900}
	for id, bytes := range want {
		c := category(t, cats, id)
		if !c.Available || c.Bytes != bytes {
			t.Errorf("%s: %+v, want %d bytes", id, c, bytes)
		}
	}
	for id, risk := range map[string]storage.Risk{"dev-yarn": storage.Safe, "dev-android": storage.Safe, "dev-cargo": storage.Review, "dev-pnpm": storage.Review, "dev-gradle-wrapper": storage.Review} {
		if got := category(t, cats, id).Risk; got != risk {
			t.Errorf("%s risk %s, want %s", id, got, risk)
		}
	}

	res := storage.Clean(context.Background(), []string{"dev-cargo", "dev-gradle-wrapper", "dev-android", "dev-yarn", "dev-pnpm"}, opts(e))
	if res.FreedBytes != 5000+6000+500+700+900 {
		t.Fatalf("%+v", res)
	}
	for _, k := range keep {
		if !exists(k) {
			t.Errorf("%s must not be removed", k)
		}
	}
	for _, root := range []string{
		filepath.Join(home, ".cargo", "registry", "cache"), filepath.Join(home, ".gradle", "wrapper", "dists"),
		filepath.Join(home, ".android", "cache"), filepath.Join(e.LocalAppData, "Yarn", "Cache"), filepath.Join(e.LocalAppData, "pnpm", "store"),
	} {
		if !exists(root) {
			t.Errorf("the folder %s itself stays", root)
		}
	}
}

func TestToolsThatAreNotInstalledAreNotOffered(t *testing.T) {
	cats, err := storage.Scan(context.Background(), opts(fakeEnv(t)))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"dev-cargo", "dev-pnpm", "dev-yarn", "dev-gradle-wrapper", "dev-android"} {
		if c := category(t, cats, id); c.Available || c.Bytes != 0 {
			t.Errorf("%s: %+v", id, c)
		}
	}
}

// docker pretends to be the Docker CLI: it keeps a build cache and some dangling images, and prunes them.
type docker struct {
	mu            sync.Mutex
	calls         []string
	cacheBytes    string
	danglingSizes []string
	down          bool
	pruneErr      error
}

func (d *docker) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	cmd := name + " " + strings.Join(args, " ")
	d.calls = append(d.calls, cmd)
	if name != "docker" || d.down {
		return nil, errors.New("Cannot connect to the Docker daemon")
	}
	switch cmd {
	case `docker system df --format {{json .}}`:
		return []byte(`{"Type":"Images","Reclaimable":"9GB (80%)","TotalCount":"40"}` + "\n" +
			`{"Type":"Build Cache","Reclaimable":"` + d.cacheBytes + ` (100%)","TotalCount":"12"}` + "\n" +
			`{"Type":"Local Volumes","Reclaimable":"7GB (90%)","TotalCount":"3"}` + "\n"), nil
	case `docker images --filter dangling=true --format {{.Size}}`:
		return []byte(strings.Join(d.danglingSizes, "\n")), nil
	case `docker builder prune --all --force`:
		if d.pruneErr != nil {
			return nil, d.pruneErr
		}
		d.cacheBytes = "0B"
		return []byte("Total:  3.4GB"), nil
	case `docker image prune --force`:
		if d.pruneErr != nil {
			return nil, d.pruneErr
		}
		d.danglingSizes = nil
		return []byte("Total reclaimed space: 1.4GB"), nil
	}
	return nil, errors.New("unexpected command: " + cmd)
}

func dockerOpts(t *testing.T, d *docker) storage.Options {
	o := opts(fakeEnv(t))
	o.Runner = d
	return o
}

func TestDockerCategoriesAreMeasuredFromDockerItself(t *testing.T) {
	d := &docker{cacheBytes: "3.4GB", danglingSizes: []string{"1.1GB", "300MB"}}
	cats, err := storage.Scan(context.Background(), dockerOpts(t, d))
	if err != nil {
		t.Fatal(err)
	}
	cache := category(t, cats, "docker-build-cache")
	if !cache.Available || cache.Bytes != 3_400_000_000 || cache.Risk != storage.Review || cache.NeedsAdmin {
		t.Fatalf("%+v", cache)
	}
	dang := category(t, cats, "docker-dangling")
	if !dang.Available || dang.Bytes != 1_400_000_000 || dang.Files != 2 {
		t.Fatalf("%+v", dang)
	}
}

func TestWithoutDockerRunningNothingIsOffered(t *testing.T) {
	d := &docker{down: true}
	cats, err := storage.Scan(context.Background(), dockerOpts(t, d))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"docker-build-cache", "docker-dangling"} {
		if c := category(t, cats, id); c.Available || c.Bytes != 0 {
			t.Errorf("%s: %+v", id, c)
		}
	}
}

func TestCleaningDockerRunsOnlyThePrunesMeantForIt(t *testing.T) {
	d := &docker{cacheBytes: "3.4GB", danglingSizes: []string{"1.1GB", "300MB"}}
	res := storage.Clean(context.Background(), []string{"docker-build-cache", "docker-dangling"}, dockerOpts(t, d))
	if res.FreedBytes != 3_400_000_000+1_400_000_000 || res.Failed != 0 {
		t.Fatalf("freed is measured before and after, not taken from the command: %+v", res)
	}
	var prunes []string
	for _, c := range d.calls {
		if strings.Contains(c, "prune") {
			prunes = append(prunes, c)
		}
	}
	want := []string{"docker builder prune --all --force", "docker image prune --force"}
	if strings.Join(prunes, "|") != strings.Join(want, "|") {
		t.Fatalf("prune commands: %v", prunes)
	}
	for _, c := range d.calls {
		for _, never := range []string{"system prune", "container prune", "volume prune", " rm ", " rmi ", " kill ", " stop "} {
			if strings.Contains(c, never) {
				t.Fatalf("containers and volumes are never touched: %s", c)
			}
		}
	}
}

func TestAFailedDockerPruneIsReportedAndFreesNothing(t *testing.T) {
	d := &docker{cacheBytes: "3.4GB", pruneErr: errors.New("error during connect: access denied")}
	res := storage.Clean(context.Background(), []string{"docker-build-cache"}, dockerOpts(t, d))
	if len(res.Outcomes) != 1 || res.Outcomes[0].Error == "" || res.FreedBytes != 0 {
		t.Fatalf("%+v", res)
	}
}

func TestDockerNeedsAWayToRunCommands(t *testing.T) {
	o := opts(fakeEnv(t)) // no runner
	res := storage.Clean(context.Background(), []string{"docker-build-cache", "docker-dangling"}, o)
	for _, out := range res.Outcomes {
		if out.Error == "" {
			t.Errorf("%s: %+v", out.ID, out)
		}
	}
}

package storage

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestParseSizeReadsDockerUnits(t *testing.T) {
	for in, want := range map[string]int64{
		"0B": 0, "12B": 12, "1kB": 1000, "12.5kB": 12500, "300MB": 300_000_000, "1.2GB": 1_200_000_000,
		"2TB": 2_000_000_000_000, " 5MB ": 5_000_000, "1.5gb": 1_500_000_000, ".5GB": 500_000_000,
	} {
		got, ok := parseSize(in)
		if !ok || got != want {
			t.Errorf("parseSize(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "GB", "12", "1.2.3GB", "1.2 GB", "1.2GiB", "-3GB", "abc", "45%", "(100%)", "1e3GB"} {
		if n, ok := parseSize(bad); ok {
			t.Errorf("parseSize(%q) = %d, true; must not be a size", bad, n)
		}
	}
}

// scripted answers each command from a table keyed by its words, and records them.
type scripted struct {
	mu      sync.Mutex
	calls   []string
	answers map[string]func() ([]byte, error)
}

func (s *scripted) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name + " " + strings.Join(args, " ")
	s.mu.Lock()
	s.calls = append(s.calls, key)
	f := s.answers[key]
	s.mu.Unlock()
	if f == nil {
		return nil, errors.New("not scripted: " + key)
	}
	return f()
}

const dfKey = `docker system df --format {{json .}}`
const danglingKey = `docker images --filter dangling=true --format {{.Size}}`

func df(buildCache string) string {
	return `{"Active":"1","Reclaimable":"1.2GB (45%)","Size":"2.7GB","TotalCount":"9","Type":"Images"}` + "\n" +
		`{"Active":"0","Reclaimable":"0B","Size":"0B","TotalCount":"0","Type":"Containers"}` + "\n" +
		buildCache + "\n" +
		`{"Active":"0","Reclaimable":"0B","Size":"0B","TotalCount":"0","Type":"Local Volumes"}` + "\n"
}

func TestTheBuildCacheIsTheReclaimableSizeNotTheTotal(t *testing.T) {
	r := &scripted{answers: map[string]func() ([]byte, error){
		dfKey: func() ([]byte, error) {
			return []byte(df(`{"Active":"2","Reclaimable":"3.4GB (70%)","Size":"4.9GB","TotalCount":"34","Type":"Build Cache"}`)), nil
		},
	}}
	size, n, ok := dockerBuildCache(context.Background(), r)
	if !ok || size != 3_400_000_000 || n != 34 {
		t.Fatalf("%d %d %v", size, n, ok)
	}
}

func TestDockerThatIsNotThereOffersNothing(t *testing.T) {
	r := &scripted{answers: map[string]func() ([]byte, error){
		dfKey:       func() ([]byte, error) { return nil, errors.New(`exec: "docker": executable file not found`) },
		danglingKey: func() ([]byte, error) { return nil, errors.New("Cannot connect to the Docker daemon") },
	}}
	if _, _, ok := dockerBuildCache(context.Background(), r); ok {
		t.Fatal("no Docker, no category")
	}
	if _, _, ok := dockerDanglingImages(context.Background(), r); ok {
		t.Fatal("a stopped engine, no category")
	}
	if _, _, ok := dockerBuildCache(context.Background(), nil); ok {
		t.Fatal("no way to run commands, no category")
	}
}

func TestAnswersThatAreNotWhatWasExpectedNeverCountAsSpace(t *testing.T) {
	for name, out := range map[string]string{
		"empty":         "",
		"no cache row":  `{"Type":"Images","Reclaimable":"1GB (1%)","TotalCount":"1"}`,
		"not json":      "this is not json\nneither is this",
		"not a size":    `{"Type":"Build Cache","Reclaimable":"lots","TotalCount":"3"}`,
		"blank lines":   "\n\n",
		"bad row first": "oops\n" + `{"Type":"Build Cache","Reclaimable":"0B","TotalCount":"0"}`,
	} {
		r := &scripted{answers: map[string]func() ([]byte, error){dfKey: func() ([]byte, error) { return []byte(out), nil }}}
		size, _, ok := dockerBuildCache(context.Background(), r)
		if size != 0 {
			t.Errorf("%s: %d bytes from nothing", name, size)
		}
		if name == "bad row first" && !ok {
			t.Errorf("%s: the engine answered", name)
		}
	}
}

func TestDanglingImagesAreSummed(t *testing.T) {
	r := &scripted{answers: map[string]func() ([]byte, error){
		danglingKey: func() ([]byte, error) { return []byte("1.1GB\n300MB\n\n12.5kB\nnot-a-size\n"), nil },
	}}
	size, n, ok := dockerDanglingImages(context.Background(), r)
	if !ok || n != 3 || size != 1_100_000_000+300_000_000+12_500 {
		t.Fatalf("%d %d %v", size, n, ok)
	}
}

package privacy

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// fakeSys is an in-memory registry: no real registry, no real app-data folder.
type fakeSys struct {
	mu         sync.Mutex
	vals       map[string]Value // key: lower(path)|lower(name)
	keys       map[string]bool  // lower(path) of every key that exists
	backupDir  string
	writes     []string
	broadcasts int
	failSet    map[string]error // by value name
	failGet    map[string]error
}

func newFake(t *testing.T) *fakeSys {
	t.Helper()
	return &fakeSys{vals: map[string]Value{}, keys: map[string]bool{}, backupDir: t.TempDir(), failSet: map[string]error{}, failGet: map[string]error{}}
}

func vkey(path, name string) string { return strings.ToLower(path) + "|" + strings.ToLower(name) }

func (f *fakeSys) addKey(path string) {
	for p := path; p != ""; {
		f.keys[strings.ToLower(p)] = true
		i := strings.LastIndex(p, `\`)
		if i < 0 {
			break
		}
		p = p[:i]
	}
}

// putDWord and put preload a value (and its keys).
func (f *fakeSys) putDWord(path, name string, v uint32) { f.put(path, name, TypeDWord, dword(v)) }
func (f *fakeSys) put(path, name string, typ uint32, data []byte) {
	f.addKey(path)
	f.vals[vkey(path, name)] = Value{Exists: true, Type: typ, Data: data}
}
func (f *fakeSys) str(path, name, s string) {
	var b []byte
	for _, r := range s + "\x00" {
		b = append(b, byte(r), byte(r>>8))
	}
	f.put(path, name, TypeString, b)
}

func (f *fakeSys) Get(path, name string) (Value, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failGet[name]; err != nil {
		return Value{}, err
	}
	v := f.vals[vkey(path, name)]
	v.Data = append([]byte(nil), v.Data...)
	return v, nil
}
func (f *fakeSys) KeyExists(path string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.keys[strings.ToLower(path)], nil
}
func (f *fakeSys) Set(path, name string, typ uint32, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failSet[name]; err != nil {
		return err
	}
	f.addKey(path)
	f.vals[vkey(path, name)] = Value{Exists: true, Type: typ, Data: append([]byte(nil), data...)}
	f.writes = append(f.writes, "set "+name)
	return nil
}
func (f *fakeSys) Delete(path, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failSet[name]; err != nil {
		return err
	}
	delete(f.vals, vkey(path, name))
	f.writes = append(f.writes, "del "+name)
	return nil
}
func (f *fakeSys) DeleteKeyIfEmpty(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	lp := strings.ToLower(path)
	for k := range f.vals {
		if strings.HasPrefix(k, lp+"|") {
			return nil
		}
	}
	for k := range f.keys {
		if strings.HasPrefix(k, lp+`\`) {
			return nil
		}
	}
	delete(f.keys, lp)
	f.writes = append(f.writes, "delkey "+path)
	return nil
}
func (f *fakeSys) Broadcast() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broadcasts++
	return nil
}
func (f *fakeSys) BackupDir() (string, error) { return f.backupDir, nil }

func (f *fakeSys) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}

var errBoom = errors.New("registry refused")

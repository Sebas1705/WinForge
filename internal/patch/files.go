package patch

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ApplyFiles patches the game at romPath with the patch at patchPath and writes
// the result next to the game as "<name> (patched)<ext>". The original is never
// touched and an existing file is never overwritten: a number is added instead.
func ApplyFiles(romPath, patchPath string) (string, Info, error) {
	rom, err := readLimited(romPath)
	if err != nil {
		return "", Info{}, err
	}
	if bytes.HasPrefix(rom, []byte("PK\x03\x04")) {
		return "", Info{}, errors.New("this game is in a compressed file: extract it first, then choose the extracted game")
	}
	p, err := readLimited(patchPath)
	if err != nil {
		return "", Info{}, err
	}
	out, info, err := Apply(rom, p)
	if err != nil {
		return "", info, err
	}
	dir, base := filepath.Split(romPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	target := filepath.Join(dir, name+" (patched)"+ext)
	for n := 2; ; n++ {
		f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_, werr := f.Write(out)
			if cerr := f.Close(); werr == nil {
				werr = cerr
			}
			if werr != nil {
				os.Remove(target)
				return "", info, werr
			}
			return target, info, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", info, err
		}
		target = filepath.Join(dir, fmt.Sprintf("%s (patched %d)%s", name, n, ext))
	}
}

func readLimited(path string) ([]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a file")
	}
	if st.Size() > MaxSize {
		return nil, errors.New("file too large")
	}
	return os.ReadFile(path)
}

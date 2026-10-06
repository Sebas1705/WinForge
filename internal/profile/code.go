package profile

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"github.com/Sebas1705/WinForge/internal/catalog"
)

// CodePrefix marks a share code and its format version.
const CodePrefix = "WF1."

// maxCode bounds both the pasted text and what it expands to, so a pasted
// "zip bomb" cannot exhaust memory.
const maxCode = 1 << 20

// EncodeCode packs a profile into one line of text that can be pasted into a
// message and imported on another PC: the same JSON as an exported file,
// compressed and made URL-safe. It carries no more than the file does.
func EncodeCode(p catalog.Profile) (string, error) {
	raw, err := Export(p)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return CodePrefix + base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

// DecodeCode reads a share code and validates it like an imported file.
func DecodeCode(code string, cat *catalog.Catalog) (*Imported, error) {
	code = strings.Join(strings.Fields(code), "") // pasted codes get wrapped and padded
	if len(code) > maxCode {
		return nil, errors.New("that code is too long to be a profile")
	}
	if !strings.HasPrefix(code, CodePrefix) {
		return nil, errors.New("not a WinForge share code")
	}
	packed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, CodePrefix))
	if err != nil {
		return nil, errors.New("not a WinForge share code: it looks cut off or altered")
	}
	zr, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		return nil, errors.New("not a WinForge share code: it looks cut off or altered")
	}
	raw, err := io.ReadAll(io.LimitReader(zr, maxCode+1))
	if err != nil || len(raw) > maxCode {
		return nil, errors.New("not a WinForge share code: it looks cut off or altered")
	}
	return Import(raw, cat)
}

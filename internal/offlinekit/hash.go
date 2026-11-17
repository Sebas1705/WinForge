package offlinekit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
)

// ctxReader stops a copy when the context is cancelled, so a large file does not hold up a cancel.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// hashFile reads a file once and returns its size and SHA-256.
func hashFile(ctx context.Context, fs FS, path string) (size int64, sum string, err error) {
	f, err := fs.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, ctxReader{ctx, f})
	if err != nil {
		return n, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// copyHashed copies src to dst (which must not exist) and returns the size and SHA-256 of what was written.
func copyHashed(ctx context.Context, fs FS, src, dst string) (size int64, sum string, err error) {
	in, err := fs.Open(src)
	if err != nil {
		return 0, "", err
	}
	defer in.Close()
	out, err := fs.Create(dst)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), ctxReader{ctx, in})
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

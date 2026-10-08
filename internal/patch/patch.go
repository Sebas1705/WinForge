// Package patch applies ROM-hack patches (IPS, UPS and BPS) to a game file the
// person already owns. A patch holds only the differences from the original, so
// it is how hacks, translations and randomizers are shared without the game
// itself. Nothing here fetches anything: bytes in, bytes out.
package patch

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
)

// MaxSize bounds the files read, so a damaged or hostile patch cannot ask for
// gigabytes of memory.
const MaxSize = 256 << 20

// Info says what was applied.
type Info struct {
	Format string `json:"format"` // IPS, UPS or BPS
	// Checked is true when the patch carries a checksum of the original game
	// (UPS and BPS do, IPS does not) and it matched.
	Checked bool `json:"checked"`
	// Headerless is true when a 512-byte copier header was removed from the
	// original first, because that is what the patch was made against.
	Headerless bool `json:"headerless"`
}

// Detect names the patch format from its first bytes, or "".
func Detect(p []byte) string {
	switch {
	case bytes.HasPrefix(p, []byte("PATCH")):
		return "IPS"
	case bytes.HasPrefix(p, []byte("UPS1")):
		return "UPS"
	case bytes.HasPrefix(p, []byte("BPS1")):
		return "BPS"
	}
	return ""
}

// ErrWrongGame means the patch was made for a different version of the game.
type ErrWrongGame struct{ Want, Got uint32 }

func (e *ErrWrongGame) Error() string {
	return fmt.Sprintf("this patch is for a different version of the game (it expects checksum %08x, your file is %08x)", e.Want, e.Got)
}

// Apply returns the patched game. It never changes its inputs.
func Apply(rom, p []byte) ([]byte, Info, error) {
	if len(rom) > MaxSize || len(p) > MaxSize {
		return nil, Info{}, errors.New("file too large")
	}
	switch Detect(p) {
	case "IPS":
		out, err := applyIPS(rom, p)
		return out, Info{Format: "IPS"}, err
	case "UPS":
		return withHeaderRetry(rom, p, "UPS", applyUPS)
	case "BPS":
		return withHeaderRetry(rom, p, "BPS", applyBPS)
	}
	return nil, Info{}, errors.New("not an IPS, UPS or BPS patch")
}

// withHeaderRetry applies a checksummed patch; when the checksum does not match
// and the file looks like it has a 512-byte copier header (common on old SNES
// dumps), it tries again without the header.
func withHeaderRetry(rom, p []byte, format string, fn func(rom, p []byte) ([]byte, error)) ([]byte, Info, error) {
	out, err := fn(rom, p)
	if err == nil {
		return out, Info{Format: format, Checked: true}, nil
	}
	var wrong *ErrWrongGame
	if errors.As(err, &wrong) && len(rom)%1024 == 512 {
		if out2, err2 := fn(rom[512:], p); err2 == nil {
			return out2, Info{Format: format, Checked: true, Headerless: true}, nil
		}
	}
	return nil, Info{Format: format}, err
}

// ---- IPS: records of (offset, length, bytes), or RLE when length is 0.

func applyIPS(rom, p []byte) ([]byte, error) {
	out := append([]byte(nil), rom...)
	i := 5
	need := func(n int) error {
		if i+n > len(p) {
			return errors.New("the IPS patch is cut short")
		}
		return nil
	}
	for {
		if i+3 <= len(p) && bytes.Equal(p[i:i+3], []byte("EOF")) {
			i += 3
			break
		}
		if err := need(5); err != nil {
			return nil, err
		}
		off := int(p[i])<<16 | int(p[i+1])<<8 | int(p[i+2])
		size := int(p[i+3])<<8 | int(p[i+4])
		i += 5
		if size == 0 { // RLE: a 2-byte run length and the byte to repeat
			if err := need(3); err != nil {
				return nil, err
			}
			run := int(p[i])<<8 | int(p[i+1])
			val := p[i+2]
			i += 3
			out = grow(out, off+run)
			for k := 0; k < run; k++ {
				out[off+k] = val
			}
			continue
		}
		if err := need(size); err != nil {
			return nil, err
		}
		out = grow(out, off+size)
		copy(out[off:], p[i:i+size])
		i += size
	}
	// An optional 3-byte length after EOF truncates the result.
	if i+3 <= len(p) {
		if n := int(p[i])<<16 | int(p[i+1])<<8 | int(p[i+2]); n < len(out) {
			out = out[:n]
		}
	}
	return out, nil
}

func grow(b []byte, n int) []byte {
	if n > MaxSize {
		return b[:0:0]
	}
	for len(b) < n {
		b = append(b, 0)
	}
	return b
}

// ---- UPS and BPS share a variable-length integer.

type reader struct {
	b []byte
	i int
}

func (r *reader) byte() (byte, error) {
	if r.i >= len(r.b) {
		return 0, errors.New("the patch is cut short")
	}
	c := r.b[r.i]
	r.i++
	return c, nil
}

// number reads the variable-length integer both formats use.
func (r *reader) number() (uint64, error) {
	var data, shift uint64 = 0, 1
	for {
		c, err := r.byte()
		if err != nil {
			return 0, err
		}
		data += uint64(c&0x7f) * shift
		if c&0x80 != 0 {
			return data, nil
		}
		shift <<= 7
		data += shift
		if shift > 1<<56 {
			return 0, errors.New("the patch is damaged")
		}
	}
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// checkFooter verifies the patch's own checksum (its last 4 bytes) and returns
// the source and target checksums stored before it.
func checkFooter(p []byte) (src, dst uint32, body []byte, err error) {
	if len(p) < 12+4 {
		return 0, 0, nil, errors.New("the patch is cut short")
	}
	if crc32.ChecksumIEEE(p[:len(p)-4]) != le32(p[len(p)-4:]) {
		return 0, 0, nil, errors.New("the patch file is damaged (its checksum does not match)")
	}
	return le32(p[len(p)-12:]), le32(p[len(p)-8:]), p[:len(p)-12], nil
}

// ---- UPS: XOR of the original with the new bytes.

func applyUPS(rom, p []byte) ([]byte, error) {
	srcCRC, dstCRC, body, err := checkFooter(p)
	if err != nil {
		return nil, err
	}
	if got := crc32.ChecksumIEEE(rom); got != srcCRC {
		return nil, &ErrWrongGame{Want: srcCRC, Got: got}
	}
	r := &reader{b: body, i: 4}
	srcSize, err := r.number()
	if err != nil {
		return nil, err
	}
	dstSize, err := r.number()
	if err != nil {
		return nil, err
	}
	if srcSize != uint64(len(rom)) && dstSize != uint64(len(rom)) {
		return nil, &ErrWrongGame{Want: srcCRC, Got: crc32.ChecksumIEEE(rom)}
	}
	if dstSize > MaxSize {
		return nil, errors.New("the patched file would be too large")
	}
	out := make([]byte, dstSize)
	copy(out, rom)
	pos := uint64(0)
	for r.i < len(body) {
		skip, err := r.number()
		if err != nil {
			return nil, err
		}
		pos += skip
		for {
			c, err := r.byte()
			if err != nil {
				return nil, err
			}
			if pos < dstSize {
				out[pos] ^= c
			}
			pos++
			if c == 0 {
				break
			}
		}
	}
	if got := crc32.ChecksumIEEE(out); got != dstCRC {
		return nil, fmt.Errorf("the patched file does not match what the patch expects (%08x, wanted %08x)", got, dstCRC)
	}
	return out, nil
}

// ---- BPS: copy and read actions.

func applyBPS(rom, p []byte) ([]byte, error) {
	srcCRC, dstCRC, body, err := checkFooter(p)
	if err != nil {
		return nil, err
	}
	if got := crc32.ChecksumIEEE(rom); got != srcCRC {
		return nil, &ErrWrongGame{Want: srcCRC, Got: got}
	}
	r := &reader{b: body, i: 4}
	srcSize, err := r.number()
	if err != nil {
		return nil, err
	}
	dstSize, err := r.number()
	if err != nil {
		return nil, err
	}
	metaSize, err := r.number()
	if err != nil {
		return nil, err
	}
	if srcSize != uint64(len(rom)) || dstSize > MaxSize || metaSize > uint64(len(body)) {
		return nil, errors.New("the patch is not for this file (the size differs)")
	}
	r.i += int(metaSize)
	out := make([]byte, 0, dstSize)
	var srcRel, dstRel int64
	for r.i < len(body) && uint64(len(out)) < dstSize {
		data, err := r.number()
		if err != nil {
			return nil, err
		}
		length := int(data>>2) + 1
		if uint64(len(out)+length) > dstSize {
			return nil, errors.New("the patch is damaged (it writes past the end)")
		}
		switch data & 3 {
		case 0: // SourceRead: the same bytes as the original at this position
			start := len(out)
			if start+length > len(rom) {
				return nil, errors.New("the patch is damaged (it reads past the original)")
			}
			out = append(out, rom[start:start+length]...)
		case 1: // TargetRead: new bytes stored in the patch
			if r.i+length > len(body) {
				return nil, errors.New("the patch is cut short")
			}
			out = append(out, body[r.i:r.i+length]...)
			r.i += length
		case 2, 3: // SourceCopy / TargetCopy: a relative offset, sign in the lowest bit
			off, err := r.number()
			if err != nil {
				return nil, err
			}
			delta := int64(off >> 1)
			if off&1 != 0 {
				delta = -delta
			}
			if data&3 == 2 {
				srcRel += delta
				if srcRel < 0 || int(srcRel)+length > len(rom) {
					return nil, errors.New("the patch is damaged (a copy is out of range)")
				}
				out = append(out, rom[srcRel:int(srcRel)+length]...)
				srcRel += int64(length)
			} else {
				dstRel += delta
				if dstRel < 0 || int(dstRel) >= len(out) && length > 0 {
					return nil, errors.New("the patch is damaged (a copy is out of range)")
				}
				for k := 0; k < length; k++ { // may overlap what it is writing, so byte by byte
					out = append(out, out[dstRel])
					dstRel++
				}
			}
		}
	}
	if uint64(len(out)) != dstSize {
		return nil, errors.New("the patch is damaged (the result has the wrong size)")
	}
	if got := crc32.ChecksumIEEE(out); got != dstCRC {
		return nil, fmt.Errorf("the patched file does not match what the patch expects (%08x, wanted %08x)", got, dstCRC)
	}
	return out, nil
}

package patch_test

import (
	"bytes"
	"errors"
	"hash/crc32"
	"testing"

	"github.com/Sebas1705/WinForge/internal/patch"
)

func crc(b []byte) uint32 { return crc32.ChecksumIEEE(b) }

func le(n uint32) []byte { return []byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)} }

// number is the variable-length integer UPS and BPS use.
func number(n uint64) []byte {
	var out []byte
	for {
		x := byte(n & 0x7f)
		n >>= 7
		if n == 0 {
			return append(out, 0x80|x)
		}
		out = append(out, x)
		n--
	}
}

func seal(body []byte, src, dst []byte) []byte {
	p := append(append([]byte(nil), body...), le(crc(src))...)
	p = append(p, le(crc(dst))...)
	return append(p, le(crc(p))...)
}

func game(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + i/13)
	}
	return b
}

func TestIPSRecordsRLEAndTruncation(t *testing.T) {
	rom := game(64)
	want := append([]byte(nil), rom...)
	p := []byte("PATCH")
	// record: change 3 bytes at offset 4
	p = append(p, 0, 0, 4, 0, 3, 0xAA, 0xBB, 0xCC)
	copy(want[4:], []byte{0xAA, 0xBB, 0xCC})
	// RLE: 10 bytes of 0x11 at offset 20
	p = append(p, 0, 0, 20, 0, 0, 0, 10, 0x11)
	for i := 0; i < 10; i++ {
		want[20+i] = 0x11
	}
	// a record past the end grows the file
	p = append(p, 0, 0, 70, 0, 2, 0xDD, 0xEE)
	want = append(want, make([]byte, 6)...)
	want = append(want, 0xDD, 0xEE)
	p = append(p, "EOF"...)
	got, info, err := patch.Apply(rom, p)
	if err != nil || !bytes.Equal(got, want) || info.Format != "IPS" || info.Checked {
		t.Fatalf("%v %+v equal=%v", err, info, bytes.Equal(got, want))
	}
	// A length after EOF truncates.
	p = append(p, 0, 0, 40)
	got, _, err = patch.Apply(rom, p)
	if err != nil || len(got) != 40 {
		t.Fatalf("truncate: %v %d", err, len(got))
	}
	if !bytes.Equal(rom, game(64)) {
		t.Fatal("Apply changed its input")
	}
}

func TestUPSRoundTripAndWrongGame(t *testing.T) {
	rom := game(300)
	dst := append([]byte(nil), rom...)
	dst[10] ^= 0xFF
	dst[200] ^= 0x0F
	dst = append(dst, 1, 2, 3)
	body := []byte("UPS1")
	body = append(body, number(uint64(len(rom)))...)
	body = append(body, number(uint64(len(dst)))...)
	// differences: skip 10, xor FF, 0 ; then 188 more (the terminator also moves the position), xor 0F, 0
	body = append(body, number(10)...)
	body = append(body, 0xFF, 0)
	body = append(body, number(188)...)
	body = append(body, 0x0F, 0)
	body = append(body, number(98)...) // reach the appended bytes (offset 300)
	body = append(body, 1, 2, 3, 0)
	p := seal(body, rom, dst)

	got, info, err := patch.Apply(rom, p)
	if err != nil || !bytes.Equal(got, dst) || !info.Checked || info.Format != "UPS" {
		t.Fatalf("%v %+v", err, info)
	}
	other := game(300)
	other[0] ^= 1
	_, _, err = patch.Apply(other, p)
	var wrong *patch.ErrWrongGame
	if !errors.As(err, &wrong) || wrong.Want != crc(rom) {
		t.Fatalf("a different game must be refused with a clear error: %v", err)
	}
	damaged := append([]byte(nil), p...)
	damaged[8] ^= 1
	if _, _, err := patch.Apply(rom, damaged); err == nil {
		t.Fatal("a damaged patch must be refused")
	}
}

func bpsPatch(rom, dst []byte, actions []byte) []byte {
	body := []byte("BPS1")
	body = append(body, number(uint64(len(rom)))...)
	body = append(body, number(uint64(len(dst)))...)
	body = append(body, number(0)...)
	body = append(body, actions...)
	return seal(body, rom, dst)
}

func TestBPSAllFourActions(t *testing.T) {
	rom := game(100)
	var dst, act []byte
	// SourceRead 10 bytes (same position)
	dst = append(dst, rom[:10]...)
	act = append(act, number(uint64((10-1)<<2|0))...)
	// TargetRead "HELLO"
	dst = append(dst, "HELLO"...)
	act = append(act, number(uint64((5-1)<<2|1))...)
	act = append(act, "HELLO"...)
	// SourceCopy 8 bytes from rom[50:58]  (relative offset +50)
	dst = append(dst, rom[50:58]...)
	act = append(act, number(uint64((8-1)<<2|2))...)
	act = append(act, number(uint64(50<<1))...)
	// TargetCopy 6 bytes starting at dst[10:] (offset 10 from 0) - "HELLO" + 1 byte
	start := 10
	for i := 0; i < 6; i++ {
		dst = append(dst, dst[start+i])
	}
	act = append(act, number(uint64((6-1)<<2|3))...)
	act = append(act, number(uint64(10<<1))...)
	// TargetCopy overlapping its own output: relative -? copy 4 bytes starting at the last byte written (run)
	last := len(dst) - 1
	// dstRel is now 16 after the previous copy; move back to `last`: delta = last-16 (negative or zero)
	delta := last - 16
	var enc uint64
	if delta < 0 {
		enc = uint64(-delta)<<1 | 1
	} else {
		enc = uint64(delta) << 1
	}
	for i := 0; i < 4; i++ {
		dst = append(dst, dst[last])
	}
	act = append(act, number(uint64((4-1)<<2|3))...)
	act = append(act, number(enc)...)

	p := bpsPatch(rom, dst, act)
	got, info, err := patch.Apply(rom, p)
	if err != nil || !bytes.Equal(got, dst) || !info.Checked || info.Format != "BPS" {
		t.Fatalf("%v %+v\n got  %x\n want %x", err, info, got, dst)
	}
}

func TestSNESCopierHeaderIsHandled(t *testing.T) {
	rom := game(2048)
	dst := append([]byte(nil), rom...)
	dst[100] ^= 0xFF
	body := []byte("UPS1")
	body = append(body, number(uint64(len(rom)))...)
	body = append(body, number(uint64(len(dst)))...)
	body = append(body, number(100)...)
	body = append(body, 0xFF, 0)
	p := seal(body, rom, dst)

	withHeader := append(make([]byte, 512), rom...)
	got, info, err := patch.Apply(withHeader, p)
	if err != nil || !info.Headerless || !bytes.Equal(got, dst) {
		t.Fatalf("%v %+v", err, info)
	}
}

func TestBadAndTruncatedPatchesNeverPanic(t *testing.T) {
	rom := game(100)
	if _, _, err := patch.Apply(rom, []byte("hello")); err == nil {
		t.Fatal("not a patch")
	}
	// Every prefix of valid patches must fail cleanly, never crash.
	ips := append([]byte("PATCH"), 0, 0, 4, 0, 3, 1, 2, 3, 0, 0, 20, 0, 0, 0, 10, 9)
	ips = append(ips, "EOF"...)
	body := []byte("BPS1")
	body = append(body, number(100)...)
	body = append(body, number(100)...)
	body = append(body, number(0)...)
	body = append(body, number(uint64((100-1)<<2|0))...)
	bps := seal(body, rom, rom)
	for _, full := range [][]byte{ips, bps} {
		for n := 0; n < len(full); n++ {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic on a patch cut at %d bytes: %v", n, r)
					}
				}()
				_, _, _ = patch.Apply(rom, full[:n])
			}()
		}
	}
	if got, _, err := patch.Apply(rom, bps); err != nil || !bytes.Equal(got, rom) {
		t.Fatalf("identity BPS: %v", err)
	}
	if _, _, err := patch.Apply(make([]byte, patch.MaxSize+1), ips); err == nil {
		t.Fatal("oversized input must be refused")
	}
}

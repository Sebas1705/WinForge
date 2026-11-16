package startup

import (
	"encoding/binary"
	"time"
)

// Windows (and Task Manager) record whether a startup item is switched on in a REG_BINARY value of
// 12 bytes under ...\Explorer\StartupApproved\{Run,Run32,StartupFolder}. The first byte is even
// (02 or 06) while the item is enabled and odd (03 or 07) while it is disabled, and a disabled
// value carries a FILETIME of when it was switched off. No value at all means enabled.

const approvedLen = 12

// filetimeEpoch is the difference, in 100 ns ticks, between 1601-01-01 and 1970-01-01.
const filetimeEpoch = 116444736000000000

// DecodeApproved reports whether a StartupApproved value means "enabled". A missing value (nil or
// empty) is enabled. A blob too short to hold the flag (fewer than 4 bytes) is damaged: it is read
// as enabled, which is what Windows does when it cannot make sense of it, and a later switch
// rewrites it whole.
func DecodeApproved(b []byte) bool {
	if len(b) < 4 {
		return true
	}
	return b[0]&1 == 0
}

// EncodeApproved is the value to write: 02 00 00 00 and eight zeros to enable, 03 00 00 00 and the
// time to disable.
func EncodeApproved(enabled bool, now time.Time) []byte {
	b := make([]byte, approvedLen)
	if enabled {
		b[0] = 0x02
		return b
	}
	b[0] = 0x03
	ft := uint64(now.UTC().UnixNano()/100) + filetimeEpoch
	binary.LittleEndian.PutUint64(b[4:], ft)
	return b
}

// DisabledAt reads the time a disabled value was written. ok is false when there is none.
func DisabledAt(b []byte) (time.Time, bool) {
	if len(b) < approvedLen || DecodeApproved(b) {
		return time.Time{}, false
	}
	ft := binary.LittleEndian.Uint64(b[4:12])
	if ft < filetimeEpoch {
		return time.Time{}, false
	}
	return time.Unix(0, int64(ft-filetimeEpoch)*100).UTC(), true
}

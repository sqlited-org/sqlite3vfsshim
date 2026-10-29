// Package vfscore contains the pure-Go definitions shared by the vfs and
// vfsshim libraries: SQLite flag types and result codes, the Error type, the
// File interface and a few conversion helpers. It has no cgo and no SQLite
// dependency.
package vfscore

import (
	"errors"
	"fmt"
	"io"
	"time"
	"unsafe"
)

// ErrorCode is a SQLite result code (primary or extended).
type ErrorCode int

// Result codes commonly returned from VFS methods.
const (
	ErrOK              ErrorCode = 0
	ErrError           ErrorCode = 1
	ErrPerm            ErrorCode = 3
	ErrBusy            ErrorCode = 5
	ErrNoMem           ErrorCode = 7
	ErrReadOnly        ErrorCode = 8
	ErrIO              ErrorCode = 10
	ErrFull            ErrorCode = 13
	ErrCantOpen        ErrorCode = 14
	ErrNotFound        ErrorCode = 12
	ErrMisuse          ErrorCode = 21
	ErrIOShortRead     ErrorCode = 10 | (2 << 8)
	ErrIOWrite         ErrorCode = 10 | (3 << 8)
	ErrIOFsync         ErrorCode = 10 | (4 << 8)
	ErrIOLock          ErrorCode = 10 | (15 << 8)
	ErrIORead          ErrorCode = 10 | (1 << 8)
	ErrIOTruncate      ErrorCode = 10 | (6 << 8)
	ErrIOFstat         ErrorCode = 10 | (7 << 8)
	ErrIOCheckReserved ErrorCode = 10 | (14 << 8)
)

// Error is an error carrying a SQLite result code. Errors returned from Shim
// and File methods that are (or wrap) an *Error have their code handed back to
// SQLite verbatim; any other non-nil error becomes SQLITE_IOERR.
type Error struct {
	Code ErrorCode
}

func (e *Error) Error() string { return fmt.Sprintf("sqlite3 vfs error (code %d)", int(e.Code)) }

// ErrFromCode converts a SQLite result code to an error (nil for SQLITE_OK).
func ErrFromCode(rc int) error {
	if rc == 0 {
		return nil
	}
	return &Error{Code: ErrorCode(rc)}
}

// CodeFromErr maps a Go error to a SQLite result code: 0 for nil, the code of
// an (wrapped) *Error, otherwise SQLITE_IOERR.
func CodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	var e *Error
	if errors.As(err, &e) {
		return int(e.Code)
	}
	return int(ErrIO)
}

// OpenFlag are the SQLITE_OPEN_* flags passed to Open.
type OpenFlag int

const (
	OpenReadOnly      OpenFlag = 0x00000001
	OpenReadWrite     OpenFlag = 0x00000002
	OpenCreate        OpenFlag = 0x00000004
	OpenDeleteOnClose OpenFlag = 0x00000008
	OpenExclusive     OpenFlag = 0x00000010
	OpenURI           OpenFlag = 0x00000040
	OpenMainDB        OpenFlag = 0x00000100
	OpenTempDB        OpenFlag = 0x00000200
	OpenTransientDB   OpenFlag = 0x00000400
	OpenMainJournal   OpenFlag = 0x00000800
	OpenTempJournal   OpenFlag = 0x00001000
	OpenSubJournal    OpenFlag = 0x00002000
	OpenSuperJournal  OpenFlag = 0x00004000
	OpenWAL           OpenFlag = 0x00080000
)

// AccessFlag are the SQLITE_ACCESS_* flags passed to Access.
type AccessFlag int

const (
	AccessExists    AccessFlag = 0
	AccessReadWrite AccessFlag = 1
	AccessRead      AccessFlag = 2
)

// LockType is a SQLITE_LOCK_* level.
type LockType int

const (
	LockNone      LockType = 0
	LockShared    LockType = 1
	LockReserved  LockType = 2
	LockPending   LockType = 3
	LockExclusive LockType = 4
)

func (l LockType) String() string {
	switch l {
	case LockNone:
		return "NONE"
	case LockShared:
		return "SHARED"
	case LockReserved:
		return "RESERVED"
	case LockPending:
		return "PENDING"
	case LockExclusive:
		return "EXCLUSIVE"
	}
	return fmt.Sprintf("LockType(%d)", int(l))
}

// SyncType is the SQLITE_SYNC_* flag set passed to File.Sync.
type SyncType int

const (
	SyncNormal   SyncType = 0x00002
	SyncFull     SyncType = 0x00003
	SyncDataOnly SyncType = 0x00010
)

// DeviceCharacteristic is a SQLITE_IOCAP_* bit set.
type DeviceCharacteristic int

const (
	IOCapAtomic              DeviceCharacteristic = 0x00000001
	IOCapSafeAppend          DeviceCharacteristic = 0x00000200
	IOCapSequential          DeviceCharacteristic = 0x00000400
	IOCapUndeletableWhenOpen DeviceCharacteristic = 0x00000800
	IOCapPowersafeOverwrite  DeviceCharacteristic = 0x00001000
	IOCapImmutable           DeviceCharacteristic = 0x00002000
)

// VFS is the minimal set of operations every VFS must provide.
//
// Optionally also implement Randomizer, Sleeper, Clock and/or URIOpener; the
// defaults use the Go standard library.
type VFS interface {
	// Open opens the named file. name is "" for anonymous temporary files.
	// flags are the OpenFlag values SQLite asked for; the returned OpenFlag
	// reports what was actually used (usually flags unchanged).
	Open(name string, flags OpenFlag) (File, OpenFlag, error)

	// Delete removes the named file. SQLite uses it for journal, WAL and
	// temporary files. If dirSync is true the deletion must be durable before
	// returning. Deleting a missing file should return an *Error with code
	// ErrIO|(23<<8) (SQLITE_IOERR_DELETE_NOENT).
	Delete(name string, dirSync bool) error

	// Access reports whether name satisfies flags: AccessExists,
	// AccessReadWrite or AccessRead. A missing file is not an error: return
	// (false, nil).
	Access(name string, flags AccessFlag) (bool, error)

	// FullPathname returns the canonical absolute form of name. SQLite uses
	// the result as the file's identity, so the same file must always map to
	// the same string. The result must fit in the maximum path name length
	// (see WithMaxPathname).
	FullPathname(name string) (string, error)
}

// Randomizer can be implemented by a VFS to supply randomness to SQLite. The
// default is crypto/rand.
type Randomizer interface {
	// Randomness fills p and returns the number of bytes written.
	Randomness(p []byte) int
}

// Sleeper can be implemented by a VFS to customise sleeping (used by SQLite's
// busy handler). The default is time.Sleep.
type Sleeper interface {
	// Sleep sleeps for about d and returns the time actually slept.
	Sleep(d time.Duration) time.Duration
}

// Clock can be implemented by a VFS to control the time SQLite sees (date and
// time SQL functions such as datetime('now')). The default is time.Now.
type Clock interface {
	CurrentTime() (time.Time, error)
}

// File is an open database file. It is implemented by files returned from a
// VFS or shim, and (in vfsshim) by files obtained from the parent VFS.
type File interface {
	// Close closes the file. SQLite calls it once when the file is no longer
	// needed. A shim wrapper should close its parent file; if it does not, the
	// parent file is closed automatically afterwards.
	Close() error
	// ReadAt reads len(p) bytes at off. A short read must return io.EOF (or an
	// *Error with ErrIOShortRead); the rest of p is zero-filled.
	ReadAt(p []byte, off int64) (n int, err error)
	// WriteAt writes all of p at off, extending the file if needed. SQLite
	// expects the write to be complete: return an error (e.g. ErrFull) rather
	// than a partial write.
	WriteAt(p []byte, off int64) (n int, err error)
	// Truncate sets the file size to size bytes.
	Truncate(size int64) error
	// Sync flushes file content to stable storage. flags tells how thorough
	// the sync must be (SyncNormal, SyncFull, optionally with SyncDataOnly).
	Sync(flags SyncType) error
	// FileSize returns the current size of the file in bytes.
	FileSize() (int64, error)
	// Lock raises the file lock to at least the given level. It may return
	// &Error{ErrBusy} when another connection holds a conflicting lock, which
	// SQLite handles by retrying or reporting SQLITE_BUSY.
	Lock(lock LockType) error
	// Unlock lowers the file lock to the given level (LockShared or LockNone).
	Unlock(lock LockType) error
	// CheckReservedLock reports whether any connection (including this one)
	// holds a RESERVED, PENDING or EXCLUSIVE lock. SQLite uses it to detect a
	// hot journal.
	CheckReservedLock() (bool, error)
	// FileControl handles SQLITE_FCNTL_* opcodes. Return &Error{ErrNotFound}
	// for opcodes that are not handled.
	FileControl(op int, arg unsafe.Pointer) error
	// SectorSize returns the sector size of the underlying device in bytes.
	// SQLite uses it to align journal writes.
	SectorSize() int
	// DeviceCharacteristics returns a bit set of IOCap* flags describing
	// guarantees of the underlying device (e.g. IOCapPowersafeOverwrite).
	DeviceCharacteristics() DeviceCharacteristic
}

// URIOpener can be implemented by a VFS that wants the URI query parameters of
// the file being opened. If implemented, it is used instead of Open.
type URIOpener interface {
	OpenURI(name string, uri URI, flags OpenFlag) (File, OpenFlag, error)
}

// ReadResultCode converts the result of File.ReadAt(p, ...) (n bytes read, err)
// into the SQLite result code, zero-filling the unread tail of p for a short
// read as SQLite requires.
func ReadResultCode(p []byte, n int, err error) int {
	if err == nil {
		return 0
	}
	short := errors.Is(err, io.EOF)
	var e *Error
	if errors.As(err, &e) && e.Code == ErrIOShortRead {
		short = true
	}
	if !short {
		return CodeFromErr(err)
	}
	if n < 0 {
		n = 0
	}
	for i := n; i < len(p); i++ {
		p[i] = 0
	}
	return int(ErrIOShortRead)
}

// julianUnixEpochMs is the unix epoch expressed in SQLite's julian-day
// milliseconds.
const julianUnixEpochMs = 210866760000000

// JulianMs converts t to SQLite's julian-day milliseconds, as used by
// xCurrentTimeInt64.
func JulianMs(t time.Time) int64 { return t.UnixMilli() + julianUnixEpochMs }

// TimeFromJulianMs is the inverse of JulianMs.
func TimeFromJulianMs(ms int64) time.Time { return time.UnixMilli(ms - julianUnixEpochMs) }

// CopyCString copies s plus a terminating NUL into dst. It reports false (and
// writes nothing) if dst is too small.
func CopyCString(dst []byte, s string) bool {
	if len(s)+1 > len(dst) {
		return false
	}
	copy(dst, s)
	dst[len(s)] = 0
	return true
}

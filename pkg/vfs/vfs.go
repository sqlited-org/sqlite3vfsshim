// Package vfs lets you implement a complete SQLite VFS (virtual file system)
// in Go. Register it with Register and select it with the "vfs" URI
// parameter, e.g. "file:test.db?vfs=myvfs".
//
// To wrap an existing VFS instead of replacing it, see the sibling package
// github.com/sqlited-org/sqlite3vfsshim/pkg/vfsshim.
package vfs

import (
	"time"
	"unsafe"

	"github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore"
)

type options struct {
	maxPathname int
	makeDefault bool
}

// VFS is the minimal set of operations every VFS must provide.
//
// Optionally also implement Randomizer, Sleeper, Clock and/or URIOpener; the
// defaults use the Go standard library.
type VFS interface {
	// Open opens the named file. name is "" for anonymous temporary files.
	// flags are the OpenFlag values SQLite asked for; the returned OpenFlag
	// reports what was actually used (usually flags unchanged).
	Open(name string, flags vfscore.OpenFlag) (File, vfscore.OpenFlag, error)

	// Delete removes the named file. SQLite uses it for journal, WAL and
	// temporary files. If dirSync is true the deletion must be durable before
	// returning. Deleting a missing file should return an *Error with code
	// ErrIO|(23<<8) (SQLITE_IOERR_DELETE_NOENT).
	Delete(name string, dirSync bool) error

	// Access reports whether name satisfies flags: AccessExists,
	// AccessReadWrite or AccessRead. A missing file is not an error: return
	// (false, nil).
	Access(name string, flags vfscore.AccessFlag) (bool, error)

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
	Sync(flags vfscore.SyncType) error
	// FileSize returns the current size of the file in bytes.
	FileSize() (int64, error)
	// Lock raises the file lock to at least the given level. It may return
	// &Error{ErrBusy} when another connection holds a conflicting lock, which
	// SQLite handles by retrying or reporting SQLITE_BUSY.
	Lock(lock vfscore.LockType) error
	// Unlock lowers the file lock to the given level (LockShared or LockNone).
	Unlock(lock vfscore.LockType) error
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
	DeviceCharacteristics() vfscore.DeviceCharacteristic
}

// Option configures Register.
type Option func(*options)

// WithMaxPathname sets the maximum path name length (default 1024).
func WithMaxPathname(n int) Option { return func(o *options) { o.maxPathname = n } }

// MakeDefault makes the VFS SQLite's default VFS.
func MakeDefault() Option { return func(o *options) { o.makeDefault = true } }

package vfsshim

import (
	"time"
	"unsafe"

	"github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore"
)

// Shim is the value passed to Register. It is an empty interface on purpose: a
// shim implements only the hooks it needs, from the optional *Hook interfaces
// below.
//
// The parent VFS lives entirely in C. Go only names it (WithParent). For every
// call SQLite makes on the shim, the C side does this:
//
//  1. if the shim (or the file's File value) implements the matching hook, call
//     it first;
//  2. if the hook returned an error, stop and return that error to SQLite (the
//     hook "vetoes" the call); the parent is not called;
//  3. otherwise call the same method of the parent VFS / parent file, still in
//     C, and return its result.
//
// Calls without a hook never enter Go. An empty struct{} is a pure passthrough
// shim. Hooks observe or veto; they cannot replace a result. To implement a
// VFS that produces its own results, use package vfs instead.
type Shim = any

// File is the per-file value returned by OpenHook.Open. It holds the hooks for
// one open file (see the file-level *Hook interfaces) and may be nil if no
// file hook is needed. Its Close hook is also the place to release resources.
type File = any

// VFS-level hooks, implemented by the value given to Register.

// OpenHook observes or vetoes opening a file; it runs before the parent opens it.
type OpenHook interface {
	// Open is called with the file name ("" for temporary files) and the
	// requested flags. The returned File receives the hooks of this open
	// file and may be nil. A non-nil error makes the open fail with that
	// error and the parent is not asked to open the file. A shim that also
	// needs the URI query parameters implements vfscore.URIOpener instead, which
	// takes precedence over OpenHook (its returned flags are ignored).
	Open(name string, flags vfscore.OpenFlag) (File, error)
}

// DeleteHook runs before the parent's Delete.
type DeleteHook interface {
	// Delete is called when SQLite removes a journal, WAL or temporary file.
	// A non-nil error prevents the deletion.
	Delete(name string, dirSync bool) error
}

// AccessHook runs before the parent's Access.
type AccessHook interface {
	// Access is called when SQLite checks for a file (for example a hot
	// journal or WAL). A non-nil error is returned to SQLite instead.
	Access(name string, flags vfscore.AccessFlag) error
}

// FullPathnameHook runs before the parent's FullPathname.
type FullPathnameHook interface {
	// FullPathname is called before SQLite opens a database. A non-nil error
	// fails the call.
	FullPathname(name string) error
}

// RandomnessHook runs before the parent's Randomness. It cannot fail.
type RandomnessHook interface {
	// Randomness is called when SQLite asks for n bytes of randomness.
	Randomness(n int)
}

// SleepHook runs before the parent's Sleep. It cannot fail.
type SleepHook interface {
	// Sleep is called when SQLite's busy handler sleeps for about d.
	Sleep(d time.Duration)
}

// CurrentTimeHook runs before the parent's CurrentTime.
type CurrentTimeHook interface {
	// CurrentTime is called for date/time SQL functions such as
	// datetime('now'). A non-nil error fails the call.
	CurrentTime() error
}

// File-level hooks, implemented by the File returned from OpenHook.Open. Each
// runs before the parent file performs the same operation; a non-nil error
// stops the operation and is returned to SQLite.

// CloseHook runs before the parent file is closed. The parent file is closed
// even if it returns an error; the error is then reported to SQLite.
type CloseHook interface{ Close() error }

// ReadHook runs before the parent file reads. p is the destination buffer and
// has not been filled yet.
type ReadHook interface {
	ReadAt(p []byte, off int64) error
}

// WriteHook runs before the parent file writes. p holds the data about to be
// written and must not be modified or retained.
type WriteHook interface {
	WriteAt(p []byte, off int64) error
}

// TruncateHook runs before xTruncate.
type TruncateHook interface{ Truncate(size int64) error }

// SyncHook runs before xSync.
type SyncHook interface {
	Sync(flags vfscore.SyncType) error
}

// FileSizeHook runs before xFileSize.
type FileSizeHook interface{ FileSize() error }

// LockHook runs before xLock.
type LockHook interface {
	Lock(lock vfscore.LockType) error
}

// UnlockHook runs before xUnlock.
type UnlockHook interface {
	Unlock(lock vfscore.LockType) error
}

// CheckReservedLockHook runs before xCheckReservedLock.
type CheckReservedLockHook interface{ CheckReservedLock() error }

// FileControlHook runs before xFileControl (SQLITE_FCNTL_* opcodes).
type FileControlHook interface {
	FileControl(op int, arg unsafe.Pointer) error
}

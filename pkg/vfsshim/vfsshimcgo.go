package vfsshim

/*
#cgo CFLAGS: -I${SRCDIR}/../vfscore/include
#include <stdlib.h>
#include "shim.h"
*/
import "C"

import (
	"runtime/cgo"
	"time"
	"unsafe"

	"github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore"
)

// Go side of the shim: how hook calls cross the C/Go boundary.
//
// The parent VFS is never touched from Go. It is chosen by name (WithParent),
// resolved and stored in C (shimRegister), and every parent call is made from
// C. Go only provides hooks. For any call, e.g. a write:
//
//	SQLite xWrite
//	  -> shimWrite()              (shim.c)    sqlite3_io_methods entry
//	  -> goShimFileWrite()        (this file) only if the File implements
//	                                          WriteHook; non-zero result = veto
//	  -> parent's xWrite          (shim.c)    called explicitly from C
//
// Go objects never cross into C directly (cgo forbids storing Go pointers in C
// memory): the Shim and each File are stored as cgo.Handle values (plain
// integers) that C keeps and passes back, and are resolved again by
// shimFromHandle / fileFromHandle.

// ---- Go entry points called from shim.c ----
//
// Direction: C -> Go. Each //export'ed goShim* function is a hook call made
// by a method in shim.c on behalf of SQLite, before shim.c calls the parent
// itself. It converts C arguments to Go values (buffers are viewed with
// unsafe.Slice, not copied), resolves the Go Shim or File from its cgo.Handle,
// runs the hook and turns the returned error into a SQLite result code with
// vfscore.CodeFromErr. Non-zero means "veto": shim.c returns it without
// calling the parent. A panic in user code is recovered by safe() and reported
// as SQLITE_IOERR so it never unwinds through C frames.

func shimFromHandle(h C.uintptr_t) Shim { return cgo.Handle(h).Value() }
func fileFromHandle(h C.uintptr_t) File { return cgo.Handle(h).Value() }

// safe converts panics in user code into SQLITE_IOERR instead of crashing
// through the C stack.
func safe(rc *C.int) {
	if r := recover(); r != nil {
		*rc = C.int(vfscore.ErrIO)
	}
}

// quiet swallows panics of hooks that cannot fail.
func quiet() { _ = recover() }

//export goShimOpen
func goShimOpen(h C.uintptr_t, zName *C.char, flags C.int, outFile *C.uintptr_t, outMask *C.uint) (rc C.int) {
	defer safe(&rc)

	*outFile, *outMask = 0, 0
	var name string
	if zName != nil {
		name = C.GoString(zName)
	}

	// vfscore.URIOpener (needs the URI parameters) takes precedence over
	// OpenHook; the flags it returns are ignored by the shim.
	var (
		f   File
		err error
	)
	switch shim := shimFromHandle(h).(type) {
	case vfscore.URIOpener:
		var vf vfscore.File
		vf, _, err = shim.OpenURI(name, vfscore.NewURI(unsafe.Pointer(zName)), vfscore.OpenFlag(flags))
		if vf != nil {
			f = vf
		}
	case OpenHook:
		f, err = shim.Open(name, vfscore.OpenFlag(flags))
	}
	if err != nil {
		return C.int(vfscore.CodeFromErr(err))
	}

	if f != nil {
		*outFile = C.uintptr_t(cgo.NewHandle(f))
		*outMask = C.uint(fileMask(f))
	}
	return 0
}

//export goShimDelete
func goShimDelete(h C.uintptr_t, zName *C.char, syncDir C.int) (rc C.int) {
	defer safe(&rc)

	if hook, ok := shimFromHandle(h).(DeleteHook); ok {
		return C.int(vfscore.CodeFromErr(hook.Delete(C.GoString(zName), syncDir != 0)))
	}
	return 0
}

//export goShimAccess
func goShimAccess(h C.uintptr_t, zName *C.char, flags C.int) (rc C.int) {
	defer safe(&rc)

	if hook, ok := shimFromHandle(h).(AccessHook); ok {
		return C.int(vfscore.CodeFromErr(hook.Access(C.GoString(zName), vfscore.AccessFlag(flags))))
	}
	return 0
}

//export goShimFullPathname
func goShimFullPathname(h C.uintptr_t, zName *C.char) (rc C.int) {
	defer safe(&rc)

	if hook, ok := shimFromHandle(h).(FullPathnameHook); ok {
		return C.int(vfscore.CodeFromErr(hook.FullPathname(C.GoString(zName))))
	}
	return 0
}

//export goShimRandomness
func goShimRandomness(h C.uintptr_t, n C.int, zOut *C.char) {
	defer quiet()

	if hook, ok := shimFromHandle(h).(RandomnessHook); ok {
		hook.Randomness(int(n))
	}
}

//export goShimSleep
func goShimSleep(h C.uintptr_t, us C.int) {
	defer quiet()

	if hook, ok := shimFromHandle(h).(SleepHook); ok {
		hook.Sleep(time.Duration(us) * time.Microsecond)
	}
}

//export goShimCurrentTime
func goShimCurrentTime(h C.uintptr_t) (rc C.int) {
	defer safe(&rc)

	if hook, ok := shimFromHandle(h).(CurrentTimeHook); ok {
		return C.int(vfscore.CodeFromErr(hook.CurrentTime()))
	}
	return 0
}

//export goShimFileClose
func goShimFileClose(fh C.uintptr_t, callClose C.int) (rc C.int) {
	defer safe(&rc)
	defer cgo.Handle(fh).Delete()

	if callClose == 0 {
		return 0
	}
	if hook, ok := fileFromHandle(fh).(CloseHook); ok {
		return C.int(vfscore.CodeFromErr(hook.Close()))
	}
	return 0
}

//export goShimFileRead
func goShimFileRead(fh C.uintptr_t, buf unsafe.Pointer, amt C.int, ofst C.sqlite3_int64) (rc C.int) {
	defer safe(&rc)

	if hook, ok := fileFromHandle(fh).(ReadHook); ok {
		return C.int(vfscore.CodeFromErr(hook.ReadAt(unsafe.Slice((*byte)(buf), int(amt)), int64(ofst))))
	}
	return 0
}

//export goShimFileWrite
func goShimFileWrite(fh C.uintptr_t, buf unsafe.Pointer, amt C.int, ofst C.sqlite3_int64) (rc C.int) {
	defer safe(&rc)

	if hook, ok := fileFromHandle(fh).(WriteHook); ok {
		return C.int(vfscore.CodeFromErr(hook.WriteAt(unsafe.Slice((*byte)(buf), int(amt)), int64(ofst))))
	}
	return 0
}

//export goShimFileTruncate
func goShimFileTruncate(fh C.uintptr_t, size C.sqlite3_int64) (rc C.int) {
	defer safe(&rc)

	if hook, ok := fileFromHandle(fh).(TruncateHook); ok {
		return C.int(vfscore.CodeFromErr(hook.Truncate(int64(size))))
	}
	return 0
}

//export goShimFileSync
func goShimFileSync(fh C.uintptr_t, flags C.int) (rc C.int) {
	defer safe(&rc)

	if hook, ok := fileFromHandle(fh).(SyncHook); ok {
		return C.int(vfscore.CodeFromErr(hook.Sync(vfscore.SyncType(flags))))
	}
	return 0
}

//export goShimFileSize
func goShimFileSize(fh C.uintptr_t) (rc C.int) {
	defer safe(&rc)

	if hook, ok := fileFromHandle(fh).(FileSizeHook); ok {
		return C.int(vfscore.CodeFromErr(hook.FileSize()))
	}
	return 0
}

//export goShimFileLock
func goShimFileLock(fh C.uintptr_t, l C.int) (rc C.int) {
	defer safe(&rc)

	if hook, ok := fileFromHandle(fh).(LockHook); ok {
		return C.int(vfscore.CodeFromErr(hook.Lock(vfscore.LockType(l))))
	}
	return 0
}

//export goShimFileUnlock
func goShimFileUnlock(fh C.uintptr_t, l C.int) (rc C.int) {
	defer safe(&rc)

	if hook, ok := fileFromHandle(fh).(UnlockHook); ok {
		return C.int(vfscore.CodeFromErr(hook.Unlock(vfscore.LockType(l))))
	}
	return 0
}

//export goShimFileCheckReservedLock
func goShimFileCheckReservedLock(fh C.uintptr_t) (rc C.int) {
	defer safe(&rc)

	if hook, ok := fileFromHandle(fh).(CheckReservedLockHook); ok {
		return C.int(vfscore.CodeFromErr(hook.CheckReservedLock()))
	}
	return 0
}

//export goShimFileControl
func goShimFileControl(fh C.uintptr_t, op C.int, arg unsafe.Pointer) (rc C.int) {
	defer safe(&rc)

	if hook, ok := fileFromHandle(fh).(FileControlHook); ok {
		return C.int(vfscore.CodeFromErr(hook.FileControl(int(op), arg)))
	}
	return 0
}

// ---- Implemented-method masks ----
//
// The C side forwards to the parent every method whose bit is clear, so Go is
// only entered for what the shim or file actually implements. The bits are the
// SHIM_V_* (VFS) and SHIM_F_* (file) macros of shim.h.

func vfsMask(s any) (m C.uint) {
	switch s.(type) {
	case vfscore.URIOpener, OpenHook:
		m |= C.SHIM_V_OPEN
	}
	if _, ok := s.(DeleteHook); ok {
		m |= C.SHIM_V_DELETE
	}
	if _, ok := s.(AccessHook); ok {
		m |= C.SHIM_V_ACCESS
	}
	if _, ok := s.(FullPathnameHook); ok {
		m |= C.SHIM_V_FULLPATH
	}
	if _, ok := s.(RandomnessHook); ok {
		m |= C.SHIM_V_RANDOM
	}
	if _, ok := s.(SleepHook); ok {
		m |= C.SHIM_V_SLEEP
	}
	if _, ok := s.(CurrentTimeHook); ok {
		m |= C.SHIM_V_TIME
	}
	return m
}

func fileMask(f any) (m C.uint) {
	if _, ok := f.(CloseHook); ok {
		m |= C.SHIM_F_CLOSE
	}
	if _, ok := f.(ReadHook); ok {
		m |= C.SHIM_F_READ
	}
	if _, ok := f.(WriteHook); ok {
		m |= C.SHIM_F_WRITE
	}
	if _, ok := f.(TruncateHook); ok {
		m |= C.SHIM_F_TRUNCATE
	}
	if _, ok := f.(SyncHook); ok {
		m |= C.SHIM_F_SYNC
	}
	if _, ok := f.(FileSizeHook); ok {
		m |= C.SHIM_F_SIZE
	}
	if _, ok := f.(LockHook); ok {
		m |= C.SHIM_F_LOCK
	}
	if _, ok := f.(UnlockHook); ok {
		m |= C.SHIM_F_UNLOCK
	}
	if _, ok := f.(CheckReservedLockHook); ok {
		m |= C.SHIM_F_CHECKRES
	}
	if _, ok := f.(FileControlHook); ok {
		m |= C.SHIM_F_CONTROL
	}
	return m
}

// ---- Registration ----

type options struct {
	parent      string
	makeDefault bool
}

// Option configures Register.
type Option func(*options)

// WithParent selects the VFS to wrap (e.g. "unix"). The default is SQLite's
// current default VFS.
func WithParent(name string) Option { return func(o *options) { o.parent = name } }

// MakeDefault makes the shim SQLite's default VFS.
func MakeDefault() Option { return func(o *options) { o.makeDefault = true } }

// Register registers shim as a new VFS called name, layered on top of its
// parent VFS. Select it with the "vfs" URI parameter, e.g.
// "file:test.db?vfs=name".
func Register(name string, shim any, opts ...Option) error {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var cparent *C.char
	if o.parent != "" {
		cparent = C.CString(o.parent)
		defer C.free(unsafe.Pointer(cparent))
	}

	var def C.int
	if o.makeDefault {
		def = 1
	}

	h := cgo.NewHandle(shim)
	rc := C.shimRegister(cname, cparent, C.uintptr_t(h), def, vfsMask(shim))
	if rc != 0 {
		h.Delete()
		return vfscore.ErrFromCode(int(rc))
	}
	return nil
}

// Unregister removes a shim VFS registered with Register. No database
// connection using it may be open.
func Unregister(name string) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var h C.uintptr_t
	rc := C.shimUnregister(cname, &h)
	if rc != 0 {
		return vfscore.ErrFromCode(int(rc))
	}
	cgo.Handle(h).Delete()
	return nil
}

// InitExtension must be called first from the entry point of a SQLite
// loadable extension (sqlite3_<name>_init), passing the
// sqlite3_api_routines pointer SQLite supplies. It is a no-op in normal builds.
// See examples/traceshim-ext.
func InitExtension(pApi unsafe.Pointer) { C.shimSetAPI(pApi) }

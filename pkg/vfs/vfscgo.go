package vfs

/*
#cgo CFLAGS: -I${SRCDIR}/../vfscore/include
#include <stdlib.h>
#include "vfs.h"
*/
import "C"

import (
	"crypto/rand"
	"runtime/cgo"
	"time"
	"unsafe"

	"github.com/emo/sqlite3vfsshim/pkg/vfscore"
)

// Go side of the library: how calls cross the Go/C boundary. All calls travel
// in one direction, SQLite -> Go (vfs.c has the C view):
//
//	SQLite xWrite -> vfsWrite() (vfs.c) -> goVFSFileWrite() (this file) -> File.WriteAt()
//
// Go objects never live in C memory: the VFS and each File are stored as
// cgo.Handle values (plain integers) that C keeps and passes back, and are
// resolved again by vfsFromHandle / fileFromHandle. A panic in user code is
// recovered by safe() and reported as SQLITE_IOERR so it never unwinds
// through C frames.

func vfsFromHandle(h C.uintptr_t) VFS   { return cgo.Handle(h).Value().(VFS) }
func fileFromHandle(h C.uintptr_t) File { return cgo.Handle(h).Value().(File) }

func safe(rc *C.int) {
	if r := recover(); r != nil {
		*rc = C.int(vfscore.ErrIO)
	}
}

//export goVFSOpen
func goVFSOpen(h C.uintptr_t, zName *C.char, flags C.int, outFlags *C.int, outFile *C.uintptr_t) (rc C.int) {
	defer safe(&rc)
	var name string
	if zName != nil {
		name = C.GoString(zName)
	}
	v := vfsFromHandle(h)
	var (
		f   File
		out vfscore.OpenFlag
		err error
	)
	if o, ok := v.(vfscore.URIOpener); ok {
		f, out, err = o.OpenURI(name, vfscore.NewURI(unsafe.Pointer(zName)), vfscore.OpenFlag(flags))
	} else {
		f, out, err = v.Open(name, vfscore.OpenFlag(flags))
	}
	if err != nil {
		return C.int(vfscore.CodeFromErr(err))
	}
	if f == nil {
		return C.int(vfscore.ErrCantOpen)
	}
	*outFlags = C.int(out)
	*outFile = C.uintptr_t(cgo.NewHandle(f))
	return 0
}

//export goVFSDelete
func goVFSDelete(h C.uintptr_t, zName *C.char, syncDir C.int) (rc C.int) {
	defer safe(&rc)
	return C.int(vfscore.CodeFromErr(vfsFromHandle(h).Delete(C.GoString(zName), syncDir != 0)))
}

//export goVFSAccess
func goVFSAccess(h C.uintptr_t, zName *C.char, flags C.int, res *C.int) (rc C.int) {
	defer safe(&rc)

	ok, err := vfsFromHandle(h).Access(C.GoString(zName), vfscore.AccessFlag(flags))
	*res = 0
	if ok {
		*res = 1
	}
	return C.int(vfscore.CodeFromErr(err))
}

//export goVFSFullPathname
func goVFSFullPathname(h C.uintptr_t, zName *C.char, nOut C.int, zOut *C.char) (rc C.int) {
	defer safe(&rc)
	s, err := vfsFromHandle(h).FullPathname(C.GoString(zName))
	if err != nil {
		return C.int(vfscore.CodeFromErr(err))
	}
	if !vfscore.CopyCString(unsafe.Slice((*byte)(unsafe.Pointer(zOut)), int(nOut)), s) {
		return C.int(vfscore.ErrCantOpen)
	}
	return 0
}

//export goVFSRandomness
func goVFSRandomness(h C.uintptr_t, n C.int, zOut *C.char) (rc C.int) {
	defer safe(&rc)
	if n <= 0 {
		return 0
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(zOut)), int(n))
	if r, ok := vfsFromHandle(h).(Randomizer); ok {
		return C.int(r.Randomness(b))
	}
	k, _ := rand.Read(b)
	return C.int(k)
}

//export goVFSSleep
func goVFSSleep(h C.uintptr_t, us C.int) (rc C.int) {
	defer safe(&rc)
	d := time.Duration(us) * time.Microsecond
	if s, ok := vfsFromHandle(h).(Sleeper); ok {
		d = s.Sleep(d)
	} else {
		time.Sleep(d)
	}
	return C.int(d / time.Microsecond)
}

//export goVFSCurrentTime
func goVFSCurrentTime(h C.uintptr_t, now *C.sqlite3_int64) (rc C.int) {
	defer safe(&rc)
	t := time.Now()
	if c, ok := vfsFromHandle(h).(Clock); ok {
		var err error
		if t, err = c.CurrentTime(); err != nil {
			return C.int(vfscore.CodeFromErr(err))
		}
	}
	*now = C.sqlite3_int64(vfscore.JulianMs(t))
	return 0
}

//export goVFSFileClose
func goVFSFileClose(fh C.uintptr_t) (rc C.int) {
	defer safe(&rc)
	defer cgo.Handle(fh).Delete()
	return C.int(vfscore.CodeFromErr(fileFromHandle(fh).Close()))
}

//export goVFSFileRead
func goVFSFileRead(fh C.uintptr_t, buf unsafe.Pointer, amt C.int, ofst C.sqlite3_int64) (rc C.int) {
	defer safe(&rc)
	p := unsafe.Slice((*byte)(buf), int(amt))
	n, err := fileFromHandle(fh).ReadAt(p, int64(ofst))
	return C.int(vfscore.ReadResultCode(p, n, err))
}

//export goVFSFileWrite
func goVFSFileWrite(fh C.uintptr_t, buf unsafe.Pointer, amt C.int, ofst C.sqlite3_int64) (rc C.int) {
	defer safe(&rc)
	_, err := fileFromHandle(fh).WriteAt(unsafe.Slice((*byte)(buf), int(amt)), int64(ofst))
	return C.int(vfscore.CodeFromErr(err))
}

//export goVFSFileTruncate
func goVFSFileTruncate(fh C.uintptr_t, size C.sqlite3_int64) (rc C.int) {
	defer safe(&rc)
	return C.int(vfscore.CodeFromErr(fileFromHandle(fh).Truncate(int64(size))))
}

//export goVFSFileSync
func goVFSFileSync(fh C.uintptr_t, flags C.int) (rc C.int) {
	defer safe(&rc)
	return C.int(vfscore.CodeFromErr(fileFromHandle(fh).Sync(vfscore.SyncType(flags))))
}

//export goVFSFileSize
func goVFSFileSize(fh C.uintptr_t, out *C.sqlite3_int64) (rc C.int) {
	defer safe(&rc)
	n, err := fileFromHandle(fh).FileSize()
	*out = C.sqlite3_int64(n)
	return C.int(vfscore.CodeFromErr(err))
}

//export goVFSFileLock
func goVFSFileLock(fh C.uintptr_t, l C.int) (rc C.int) {
	defer safe(&rc)
	return C.int(vfscore.CodeFromErr(fileFromHandle(fh).Lock(vfscore.LockType(l))))
}

//export goVFSFileUnlock
func goVFSFileUnlock(fh C.uintptr_t, l C.int) (rc C.int) {
	defer safe(&rc)
	return C.int(vfscore.CodeFromErr(fileFromHandle(fh).Unlock(vfscore.LockType(l))))
}

//export goVFSFileCheckReservedLock
func goVFSFileCheckReservedLock(fh C.uintptr_t, out *C.int) (rc C.int) {
	defer safe(&rc)
	ok, err := fileFromHandle(fh).CheckReservedLock()
	*out = 0
	if ok {
		*out = 1
	}
	return C.int(vfscore.CodeFromErr(err))
}

//export goVFSFileControl
func goVFSFileControl(fh C.uintptr_t, op C.int, arg unsafe.Pointer) (rc C.int) {
	defer safe(&rc)
	return C.int(vfscore.CodeFromErr(fileFromHandle(fh).FileControl(int(op), arg)))
}

//export goVFSFileSectorSize
func goVFSFileSectorSize(fh C.uintptr_t) (rc C.int) {
	defer func() {
		if recover() != nil {
			rc = 4096
		}
	}()
	return C.int(fileFromHandle(fh).SectorSize())
}

//export goVFSFileDeviceCharacteristics
func goVFSFileDeviceCharacteristics(fh C.uintptr_t) (rc C.int) {
	defer func() {
		if recover() != nil {
			rc = 0
		}
	}()
	return C.int(fileFromHandle(fh).DeviceCharacteristics())
}

// Register registers v as a new VFS called name. Select it with the "vfs" URI
// parameter, e.g. "file:test.db?vfs=name".
func Register(name string, v VFS, opts ...Option) error {
	o := options{maxPathname: 1024}
	for _, opt := range opts {
		opt(&o)
	}
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var def C.int
	if o.makeDefault {
		def = 1
	}
	h := cgo.NewHandle(v)
	rc := C.vfsRegister(cname, C.uintptr_t(h), C.int(o.maxPathname), def)
	if rc != 0 {
		h.Delete()
		return vfscore.ErrFromCode(int(rc))
	}
	return nil
}

// Unregister removes a VFS registered with Register. No database connection
// using it may be open.
func Unregister(name string) error {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	var h C.uintptr_t
	rc := C.vfsUnregister(cname, &h)
	if rc != 0 {
		return vfscore.ErrFromCode(int(rc))
	}
	cgo.Handle(h).Delete()
	return nil
}

// InitExtension must be called first from the entry point of a SQLite
// loadable extension (sqlite3_<name>_init), passing the sqlite3_api_routines
// pointer SQLite supplies. It is a no-op in normal builds. Build with
// -tags SQLITE3VFS_LOADABLE_EXT -buildmode=c-shared.
func InitExtension(pApi unsafe.Pointer) { C.vfsSetAPI(pApi) }

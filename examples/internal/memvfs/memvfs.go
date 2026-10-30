// Package memvfs is a complete in-memory SQLite VFS written with the
// github.com/sqlited-org/sqlite3vfsshim/pkg/vfs library. It is shared by the memvfs and
// memvfs-ext examples.
//
// Limitations: locking is a no-op, so use a single connection per database;
// WAL mode and mmap are not available.
package memvfs

import (
	"fmt"
	"io"
	"sync"
	"unsafe"

	"github.com/sqlited-org/sqlite3vfsshim/pkg/vfs"
	"github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore"
)

// VFS keeps every file (database, journal, temp files) in a map.
type MemVFS struct {
	mu    sync.Mutex
	files map[string]*data
	temp  int
}

type data struct {
	mu  sync.Mutex
	buf []byte
}

// New returns an empty in-memory VFS.
func New() *MemVFS { return &MemVFS{files: map[string]*data{}} }

// Size returns the size of the named file, or -1 if it does not exist.
func (v *MemVFS) Size(name string) int {
	v.mu.Lock()
	defer v.mu.Unlock()
	d, ok := v.files[name]
	if !ok {
		return -1
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.buf)
}

func (v *MemVFS) Open(name string, flags vfscore.OpenFlag) (vfs.File, vfscore.OpenFlag, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if name == "" { // SQLite asks for an anonymous temporary file
		v.temp++
		name = fmt.Sprintf("\x00temp-%d", v.temp)
	}
	d, ok := v.files[name]
	if !ok {
		if flags&vfscore.OpenCreate == 0 {
			return nil, 0, &vfscore.Error{Code: vfscore.ErrCantOpen}
		}
		d = &data{}
		v.files[name] = d
	}
	return &file{vfs: v, d: d, name: name, deleteOnClose: flags&vfscore.OpenDeleteOnClose != 0}, flags, nil
}

func (v *MemVFS) Delete(name string, _ bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.files[name]; !ok {
		return &vfscore.Error{Code: vfscore.ErrIO | (23 << 8)} // SQLITE_IOERR_DELETE_NOENT
	}
	delete(v.files, name)
	return nil
}

func (v *MemVFS) Access(name string, _ vfscore.AccessFlag) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.files[name]
	return ok, nil
}

// There are no directories, so names are already canonical.
func (v *MemVFS) FullPathname(name string) (string, error) { return name, nil }

type file struct {
	vfs           *MemVFS
	d             *data
	name          string
	deleteOnClose bool
}

func (f *file) Close() error {
	if f.deleteOnClose {
		f.vfs.mu.Lock()
		delete(f.vfs.files, f.name)
		f.vfs.mu.Unlock()
	}
	return nil
}

func (f *file) ReadAt(p []byte, off int64) (int, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if off >= int64(len(f.d.buf)) {
		return 0, io.EOF // short read: the rest of p is zero-filled for us
	}
	n := copy(p, f.d.buf[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *file) WriteAt(p []byte, off int64) (int, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if end := off + int64(len(p)); end > int64(len(f.d.buf)) {
		f.d.buf = append(f.d.buf, make([]byte, end-int64(len(f.d.buf)))...)
	}
	copy(f.d.buf[off:], p)
	return len(p), nil
}

func (f *file) Truncate(size int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if size < int64(len(f.d.buf)) {
		f.d.buf = f.d.buf[:size]
	} else {
		f.d.buf = append(f.d.buf, make([]byte, size-int64(len(f.d.buf)))...)
	}
	return nil
}

func (f *file) Sync(vfscore.SyncType) error { return nil } // memory: nothing to flush
func (f *file) FileSize() (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	return int64(len(f.d.buf)), nil
}

func (f *file) Lock(vfscore.LockType) error                         { return nil }
func (f *file) Unlock(vfscore.LockType) error                       { return nil }
func (f *file) CheckReservedLock() (bool, error)                    { return false, nil }
func (f *file) SectorSize() int                                     { return 4096 }
func (f *file) DeviceCharacteristics() vfscore.DeviceCharacteristic { return 0 }
func (f *file) FileControl(int, unsafe.Pointer) error {
	return &vfscore.Error{Code: vfscore.ErrNotFound} // no file controls handled
}

package vfs_test

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/emo/sqlite3vfsshim/pkg/vfs"
	"github.com/emo/sqlite3vfsshim/pkg/vfscore"
	_ "github.com/mattn/go-sqlite3"
)

// memVFS is a complete in-memory VFS.
type memVFS struct {
	mu         sync.Mutex
	files      map[string]*memData
	temp       int
	failWrites bool
	denyURI    bool // deny Open when the URI has deny=1 (uses URIOpener)
}

type memData struct {
	mu   sync.Mutex
	data []byte
}

func newMemVFS() *memVFS { return &memVFS{files: map[string]*memData{}} }

func (v *memVFS) Open(name string, fl vfscore.OpenFlag) (vfs.File, vfscore.OpenFlag, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if name == "" {
		v.temp++
		name = fmt.Sprintf("\x00temp-%d", v.temp)
	}
	d, ok := v.files[name]
	if !ok {
		if fl&vfscore.OpenCreate == 0 {
			return nil, 0, &vfscore.Error{Code: vfscore.ErrCantOpen}
		}
		d = &memData{}
		v.files[name] = d
	}
	return &memFile{v: v, d: d, name: name, deleteOnClose: fl&vfscore.OpenDeleteOnClose != 0}, fl, nil
}

func (v *memVFS) Delete(name string, dirSync bool) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.files[name]; !ok {
		return &vfscore.Error{Code: vfscore.ErrIO | (23 << 8)}
	}
	delete(v.files, name)
	return nil
}

func (v *memVFS) Access(name string, fl vfscore.AccessFlag) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.files[name]
	return ok, nil
}

func (v *memVFS) FullPathname(name string) (string, error) { return name, nil }

type memFile struct {
	v             *memVFS
	d             *memData
	name          string
	deleteOnClose bool
}

func (f *memFile) Close() error {
	if f.deleteOnClose {
		f.v.mu.Lock()
		delete(f.v.files, f.name)
		f.v.mu.Unlock()
	}
	return nil
}
func (f *memFile) ReadAt(p []byte, off int64) (int, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if off >= int64(len(f.d.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.d.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
func (f *memFile) WriteAt(p []byte, off int64) (int, error) {
	if f.v.failWrites {
		return 0, &vfscore.Error{Code: vfscore.ErrFull}
	}
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if end := off + int64(len(p)); end > int64(len(f.d.data)) {
		f.d.data = append(f.d.data, make([]byte, end-int64(len(f.d.data)))...)
	}
	copy(f.d.data[off:], p)
	return len(p), nil
}
func (f *memFile) Truncate(size int64) error {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	if size < int64(len(f.d.data)) {
		f.d.data = f.d.data[:size]
	} else {
		f.d.data = append(f.d.data, make([]byte, size-int64(len(f.d.data)))...)
	}
	return nil
}
func (f *memFile) Sync(vfscore.SyncType) error { return nil }
func (f *memFile) FileSize() (int64, error) {
	f.d.mu.Lock()
	defer f.d.mu.Unlock()
	return int64(len(f.d.data)), nil
}
func (f *memFile) Lock(vfscore.LockType) error                         { return nil }
func (f *memFile) Unlock(vfscore.LockType) error                       { return nil }
func (f *memFile) CheckReservedLock() (bool, error)                    { return false, nil }
func (f *memFile) SectorSize() int                                     { return 4096 }
func (f *memFile) DeviceCharacteristics() vfscore.DeviceCharacteristic { return 0 }
func (f *memFile) FileControl(int, unsafe.Pointer) error {
	return &vfscore.Error{Code: vfscore.ErrNotFound}
}

func openDB(t *testing.T, vfsName, file, q string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?vfs=%s%s", file, vfsName, q))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func TestMemVFS(t *testing.T) {
	m := newMemVFS()
	if err := vfs.Register("mem", m); err != nil {
		t.Fatal(err)
	}
	defer vfs.Unregister("mem")

	db := openDB(t, "mem", "mem.db", "")
	mustExec(t, db, `create table t(id integer primary key, v text)`)
	for i := 0; i < 200; i++ {
		mustExec(t, db, `insert into t(v) values (?)`, fmt.Sprint("row", i))
	}
	var n int
	if err := db.QueryRow(`select count(*) from t`).Scan(&n); err != nil || n != 200 {
		t.Fatalf("count=%d err=%v", n, err)
	}
	if _, err := db.Exec(`select * from t order by v desc`); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	_, ok := m.files["mem.db"]
	m.mu.Unlock()
	if !ok {
		t.Fatal("database not stored in the in-memory VFS")
	}

	// Data survives reconnecting.
	db.Close()
	db2 := openDB(t, "mem", "mem.db", "")
	if err := db2.QueryRow(`select count(*) from t`).Scan(&n); err != nil || n != 200 {
		t.Fatalf("after reopen count=%d err=%v", n, err)
	}

	// mmap is ignored safely.
	db2.QueryRow(`pragma mmap_size=1048576`).Scan(&n)
	if err := db2.QueryRow(`select count(*) from t`).Scan(&n); err != nil || n != 200 {
		t.Fatalf("after mmap pragma count=%d err=%v", n, err)
	}
}

func TestWALRefusedCleanly(t *testing.T) {
	if err := vfs.Register("mem-wal", newMemVFS()); err != nil {
		t.Fatal(err)
	}
	defer vfs.Unregister("mem-wal")
	db := openDB(t, "mem-wal", "w.db", "")
	mustExec(t, db, `create table t(v)`)
	var mode string
	if err := db.QueryRow(`pragma journal_mode=wal`).Scan(&mode); err == nil && mode == "wal" {
		t.Fatal("WAL must not be enabled on a VFS without shared memory")
	}
	mustExec(t, db, `insert into t values (1)`)
}

func TestFaultInjection(t *testing.T) {
	m := newMemVFS()
	vfs.Register("mem-fault", m)
	defer vfs.Unregister("mem-fault")
	db := openDB(t, "mem-fault", "f.db", "")
	mustExec(t, db, `create table t(v)`)
	m.failWrites = true
	if _, err := db.Exec(`insert into t values (1)`); err == nil {
		t.Fatal("expected insert to fail")
	}
	m.failWrites = false
	mustExec(t, db, `insert into t values (2)`)
}

type uriVFS struct{ *memVFS }

func (v uriVFS) OpenURI(name string, uri vfscore.URI, fl vfscore.OpenFlag) (vfscore.File, vfscore.OpenFlag, error) {
	if uri.Parameter("deny") == "1" {
		return nil, 0, &vfscore.Error{Code: vfscore.ErrPerm}
	}
	return v.memVFS.Open(name, fl)
}

func TestURIOpener(t *testing.T) {
	vfs.Register("mem-uri", uriVFS{newMemVFS()})
	defer vfs.Unregister("mem-uri")
	db := openDB(t, "mem-uri", "u.db", "")
	mustExec(t, db, `create table t(v)`)
	db2 := openDB(t, "mem-uri", "u2.db", "&deny=1")
	if _, err := db2.Exec(`create table t(v)`); err == nil {
		t.Fatal("expected open to be denied")
	}
}

type clockVFS struct{ *memVFS }

func (clockVFS) CurrentTime() (time.Time, error) {
	return time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC), nil
}

func TestClock(t *testing.T) {
	vfs.Register("mem-clock", clockVFS{newMemVFS()})
	defer vfs.Unregister("mem-clock")
	db := openDB(t, "mem-clock", "c.db", "")
	var s string
	if err := db.QueryRow(`select strftime('%Y', 'now')`).Scan(&s); err != nil || s != "2001" {
		t.Errorf("year=%q err=%v", s, err)
	}
}

type panicVFS struct{ *memVFS }

func (panicVFS) Open(string, vfscore.OpenFlag) (vfs.File, vfscore.OpenFlag, error) { panic("boom") }

func TestPanicIsContained(t *testing.T) {
	vfs.Register("mem-panic", panicVFS{newMemVFS()})
	defer vfs.Unregister("mem-panic")
	db := openDB(t, "mem-panic", "p.db", "")
	if _, err := db.Exec(`create table t(v)`); err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterErrors(t *testing.T) {
	if err := vfs.Register("unix", newMemVFS()); err == nil {
		t.Error("expected error for duplicate name")
	}
	var e *vfscore.Error
	if err := vfs.Unregister("doesnotexist"); !errors.As(err, &e) {
		t.Errorf("unexpected: %v", err)
	}
}

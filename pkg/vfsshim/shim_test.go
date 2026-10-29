package vfsshim_test

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emo/sqlite3vfsshim/pkg/vfscore"

	shim "github.com/emo/sqlite3vfsshim/pkg/vfsshim"
	_ "github.com/mattn/go-sqlite3"
)

type counters struct {
	opens, reads, writes, syncs, locks, deletes, fullpaths, randoms int64
	failWrites                                                      atomic.Bool
}

type countingShim struct {
	c *counters
}

func (s countingShim) Open(name string, fl vfscore.OpenFlag) (shim.File, error) {
	atomic.AddInt64(&s.c.opens, 1)
	return &countingFile{c: s.c}, nil
}
func (s countingShim) Delete(n string, d bool) error {
	atomic.AddInt64(&s.c.deletes, 1)
	return nil
}
func (s countingShim) FullPathname(name string) error {
	atomic.AddInt64(&s.c.fullpaths, 1)
	return nil
}
func (s countingShim) Randomness(n int) {
	atomic.AddInt64(&s.c.randoms, 1)
}

type countingFile struct {
	c *counters
}

func (f *countingFile) ReadAt(p []byte, o int64) error {
	atomic.AddInt64(&f.c.reads, 1)
	return nil
}
func (f *countingFile) WriteAt(p []byte, o int64) error {
	atomic.AddInt64(&f.c.writes, 1)
	if f.c.failWrites.Load() {
		return &vfscore.Error{Code: vfscore.ErrFull} // veto: the parent never writes
	}
	return nil
}
func (f *countingFile) Sync(fl vfscore.SyncType) error {
	atomic.AddInt64(&f.c.syncs, 1)
	return nil
}
func (f *countingFile) Lock(l vfscore.LockType) error {
	atomic.AddInt64(&f.c.locks, 1)
	return nil
}

func openDB(t *testing.T, vfs, file string, q string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?vfs=%s%s", file, vfs, q))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCountingShim(t *testing.T) {
	c := &counters{}
	if err := shim.Register("counting", countingShim{c: c}, shim.WithParent("unix")); err != nil {
		t.Fatal(err)
	}
	defer shim.Unregister("counting")

	db := openDB(t, "counting", filepath.Join(t.TempDir(), "a.db"), "")
	mustExec(t, db, `create table t(id integer primary key, v text)`)
	for i := 0; i < 100; i++ {
		mustExec(t, db, `insert into t(v) values (?)`, fmt.Sprint("row", i))
	}
	var n int
	if err := db.QueryRow(`select count(*) from t`).Scan(&n); err != nil || n != 100 {
		t.Fatalf("count=%d err=%v", n, err)
	}
	for name, v := range map[string]int64{
		"opens": c.opens, "reads": c.reads, "writes": c.writes, "syncs": c.syncs,
		"locks": c.locks, "fullpaths": c.fullpaths, "deletes": c.deletes,
	} {
		if v == 0 {
			t.Errorf("expected %s > 0", name)
		}
	}
}

func TestFaultInjection(t *testing.T) {
	c := &counters{}
	shim.Register("faulty", countingShim{c: c}, shim.WithParent("unix"))
	defer shim.Unregister("faulty")

	db := openDB(t, "faulty", filepath.Join(t.TempDir(), "b.db"), "")
	mustExec(t, db, `create table t(v)`)
	c.failWrites.Store(true)
	_, err := db.Exec(`insert into t values (1)`)
	if err == nil {
		t.Fatal("expected insert to fail")
	}
	t.Log("got expected error:", err)
	c.failWrites.Store(false)
	mustExec(t, db, `insert into t values (2)`)
}

func TestWAL(t *testing.T) {
	c := &counters{}
	shim.Register("walshim", countingShim{c: c}, shim.WithParent("unix"))
	defer shim.Unregister("walshim")

	db := openDB(t, "walshim", filepath.Join(t.TempDir(), "w.db"), "")
	var mode string
	if err := db.QueryRow(`pragma journal_mode=wal`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("mode=%q err=%v", mode, err)
	}
	mustExec(t, db, `create table t(v)`)
	mustExec(t, db, `insert into t values (1)`)
	if c.writes == 0 {
		t.Error("expected WAL writes to be seen by shim")
	}
}

func TestStacking(t *testing.T) {
	inner, outer := &counters{}, &counters{}
	shim.Register("inner", countingShim{c: inner}, shim.WithParent("unix"))
	shim.Register("outer", countingShim{c: outer}, shim.WithParent("inner"))
	defer shim.Unregister("outer")
	defer shim.Unregister("inner")

	db := openDB(t, "outer", filepath.Join(t.TempDir(), "s.db"), "")
	mustExec(t, db, `create table t(v)`)
	if inner.writes == 0 || inner.writes != outer.writes {
		t.Errorf("inner=%d outer=%d writes", inner.writes, outer.writes)
	}
}

type denyShim struct{}

func (denyShim) OpenURI(name string, uri vfscore.URI, fl vfscore.OpenFlag) (vfscore.File, vfscore.OpenFlag, error) {
	if uri.Parameter("deny") == "1" {
		return nil, fl, &vfscore.Error{Code: vfscore.ErrPerm}
	}
	return nil, fl, nil
}

func TestURIParameter(t *testing.T) {
	shim.Register("uri", denyShim{}, shim.WithParent("unix"))
	defer shim.Unregister("uri")
	dir := t.TempDir()

	db := openDB(t, "uri", filepath.Join(dir, "u.db"), "")
	mustExec(t, db, `create table t(v)`)

	db2 := openDB(t, "uri", filepath.Join(dir, "u2.db"), "&deny=1")
	if _, err := db2.Exec(`create table t(v)`); err == nil {
		t.Fatal("expected open to be denied")
	}
}

func TestPanicIsContained(t *testing.T) {
	shim.Register("panicky", panicShim{}, shim.WithParent("unix"))
	defer shim.Unregister("panicky")
	db := openDB(t, "panicky", filepath.Join(t.TempDir(), "p.db"), "")
	if _, err := db.Exec(`create table t(v)`); err == nil {
		t.Fatal("expected error")
	}
}

type panicShim struct{}

func (panicShim) Open(string, vfscore.OpenFlag) (shim.File, error) {
	panic("boom")
}

func TestRegisterErrors(t *testing.T) {
	if err := shim.Register("x", struct{}{}, shim.WithParent("nope")); err == nil {
		t.Error("expected error for unknown parent")
	}
	if err := shim.Register("unix", struct{}{}); err == nil {
		t.Error("expected error for duplicate name")
	}
	var e *vfscore.Error
	if err := shim.Unregister("doesnotexist"); !errors.As(err, &e) {
		t.Errorf("unexpected: %v", err)
	}
}

func TestTimeAndRandom(t *testing.T) {
	var calls atomic.Int64
	shim.Register("clock", clockShim{&calls}, shim.WithParent("unix"))
	defer shim.Unregister("clock")
	db := openDB(t, "clock", filepath.Join(t.TempDir(), "c.db"), "")
	var s string
	if err := db.QueryRow(`select strftime('%Y', 'now')`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	if calls.Load() == 0 {
		t.Error("CurrentTime hook was not called")
	}
	// The parent still supplies the actual time.
	if s != fmt.Sprint(time.Now().Year()) {
		t.Errorf("year=%s", s)
	}
}

type clockShim struct{ calls *atomic.Int64 }

func (c clockShim) CurrentTime() error {
	c.calls.Add(1)
	return nil
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// An empty shim is a pure passthrough handled entirely in C, WAL included.
func TestEmptyShimIsPassthrough(t *testing.T) {
	if err := shim.Register("empty", struct{}{}, shim.WithParent("unix")); err != nil {
		t.Fatal(err)
	}
	defer shim.Unregister("empty")
	db := openDB(t, "empty", filepath.Join(t.TempDir(), "e.db"), "")
	var mode string
	if err := db.QueryRow(`pragma journal_mode=wal`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("mode=%q err=%v", mode, err)
	}
	mustExec(t, db, `create table t(v)`)
	mustExec(t, db, `insert into t values (1)`)
}

// A File implementing only some hooks sees only those calls; the other calls
// go straight to the parent in C.
func TestOnlyImplementedHooksEnterGo(t *testing.T) {
	c := &counters{}
	if err := shim.Register("partial", partialShim{c}, shim.WithParent("unix")); err != nil {
		t.Fatal(err)
	}
	defer shim.Unregister("partial")
	db := openDB(t, "partial", filepath.Join(t.TempDir(), "p.db"), "")
	mustExec(t, db, `create table t(v)`)
	mustExec(t, db, `insert into t values (1)`)
	var n int
	if err := db.QueryRow(`select count(*) from t`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if c.opens == 0 || c.reads == 0 {
		t.Errorf("opens=%d reads=%d", c.opens, c.reads)
	}
	if c.writes != 0 || c.syncs != 0 {
		t.Errorf("unimplemented hooks ran: writes=%d syncs=%d", c.writes, c.syncs)
	}
}

type partialShim struct{ c *counters }

func (s partialShim) Open(name string, fl vfscore.OpenFlag) (shim.File, error) {
	atomic.AddInt64(&s.c.opens, 1)
	return &readOnlyHook{s.c}, nil
}

type readOnlyHook struct{ c *counters }

func (f *readOnlyHook) ReadAt(p []byte, o int64) error {
	atomic.AddInt64(&f.c.reads, 1)
	return nil
}

// A veto from OpenHook fails the open and the parent is never asked.
func TestOpenVeto(t *testing.T) {
	shim.Register("veto", vetoShim{}, shim.WithParent("unix"))
	defer shim.Unregister("veto")
	path := filepath.Join(t.TempDir(), "v.db")
	db := openDB(t, "veto", path, "")
	if _, err := db.Exec(`create table t(v)`); err == nil {
		t.Fatal("expected open to be vetoed")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("parent created the file despite the veto")
	}
}

type vetoShim struct{}

func (vetoShim) Open(string, vfscore.OpenFlag) (shim.File, error) {
	return nil, &vfscore.Error{Code: vfscore.ErrPerm}
}

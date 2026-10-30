// Command trace demonstrates a shim VFS that logs every file operation of the
// VFS it wraps, similar in spirit to SQLite's vfstrace.
package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	"github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore"

	_ "github.com/mattn/go-sqlite3"
	shim "github.com/sqlited-org/sqlite3vfsshim/pkg/vfsshim"
)

// traceShim implements only the Open hook; traceFile implements the file hooks
// it wants to log. After each hook returns nil, C performs the real call on the
// parent VFS ("unix"), and calls without a hook go straight to the parent.
type traceShim struct{}

func (traceShim) Open(name string, fl vfscore.OpenFlag) (any, error) {
	log.Printf("open %q flags=%#x", name, int(fl))
	return &traceFile{name}, nil
}

type traceFile struct{ name string }

func (f *traceFile) ReadAt(p []byte, off int64) error {
	log.Printf("  read  %s off=%d len=%d", filepath.Base(f.name), off, len(p))
	return nil
}
func (f *traceFile) WriteAt(p []byte, off int64) error {
	log.Printf("  write %s off=%d len=%d", filepath.Base(f.name), off, len(p))
	return nil
}
func (f *traceFile) Sync(fl vfscore.SyncType) error {
	log.Printf("  sync  %s", filepath.Base(f.name))
	return nil
}
func (f *traceFile) Lock(l vfscore.LockType) error {
	log.Printf("  lock  %s %v", filepath.Base(f.name), l)
	return nil
}

func main() {
	if err := shim.Register("trace", traceShim{}, shim.WithParent("unix")); err != nil {
		log.Fatal(err)
	}
	dir, _ := os.MkdirTemp("", "trace")
	defer os.RemoveAll(dir)
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, "t.db")+"?vfs=trace")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`create table t(v); insert into t values (1)`); err != nil {
		log.Fatal(err)
	}
}

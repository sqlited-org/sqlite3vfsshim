//go:build SQLITE3VFSSHIM_LOADABLE_EXT

package main

import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore"

	shim "github.com/sqlited-org/sqlite3vfsshim/pkg/vfsshim"
)

// sqliteOKLoadPermanently tells SQLite never to unload this library (it
// contains the Go runtime, which cannot be unloaded safely).
const sqliteOKLoadPermanently = 256

// sqlite3_traceshim_init is the extension entry point. SQLite derives its
// name from the file name: traceshim.so -> sqlite3_traceshim_init.
//
//export sqlite3_traceshim_init
func sqlite3_traceshim_init(db, pzErrMsg, pApi unsafe.Pointer) C.int {
	shim.InitExtension(pApi)                                        // must come first: wires up the SQLite API table
	if err := shim.Register("traceshim", traceShim{}); err != nil { // wraps the default VFS
		fmt.Fprintln(os.Stderr, "traceshim: register:", err)
		return 1 // SQLITE_ERROR
	}
	return sqliteOKLoadPermanently
}

// traceShim logs file operations of the VFS it wraps to stderr. Its hooks run
// first; then C performs the real call on the parent VFS.
type traceShim struct{}

func (traceShim) Open(name string, fl vfscore.OpenFlag) (shim.File, error) {
	fmt.Fprintf(os.Stderr, "traceshim: open %q flags=%#x\n", name, int(fl))
	return &traceFile{filepath.Base(name)}, nil
}

type traceFile struct{ name string }

func (f *traceFile) WriteAt(p []byte, off int64) error {
	fmt.Fprintf(os.Stderr, "traceshim: write %s off=%d len=%d\n", f.name, off, len(p))
	return nil
}

func (f *traceFile) Sync(fl vfscore.SyncType) error {
	fmt.Fprintf(os.Stderr, "traceshim: sync %s\n", f.name)
	return nil
}

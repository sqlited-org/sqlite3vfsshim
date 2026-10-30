//go:build SQLITE3VFS_LOADABLE_EXT

package main

import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/sqlited-org/sqlite3vfsshim/examples/internal/memvfs"
	"github.com/sqlited-org/sqlite3vfsshim/pkg/vfs"
)

// sqliteOKLoadPermanently tells SQLite never to unload this library (it
// contains the Go runtime, which cannot be unloaded safely).
const sqliteOKLoadPermanently = 256

// sqlite3_memvfs_init is the extension entry point. SQLite derives its name
// from the file name: memvfs.so -> sqlite3_memvfs_init.
//
//export sqlite3_memvfs_init
func sqlite3_memvfs_init(db, pzErrMsg, pApi unsafe.Pointer) C.int {
	vfs.InitExtension(pApi) // must come first: wires up the SQLite API table
	if err := vfs.Register("memvfs", memvfs.New()); err != nil {
		fmt.Fprintln(os.Stderr, "memvfs: register:", err)
		return 1 // SQLITE_ERROR
	}
	return sqliteOKLoadPermanently
}

//go:build SQLITE3VFS_LOADABLE_EXT

package vfs

/*
   #cgo CFLAGS: -DSQLITE3VFS_LOADABLE_EXT=1
*/
import "C"

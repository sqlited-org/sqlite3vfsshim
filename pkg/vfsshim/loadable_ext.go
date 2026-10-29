//go:build SQLITE3VFSSHIM_LOADABLE_EXT

package vfsshim

/*
   #cgo CFLAGS: -DSQLITE3VFSSHIM_LOADABLE_EXT=1
*/
import "C"

package vfscore

/*
#cgo CFLAGS: -I./include
#include <stdlib.h>

#ifdef SQLITE3VFSCORE_LOADABLE_EXT
#include "sqlite3ext.h"
#else
#include "sqlite3-binding.h"
#endif

static const char *sqlite3VFS_URIParameter(const char *zName, const char *zKey) { return sqlite3_uri_parameter(zName, zKey); }
*/
import "C"

import (
	"unsafe"
)

// URI gives access to the URI query parameters of a file being opened. It is
// only valid for the duration of the OpenURI call.
type URI struct{ name *C.char }

func NewURI(pname unsafe.Pointer) URI {
	return URI{(*C.char)(pname)}
}

// Parameter returns the value of the named URI parameter, or "" if absent.
func (u URI) Parameter(key string) string {
	if u.name == nil {
		return ""
	}

	ck := C.CString(key)
	defer C.free(unsafe.Pointer(ck))

	v := C.sqlite3VFS_URIParameter(u.name, ck)
	if v == nil {
		return ""
	}
	return C.GoString(v)
}

package vfscore_test

import (
	"testing"
	"unsafe"

	_ "github.com/mattn/go-sqlite3" // provides sqlite3_uri_parameter

	"github.com/emo/sqlite3vfsshim/pkg/vfscore"
)

// SQLite hands a VFS a filename laid out as "name\0key\0value\0...\0\0",
// preceded by four zero bytes (newer SQLite walks backwards to find the start).
func TestURIParameter(t *testing.T) {
	buf := []byte("\x00\x00\x00\x00main.db\x00mode\x00ro\x00cache\x00shared\x00empty\x00\x00\x00")
	u := vfscore.NewURI(unsafe.Pointer(&buf[4]))

	for key, want := range map[string]string{
		"mode": "ro", "cache": "shared", "empty": "", "missing": "",
	} {
		if got := u.Parameter(key); got != want {
			t.Errorf("Parameter(%q) = %q, want %q", key, got, want)
		}
	}
	if got := (vfscore.URI{}).Parameter("mode"); got != "" {
		t.Errorf("zero URI returned %q", got)
	}
}

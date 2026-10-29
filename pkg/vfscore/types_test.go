package vfscore

import (
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

func TestErrorCodeRoundTrip(t *testing.T) {
	if ErrFromCode(0) != nil {
		t.Fatal("code 0 must map to nil error")
	}
	var e *Error
	if !errors.As(ErrFromCode(int(ErrBusy)), &e) || e.Code != ErrBusy {
		t.Fatalf("got %v", e)
	}
	if CodeFromErr(nil) != 0 {
		t.Fatal("nil must be 0")
	}
	if got := CodeFromErr(fmt.Errorf("wrap: %w", &Error{Code: ErrFull})); got != int(ErrFull) {
		t.Fatalf("wrapped code = %d", got)
	}
	if got := CodeFromErr(errors.New("plain")); got != int(ErrIO) {
		t.Fatalf("plain error code = %d, want SQLITE_IOERR", got)
	}
}

func TestReadResultCode(t *testing.T) {
	p := []byte{1, 2, 3, 4}
	if ReadResultCode(p, 4, nil) != 0 {
		t.Fatal("success must be 0")
	}
	if rc := ReadResultCode(p, 2, io.EOF); rc != int(ErrIOShortRead) {
		t.Fatalf("rc = %d", rc)
	}
	if p[0] != 1 || p[1] != 2 || p[2] != 0 || p[3] != 0 {
		t.Fatalf("tail not zero-filled: %v", p)
	}
	if rc := ReadResultCode(p, 0, &Error{Code: ErrIORead}); rc != int(ErrIORead) {
		t.Fatalf("rc = %d", rc)
	}
}

func TestJulianRoundTrip(t *testing.T) {
	now := time.UnixMilli(1700000000123)
	if got := TimeFromJulianMs(JulianMs(now)); !got.Equal(now) {
		t.Fatalf("got %v want %v", got, now)
	}
	// 2000-01-01 12:00 UTC is Julian day 2451545.0.
	if got := JulianMs(time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)); got != 2451545*86400000 {
		t.Fatalf("got %d", got)
	}
}

func TestCopyCString(t *testing.T) {
	buf := make([]byte, 4)
	if !CopyCString(buf, "abc") || string(buf[:3]) != "abc" || buf[3] != 0 {
		t.Fatalf("buf = %v", buf)
	}
	if CopyCString(buf, "abcd") {
		t.Fatal("must fail when NUL does not fit")
	}
}

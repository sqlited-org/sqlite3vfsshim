# vfsshim

A Go API for writing SQLite **VFS shims**: VFSes that stack on
top of an existing VFS (e.g. `unix`) and intercept calls, as described in the
[SQLite VFS docs](https://www.sqlite.org/vfs.html). It is the sibling of
[psanford/sqlite3vfs](https://github.com/psanford/sqlite3vfs), but where that
project implements a VFS from scratch, here you only override what you need and
everything else is delegated to the parent. The parent VFS lives only in the C
code; Go supplies its name and optional hooks. To implement a complete VFS with no
parent, use the sibling library [pkg/vfs](../vfs/README.md).

Install: `go get github.com/sqlited-org/sqlite3vfsshim/pkg/vfsshim`. Works with [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3).

## Usage

Go never touches the parent VFS. You implement **hooks**; for every call SQLite
makes, the C code (`shim.c`) runs your hook (if you have one), and then, unless
the hook returned an error, calls the parent VFS itself.

```go
type myShim struct{}

// Runs before the parent opens the file. The returned value holds the hooks
// of this open file (it may be nil).
func (myShim) Open(name string, fl vfscore.OpenFlag) (shim.File, error) {
	return &myFile{name}, nil
}

type myFile struct{ name string }

// Runs before the parent writes. Returning an error vetoes the write.
func (f *myFile) WriteAt(p []byte, off int64) error {
	log.Printf("write %s off=%d len=%d", f.name, off, len(p))
	return nil
}

shim.Register("myvfs", myShim{}, shim.WithParent("unix")) // "unix" is only a name
db, _ := sql.Open("sqlite3", "file:test.db?vfs=myvfs")
```

### Hooks

`Shim` and `File` are empty interfaces; implement only what you need. A call
with no hook never enters Go, and an empty `struct{}` is a pure passthrough.

| Level | Hook interfaces |
|-------|-----------------|
| VFS Shim (the value given to `Register`) | `OpenHook`, `DeleteHook`, `AccessHook`, `FullPathnameHook`, `RandomnessHook`, `SleepHook`, `CurrentTimeHook` (or `vfscore.URIOpener` instead of `OpenHook`) |
| File (the value returned by `Open`) | `CloseHook`, `ReadHook`, `WriteHook`, `TruncateHook`, `SyncHook`, `FileSizeHook`, `LockHook`, `UnlockHook`, `CheckReservedLockHook`, `FileControlHook` |

The hook methods return only an `error` (no results), for example
`Delete(name string, dirSync bool) error`, `ReadAt(p []byte, off int64) error`,
`Randomness(n int)`, `Sleep(d time.Duration)` and `CurrentTime() error`.
See [shim.go](shim.go) for every signature.

Semantics:

- A hook **observes or vetoes**: a non-nil error (e.g. `&vfscore.Error{Code:
  vfscore.ErrFull}`) is returned to SQLite and the parent is not called.
  `Randomness` and `Sleep` cannot fail. `Close` errors are reported
  but the parent file is always closed.
- Hooks run before the parent call, so they cannot change a result (e.g. a
  read's data or the parent's time). To produce your own results, implement a
  full VFS with [pkg/vfs](../vfs/README.md).
- `OpenHook.Open(name, flags)` has no URI argument. A shim that needs the URI
  parameters implements [`vfscore.URIOpener`](../vfscore/types.go) instead,
  `OpenURI(name, uri, flags) (vfscore.File, vfscore.OpenFlag, error)`, which
  takes precedence over `Open` (`uri.Parameter("key")`, valid only during the
  call). Its returned flags are ignored and a nil file means no file hooks.
- Shared memory, mmap, dynamic loading, sector size, device characteristics
  and system calls are always forwarded to the parent in C, so WAL works.

### Loadable SQLite extension

A shim can also be built as a SQLite loadable extension, so it can be loaded
at runtime into the `sqlite3` CLI or any application that supports extensions.
Build with `-tags SQLITE3VFSSHIM_LOADABLE_EXT -buildmode=c-shared`. With that
tag the C code reaches SQLite through the `sqlite3_api_routines` table
(`sqlite3ext.h`) instead of linking against it, so the `.so` has no undefined
SQLite symbols.

The extension needs a package `main` that exports the entry point
`sqlite3_<filename>_init` and calls `shim.InitExtension` first:

```go
//export sqlite3_traceshim_init
func sqlite3_traceshim_init(db, pzErrMsg, pApi unsafe.Pointer) C.int {
	shim.InitExtension(pApi)
	if err := shim.Register("traceshim", traceShim{}); err != nil {
		return 1 // SQLITE_ERROR
	}
	return 256 // SQLITE_OK_LOAD_PERMANENTLY: the library contains the Go runtime
}
```

Full example: [examples/traceshim-ext](../../examples/traceshim-ext/ext.go).

```
$ cd ../../examples/traceshim-ext && make
$ sqlite3
sqlite> .load ./traceshim
sqlite> .open file:test.db?vfs=traceshim
sqlite> create table t(v);
traceshim: open "/.../test.db" flags=0x146
...
```

Notes: the package that contains the `main` function must not import
`mattn/go-sqlite3` (it would link a second SQLite), and two Go runtimes in one
process are not supported, so don't load the extension into a Go program.
[loadable_ext_test.go](loadable_ext_test.go) builds the example and loads it in
the `sqlite3` CLI (skipped if the CLI is missing).

See [examples/trace](../../examples/trace/main.go) and [shim_test.go](shim_test.go)
(counting, veto/fault injection, WAL, stacking, URI parameters, clock hook).

## Design

- `shim.c`: a C `sqlite3_vfs` + `sqlite3_io_methods`. It looks up and stores the
  parent VFS, and every parent call is made there. Each `sqlite3_file` is
  `[shimFile | parent's sqlite3_file]` in one allocation (`szOsFile` is the sum).
- `vfsshimcgo.go`: only the exported `goShim*` hook entry points, hook-mask detection
  and `Register`/`Unregister`.
- Per method, C calls the Go hook if the shim/file implements it (bits set at
  `Register` / `Open` time), then the parent. See the call-flow comment at the
  top of `shim.c`.
- Errors: return `*vfscore.Error{Code}` to pass an exact SQLite code (e.g.
  `vfscore.ErrBusy`); other errors become `SQLITE_IOERR`. Panics in hooks
  become `SQLITE_IOERR`.
- Shims can be stacked: a shim's parent may itself be a shim.

## Limitations

- Hooks can observe or veto, not replace results.
- `Unregister` must not be called while connections using the shim are open.
- Always needs a parent VFS; for a VFS with no parent use [pkg/vfs](../vfs/README.md).

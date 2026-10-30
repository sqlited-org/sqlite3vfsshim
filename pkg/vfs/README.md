# vfs (`github.com/sqlited-org/sqlite3vfsshim/pkg/vfs`)

Implement a complete SQLite **VFS** in Go, in the spirit of
[psanford/sqlite3vfs](https://github.com/psanford/sqlite3vfs). The VFS replaces
SQLite's file layer entirely; there is no parent VFS. To wrap an existing VFS and
override only some calls, use the sibling library
[pkg/vfsshim](../vfsshim/README.md).

Install: `go get github.com/sqlited-org/sqlite3vfsshim/pkg/vfs`. Works with
[mattn/go-sqlite3](https://github.com/mattn/go-sqlite3).

## Usage

```go
type myVFS struct{}                                   // implements vfs.VFS
func (myVFS) Open(name string, fl vfscore.OpenFlag) (vfscore.File, vfscore.OpenFlag, error) { ... }
func (myVFS) Delete(name string, dirSync bool) error                            { ... }
func (myVFS) Access(name string, fl vfscore.AccessFlag) (bool, error)               { ... }
func (myVFS) FullPathname(name string) (string, error)                          { ... }

vfs.Register("myvfs", myVFS{})
db, _ := sql.Open("sqlite3", "file:test.db?vfs=myvfs")
```

`Open` returns a `vfscore.File` (`ReadAt`, `WriteAt`, `Sync`, `Lock`, ...).

Optional interfaces on your VFS, with standard-library defaults when absent:

| Interface | Purpose |
|---|---|
| `vfs.Randomizer` | `Randomness(p []byte) int` |
| `vfs.Sleeper` | `Sleep(d) time.Duration` |
| `vfs.Clock` | `CurrentTime() (time.Time, error)` (controls `datetime('now')`) |
| `vfscore.URIOpener` | `OpenURI(name, uri, flags)` to read URI query parameters |

Options: `vfs.WithMaxPathname(n)`, `vfs.MakeDefault()`.

## Errors

Return `*vfscore.Error{Code}` to hand SQLite an exact result code (e.g. `vfscore.ErrBusy`);
any other error becomes `SQLITE_IOERR`. `io.EOF` from `ReadAt` is a short read
(remainder zero-filled). Panics in your code become `SQLITE_IOERR`.

## Examples

- [examples/memvfs](../../examples/memvfs/main.go): in-memory VFS
  ([implementation](../../examples/internal/memvfs/memvfs.go)).
- [examples/memvfs-ext](../../examples/memvfs-ext/ext.go): the same VFS as a
  loadable SQLite extension.

## Loadable SQLite extension

Build with `-tags SQLITE3VFS_LOADABLE_EXT -buildmode=c-shared`. The C code then
reaches SQLite through `sqlite3_api_routines` (`sqlite3ext.h`), so the `.so` has no
undefined SQLite symbols. Export `sqlite3_<filename>_init`, call
`vfs.InitExtension(pApi)` first, then `vfs.Register`, and return `256`
(`SQLITE_OK_LOAD_PERMANENTLY`: the library contains the Go runtime):

```
$ cd examples/memvfs-ext && make
$ sqlite3 -cmd '.load ./memvfs' -cmd '.open file:test.db?vfs=memvfs'
```

The package with `main` must not import `mattn/go-sqlite3`, and the extension
cannot be loaded into a Go program (two Go runtimes). See
[loadable_ext_test.go](loadable_ext_test.go).

## Limitations

- No shared memory / mmap, so WAL mode is refused cleanly.
- No dynamic extension loading from inside the VFS.
- `Unregister` must not be called while connections using the VFS are open.

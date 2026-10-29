# sqlite3vfsshim

Go APIs for SQLite VFSes, as two independent Go modules (use both together via
the `go.work` workspace at the repository root):

| Module | Import path | What it is |
|---|---|---|
| [pkg/vfs](pkg/vfs/README.md) | `github.com/emo/sqlite3vfsshim/pkg/vfs` | Implement a **complete, standard VFS** in Go (like [psanford/sqlite3vfs](https://github.com/psanford/sqlite3vfs)). No parent VFS. |
| [pkg/vfsshim](pkg/vfsshim/README.md) | `github.com/emo/sqlite3vfsshim/pkg/vfsshim` | Write a **VFS shim** that stacks on an existing VFS (`unix`, another shim, ...) and hooks only the calls you need (observe/veto); the parent is called from C, so WAL works unchanged. |
| [pkg/vfscore](pkg/vfscore/README.md) | `github.com/emo/sqlite3vfsshim/pkg/vfscore` | Pure-Go types shared by both libraries (errors, flags, `File`, helpers). Import it for `vfscore.OpenFlag`, `vfscore.File`, etc. |
| [examples](examples) | `github.com/emo/sqlite3vfsshim/examples` | Runnable examples (own module, depends on both). |

Both are cgo libraries with no dependency on each other (the shared flag/error types
and `URI` live in `pkg/vfscore`), both work with
[mattn/go-sqlite3](https://github.com/mattn/go-sqlite3), and both can be built as
SQLite loadable extensions (`-tags SQLITE3VFS_LOADABLE_EXT` /
`SQLITE3VFSSHIM_LOADABLE_EXT`, see each README).

Which one?

- Storing the database somewhere else (memory, object store, encrypted blob...):
  **vfs**.
- Observing, modifying or failing operations of a normal file-based database
  (tracing, fault injection, encryption of pages, clock override...): **vfsshim**.

## Examples

| Example | Library | Shows |
|---|---|---|
| [memvfs](examples/memvfs/main.go) | vfs | in-memory database |
| [memvfs-ext](examples/memvfs-ext/ext.go) | vfs | the same as a loadable extension |
| [trace](examples/trace/main.go) | vfsshim | logging every file operation |
| [traceshim-ext](examples/traceshim-ext/ext.go) | vfsshim | the trace shim as a loadable extension |

## Development

```
go test ./pkg/vfscore ./pkg/vfs ./pkg/vfsshim   # from the repository root (workspace mode)
go run ./examples/memvfs
```

The loadable-extension tests build the example extensions and load them into the
`sqlite3` CLI (skipped when the CLI is not installed).

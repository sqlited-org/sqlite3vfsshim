# vfscore

Pure-Go types shared by [`vfs`](../vfs) and [`vfsshim`](../vfsshim): a small cgo part for URI parameters, otherwise no dependencies.

| Area | Identifiers |
|------|-------------|
| Errors | `ErrorCode`, `Err*`, `Error`, `ErrFromCode`, `CodeFromErr` |
| Flags | `OpenFlag`, `AccessFlag`, `LockType`, `SyncType`, `DeviceCharacteristic` and their constants |
| File | the `File` interface implemented by both libraries |
| URI | `URI`, `NewURI`, `URI.Parameter`, and the `URIOpener` interface (`OpenURI(name, uri, flags)`) |
| Helpers | `ReadResultCode`, `JulianMs`, `TimeFromJulianMs`, `CopyCString` |

Import it alongside either library:

```go
import (
    "github.com/emo/sqlite3vfsshim/pkg/vfs"      // or pkg/vfsshim
    "github.com/emo/sqlite3vfsshim/pkg/vfscore"
)
```

It is a separate module (not imported from one cgo library by the other) so a
program or loadable extension links only one C layer.

`URI.Parameter` calls `sqlite3_uri_parameter`, which `vfscore` only declares
(via the shared headers). The symbol comes from go-sqlite3 in the final binary,
so the tests ([uri_test.go](uri_test.go)) blank-import `mattn/go-sqlite3`.

## Shared SQLite headers

[`include/`](include) holds the single copy of `sqlite3-binding.h` and
`sqlite3ext.h` (from go-sqlite3 v1.14.52). `vfs` and `vfsshim` find them with
`#cgo CFLAGS: -I${SRCDIR}/../vfscore/include`, so the three modules must sit
side by side (this repository layout, or a `replace`/`go.work` checkout).

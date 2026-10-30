module github.com/sqlited-org/sqlite3vfsshim/examples

go 1.22

require (
	github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore v0.0.0
	github.com/sqlited-org/sqlite3vfsshim/pkg/vfs v0.0.0
	github.com/sqlited-org/sqlite3vfsshim/pkg/vfsshim v0.0.0
	github.com/mattn/go-sqlite3 v1.14.52
)

replace (
	github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore => ../pkg/vfscore
	github.com/sqlited-org/sqlite3vfsshim/pkg/vfs => ../pkg/vfs
	github.com/sqlited-org/sqlite3vfsshim/pkg/vfsshim => ../pkg/vfsshim
)

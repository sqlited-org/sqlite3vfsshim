module github.com/sqlited-org/sqlite3vfsshim/pkg/vfsshim

go 1.21

require github.com/mattn/go-sqlite3 v1.14.52

require github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore v0.0.0

replace github.com/sqlited-org/sqlite3vfsshim/pkg/vfscore => ../vfscore

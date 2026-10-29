module github.com/emo/sqlite3vfsshim/examples

go 1.22

require (
	github.com/emo/sqlite3vfsshim/pkg/vfscore v0.0.0
	github.com/emo/sqlite3vfsshim/pkg/vfs v0.0.0
	github.com/emo/sqlite3vfsshim/pkg/vfsshim v0.0.0
	github.com/mattn/go-sqlite3 v1.14.52
)

replace (
	github.com/emo/sqlite3vfsshim/pkg/vfscore => ../pkg/vfscore
	github.com/emo/sqlite3vfsshim/pkg/vfs => ../pkg/vfs
	github.com/emo/sqlite3vfsshim/pkg/vfsshim => ../pkg/vfsshim
)

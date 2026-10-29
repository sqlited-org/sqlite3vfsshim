module github.com/emo/sqlite3vfsshim/pkg/vfs

go 1.21

require github.com/mattn/go-sqlite3 v1.14.52

require github.com/emo/sqlite3vfsshim/pkg/vfscore v0.0.0

replace github.com/emo/sqlite3vfsshim/pkg/vfscore => ../vfscore

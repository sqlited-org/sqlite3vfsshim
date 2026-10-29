# Build the loadable extension:  make   (produces memvfs.so)
# Use it:  sqlite3 -cmd '.load ./memvfs' -cmd '.open file:test.db?vfs=memvfs'
memvfs.so: main.go ext.go
	go build -tags SQLITE3VFS_LOADABLE_EXT -buildmode=c-shared -o $@ .
	rm -f memvfs.h

.PHONY: clean
clean:
	rm -f memvfs.so memvfs.h

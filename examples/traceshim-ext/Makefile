# Build the loadable extension:  make   (produces traceshim.so)
# Use it:  sqlite3 -cmd '.load ./traceshim' 'file:test.db?vfs=traceshim'  (see README)
traceshim.so: main.go ext.go
	go build -tags SQLITE3VFSSHIM_LOADABLE_EXT -buildmode=c-shared -o $@ .
	rm -f traceshim.h

.PHONY: clean
clean:
	rm -f traceshim.so traceshim.h

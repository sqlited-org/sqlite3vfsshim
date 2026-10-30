// Command memvfs shows how to build a standalone VFS (one that wraps no other
// VFS) with the pkg/vfs library: a complete in-memory file system in pure Go
// (see ../internal/memvfs).
package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sqlited-org/sqlite3vfsshim/examples/internal/memvfs"
	"github.com/sqlited-org/sqlite3vfsshim/pkg/vfs"
)

func main() {
	mem := memvfs.New()
	if err := vfs.Register("memvfs", mem); err != nil {
		log.Fatal(err)
	}
	defer vfs.Unregister("memvfs")

	db, err := sql.Open("sqlite3", "file:demo.db?vfs=memvfs")
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`create table t(id integer primary key, v text);
		insert into t(v) values ('hello'), ('from'), ('memory')`); err != nil {
		log.Fatal(err)
	}
	rows, err := db.Query(`select v from t order by id`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var s string
		rows.Scan(&s)
		fmt.Println(s)
	}
	rows.Close()
	db.Close()

	fmt.Printf("demo.db is %d bytes, held in memory (no file on disk)\n", mem.Size("demo.db"))
}

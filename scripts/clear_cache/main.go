package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func main() {
	db, _ := sql.Open("sqlite", "data/hk_parts.db?_pragma=journal_mode(WAL)")
	r, err := db.Exec("DELETE FROM online_parts_cache")
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	n, _ := r.RowsAffected()
	fmt.Printf("Deleted %d cached entries\n", n)
}

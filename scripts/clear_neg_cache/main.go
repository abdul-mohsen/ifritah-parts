package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "data/hk_parts.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Count before
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM online_parts_cache WHERE source = 'not_found'`).Scan(&count)
	fmt.Printf("Negative cache entries before: %d\n", count)

	res, err := db.Exec(`DELETE FROM online_parts_cache WHERE source = 'not_found'`)
	if err != nil {
		log.Fatal(err)
	}
	rows, _ := res.RowsAffected()
	fmt.Printf("Deleted %d negative cache entries\n", rows)

	// Count after
	db.QueryRow(`SELECT COUNT(*) FROM online_parts_cache WHERE source = 'not_found'`).Scan(&count)
	fmt.Printf("Negative cache entries after: %d\n", count)

	// Total cache entries remaining
	db.QueryRow(`SELECT COUNT(*) FROM online_parts_cache`).Scan(&count)
	fmt.Printf("Total cache entries remaining: %d\n", count)
}

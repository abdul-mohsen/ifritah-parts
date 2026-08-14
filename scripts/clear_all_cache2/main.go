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

	// Clear ALL online cache — forces re-scrape with new regex
	res, err := db.Exec(`DELETE FROM online_parts_cache`)
	if err != nil {
		log.Fatal("clear online_parts_cache:", err)
	}
	rows, _ := res.RowsAffected()
	fmt.Printf("Cleared %d online_parts_cache entries (forces full re-scrape)\n", rows)

	// Clear dealer cache
	db.Exec(`CREATE TABLE IF NOT EXISTS dealer_parts_index (oem_number TEXT, description TEXT, make TEXT, category TEXT, source TEXT, fetched_at DATETIME)`)
	res, err = db.Exec(`DELETE FROM dealer_parts_index`)
	if err == nil {
		rows, _ = res.RowsAffected()
		fmt.Printf("Cleared %d dealer_parts_index entries\n", rows)
	}

	fmt.Println("All caches cleared. Server will re-scrape from PartsOuq with expanded brand regex.")
}

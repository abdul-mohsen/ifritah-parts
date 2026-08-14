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

	// Check for bogus entry
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM dealer_parts_index WHERE oem_number LIKE '%99999%'`).Scan(&count)
	fmt.Printf("Bogus dealer entries: %d\n", count)

	// Delete it
	res, err := db.Exec(`DELETE FROM dealer_parts_index WHERE oem_number LIKE '%99999%'`)
	if err != nil {
		log.Fatal(err)
	}
	rows, _ := res.RowsAffected()
	fmt.Printf("Deleted %d bogus dealer entries\n", rows)
}

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

	// Clear bad dealer entries
	res, err := db.Exec(`DELETE FROM dealer_parts_index WHERE description LIKE '%GENUINE OEM%' OR description LIKE '%PARTS AND ACCESSORIES%' OR description LIKE '%HYUNDAI PARTS%' OR description LIKE '%KIA PARTS%'`)
	if err != nil {
		log.Fatal(err)
	}
	rows, _ := res.RowsAffected()
	fmt.Printf("Cleaned %d bad dealer entries\n", rows)

	// Also clear all dealer cache for full re-test
	res, err = db.Exec(`DELETE FROM dealer_parts_index`)
	if err != nil {
		log.Fatal(err)
	}
	rows, _ = res.RowsAffected()
	fmt.Printf("Cleared all %d dealer entries for clean re-test\n", rows)

	// Also clear negative online cache
	res, err = db.Exec(`DELETE FROM online_parts_cache WHERE source = 'not_found'`)
	if err != nil {
		log.Fatal(err)
	}
	rows, _ = res.RowsAffected()
	fmt.Printf("Cleared %d negative cache entries\n", rows)
}

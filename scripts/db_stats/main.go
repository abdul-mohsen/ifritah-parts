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

	var c int
	db.QueryRow("SELECT COUNT(*) FROM oem_search_index").Scan(&c)
	fmt.Println("oem_search_index rows:", c)

	db.QueryRow("SELECT COUNT(DISTINCT oemNumber) FROM articlecrosses").Scan(&c)
	fmt.Println("articlecrosses distinct OEM:", c)

	db.QueryRow("SELECT COUNT(*) FROM articlecrosses").Scan(&c)
	fmt.Println("articlecrosses total rows:", c)

	db.QueryRow("SELECT COUNT(*) FROM online_parts_cache").Scan(&c)
	fmt.Println("online_parts_cache:", c)

	db.QueryRow("SELECT COUNT(*) FROM online_parts_cache WHERE source='not_found'").Scan(&c)
	fmt.Println("online_parts_cache not_found:", c)

	db.QueryRow("SELECT COUNT(*) FROM dealer_parts_index").Scan(&c)
	fmt.Println("dealer_parts_index:", c)

	// Check some NOT FOUND parts to see what happens
	notFound := []string{"27301-3C010", "28950-2E300", "39110-3FVN0", "87614-D3100", "82460-D3010"}
	for _, p := range notFound {
		var cnt int
		norm := ""
		for _, ch := range p {
			if (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				norm += string(ch)
			}
		}
		db.QueryRow("SELECT COUNT(*) FROM articlecrosses WHERE oemNumber = ?", p).Scan(&cnt)
		fmt.Printf("  articlecrosses %s: %d\n", p, cnt)
		db.QueryRow("SELECT COUNT(*) FROM articlecrosses WHERE oemNumber LIKE ?", norm[:8]+"%").Scan(&cnt)
		fmt.Printf("  articlecrosses prefix %s%%: %d\n", norm[:8], cnt)
		db.QueryRow("SELECT COUNT(*) FROM oem_search_index WHERE oem_number LIKE ?", norm[:8]+"%").Scan(&cnt)
		fmt.Printf("  oem_search_index prefix %s%%: %d\n", norm[:8], cnt)
	}
}

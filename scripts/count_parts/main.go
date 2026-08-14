package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "data/hk_parts.db")
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
	defer db.Close()

	// Count distinct OEM numbers
	var oemCount int
	db.QueryRow("SELECT COUNT(DISTINCT raw_number) FROM oem_search_index").Scan(&oemCount)
	fmt.Printf("oem_search_index distinct raw_number: %d\n", oemCount)

	// Count articlecrosses
	var acCount int
	db.QueryRow("SELECT COUNT(DISTINCT oemNumber) FROM articlecrosses").Scan(&acCount)
	fmt.Printf("articlecrosses distinct oemNumber: %d\n", acCount)

	// Count hk_parts_cache distinct articles
	var hkCount int
	db.QueryRow("SELECT COUNT(DISTINCT articleNumber) FROM hk_parts_cache").Scan(&hkCount)
	fmt.Printf("hk_parts_cache distinct articles: %d\n", hkCount)

	// Count online_parts_cache
	var onlineCount int
	db.QueryRow("SELECT COUNT(*) FROM online_parts_cache").Scan(&onlineCount)
	fmt.Printf("online_parts_cache entries: %d\n", onlineCount)

	// Show sample OEM numbers
	fmt.Println("\nSample OEM numbers from oem_search_index:")
	rows, _ := db.Query("SELECT DISTINCT raw_number, description, brand_name FROM oem_search_index LIMIT 20")
	defer rows.Close()
	for rows.Next() {
		var raw, desc, brand string
		rows.Scan(&raw, &desc, &brand)
		fmt.Printf("  %-18s %-40s %s\n", raw, desc, brand)
	}

	// Show sample from articlecrosses
	fmt.Println("\nSample OEM numbers from articlecrosses:")
	rows2, _ := db.Query("SELECT DISTINCT oemNumber FROM articlecrosses LIMIT 20")
	defer rows2.Close()
	for rows2.Next() {
		var oem string
		rows2.Scan(&oem)
		fmt.Printf("  %s\n", oem)
	}
}

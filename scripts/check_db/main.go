package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", `c:\ssda\chatGPT\parts-engine\data\hk_parts.db`)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Count
	var cnt int
	db.QueryRow("SELECT COUNT(*) FROM articles").Scan(&cnt)
	fmt.Printf("Total articles: %d\n", cnt)

	db.QueryRow("SELECT COUNT(*) FROM oem_cross_ref").Scan(&cnt)
	fmt.Printf("Total OEM cross-refs: %d\n", cnt)

	// Sample article numbers
	fmt.Println("\nSample article numbers:")
	rows, _ := db.Query("SELECT articleNumber FROM articles LIMIT 20")
	defer rows.Close()
	for rows.Next() {
		var s string
		rows.Scan(&s)
		fmt.Println("  ", s)
	}

	// Check for 52933
	fmt.Println("\nSearch for '%52933%':")
	rows2, _ := db.Query("SELECT articleNumber, description FROM articles WHERE articleNumber LIKE '%52933%'")
	defer rows2.Close()
	found := false
	for rows2.Next() {
		var a, d string
		rows2.Scan(&a, &d)
		fmt.Printf("  %s — %s\n", a, d)
		found = true
	}
	if !found {
		fmt.Println("  (none)")
	}

	// OEM cross-ref search
	fmt.Println("\nOEM cross-ref for '%52933%':")
	rows3, _ := db.Query("SELECT rawNumber, articleNumber FROM oem_cross_ref WHERE rawNumber LIKE '%52933%'")
	defer rows3.Close()
	found = false
	for rows3.Next() {
		var r, a string
		rows3.Scan(&r, &a)
		fmt.Printf("  %s → %s\n", r, a)
		found = true
	}
	if !found {
		fmt.Println("  (none)")
	}
}

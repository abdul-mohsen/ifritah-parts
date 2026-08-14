package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "data/hk_parts.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("=== TABLES ===")
	rows, _ := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	var tables []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
		fmt.Println(" ", name)
	}
	rows.Close()

	for _, t := range tables {
		fmt.Printf("\n=== %s ===\n", t)

		// Count
		var count int
		db.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s", t)).Scan(&count)
		fmt.Printf("  Count: %d\n", count)

		// Schema
		schemaRows, _ := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", t))
		var cols []string
		for schemaRows.Next() {
			var cid int
			var name, typ string
			var notNull int
			var dflt sql.NullString
			var pk int
			schemaRows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk)
			cols = append(cols, name+"("+typ+")")
		}
		schemaRows.Close()
		fmt.Printf("  Columns: %s\n", strings.Join(cols, ", "))

		// Sample rows
		if count > 0 {
			sampleRows, _ := db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT 3", t))
			colNames, _ := sampleRows.Columns()
			fmt.Printf("  Sample (%s):\n", strings.Join(colNames, " | "))
			vals := make([]interface{}, len(colNames))
			ptrs := make([]interface{}, len(colNames))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			for sampleRows.Next() {
				sampleRows.Scan(ptrs...)
				parts := make([]string, len(vals))
				for i, v := range vals {
					parts[i] = fmt.Sprintf("%v", v)
				}
				fmt.Printf("    %s\n", strings.Join(parts, " | "))
			}
			sampleRows.Close()
		}
	}

	// Check if TecDoc MySQL is accessible
	fmt.Println("\n=== CHECKING articlecrosses for aftermarket brands ===")
	crossRows, err := db.Query("SELECT DISTINCT brand FROM articlecrosses LIMIT 20")
	if err != nil {
		fmt.Println("  No brand column or error:", err)
		// Try other columns
		crossRows2, err2 := db.Query("SELECT * FROM articlecrosses LIMIT 5")
		if err2 == nil {
			cols, _ := crossRows2.Columns()
			fmt.Printf("  Columns: %s\n", strings.Join(cols, ", "))
			crossRows2.Close()
		}
	} else {
		for crossRows.Next() {
			var brand string
			crossRows.Scan(&brand)
			fmt.Printf("  Brand: %s\n", brand)
		}
		crossRows.Close()
	}

	// Check if we can read MySQL TecDoc
	fmt.Println("\n=== Checking for TecDoc data ===")
	if _, err := os.Stat("data/tecdoc.db"); err == nil {
		fmt.Println("  Found tecdoc.db!")
	}
	if _, err := os.Stat("tecdoc.db"); err == nil {
		fmt.Println("  Found tecdoc.db in root!")
	}

	// Check MySQL connection info in code
	fmt.Println("\n=== Looking at articlecrosses sample with all columns ===")
	sampleRows, err := db.Query("SELECT * FROM articlecrosses LIMIT 5")
	if err != nil {
		fmt.Println("  Error:", err)
		return
	}
	cols, _ := sampleRows.Columns()
	fmt.Printf("  Columns: %s\n", strings.Join(cols, ", "))
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for sampleRows.Next() {
		sampleRows.Scan(ptrs...)
		for i, c := range cols {
			fmt.Printf("    %s: %v\n", c, vals[i])
		}
		fmt.Println("    ---")
	}
	sampleRows.Close()
}

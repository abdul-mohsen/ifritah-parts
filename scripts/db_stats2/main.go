package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "data/hk_parts.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	queries := []string{
		"SELECT COUNT(*) FROM oem_search_index",
		"SELECT COUNT(DISTINCT oemNumber) FROM articlecrosses",
		"SELECT COUNT(*) FROM articlecrosses",
		"SELECT COUNT(*) FROM online_parts_cache",
		"SELECT COUNT(*) FROM online_parts_cache WHERE source='not_found'",
		"SELECT COUNT(*) FROM dealer_parts_index",
	}
	labels := []string{
		"oem_search_index rows",
		"articlecrosses distinct OEM",
		"articlecrosses total rows",
		"online_parts_cache total",
		"online_parts_cache not_found",
		"dealer_parts_index rows",
	}

	for i, q := range queries {
		var c int
		if err := db.QueryRow(q).Scan(&c); err != nil {
			fmt.Printf("%s: ERROR %v\n", labels[i], err)
		} else {
			fmt.Printf("%s: %d\n", labels[i], c)
		}
	}

	// Test specific NOT FOUND parts
	fmt.Println("\n--- Checking NOT FOUND parts ---")
	notFound := []string{"27301-3C010", "28950-2E300", "39110-3FVN0", "87614-D3100", "82460-D3010"}
	for _, p := range notFound {
		norm := ""
		for _, ch := range strings.ToUpper(p) {
			if (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') {
				norm += string(ch)
			}
		}
		var cnt int
		db.QueryRow("SELECT COUNT(*) FROM articlecrosses WHERE oemNumber = ?", p).Scan(&cnt)
		fmt.Printf("  %s → articlecrosses exact: %d", p, cnt)

		prefix := norm
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		db.QueryRow("SELECT COUNT(*) FROM articlecrosses WHERE oemNumber LIKE ?", prefix+"%").Scan(&cnt)
		fmt.Printf(", prefix %s%%: %d", prefix, cnt)

		db.QueryRow("SELECT COUNT(*) FROM oem_search_index WHERE oem_number LIKE ?", prefix+"%").Scan(&cnt)
		fmt.Printf(", oem_index prefix: %d\n", cnt)
	}
}

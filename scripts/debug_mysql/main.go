package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	user := envOr("DBUSER", "root")
	pass := envOr("PASSWORD", "")
	host := envOr("HOST", "127.0.0.1")
	port := envOr("DBPORT", "3306")
	dbname := envOr("DBNAME", "dev_ifritah")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4", user, pass, host, port, dbname)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Open:", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatal("Ping:", err)
	}
	fmt.Println("Connected to MySQL:", dbname)

	// List all tables
	fmt.Println("\n=== ALL TABLES ===")
	rows, _ := db.Query("SHOW TABLES")
	var tables []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
	}
	rows.Close()
	for _, t := range tables {
		fmt.Printf("  %s\n", t)
	}

	// Check for TecDoc standard tables
	fmt.Println("\n=== LOOKING FOR CROSS-REFERENCE TABLES ===")
	crossTables := []string{"article_oe", "article_oe_n", "articlecrosses", "oem_search_index", "article_links"}
	for _, t := range crossTables {
		var count int
		err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", t)).Scan(&count)
		if err == nil {
			fmt.Printf("  %s: %d rows\n", t, count)
		}
	}

	// Check articlecrosses schema and sample WITH brand info
	fmt.Println("\n=== articlecrosses schema ===")
	schemaRows, err := db.Query("DESCRIBE articlecrosses")
	if err == nil {
		for schemaRows.Next() {
			var field, typ, null, key string
			var dflt, extra sql.NullString
			schemaRows.Scan(&field, &typ, &null, &key, &dflt, &extra)
			fmt.Printf("  %s %s\n", field, typ)
		}
		schemaRows.Close()
	}

	// Count distinct brands in articlecrosses
	fmt.Println("\n=== DISTINCT BRANDS in articlecrosses ===")
	brandRows, err := db.Query("SELECT brandName, COUNT(*) as cnt FROM articlecrosses GROUP BY brandName ORDER BY cnt DESC LIMIT 50")
	if err == nil {
		for brandRows.Next() {
			var brand string
			var cnt int
			brandRows.Scan(&brand, &cnt)
			fmt.Printf("  %s: %d\n", brand, cnt)
		}
		brandRows.Close()
	}

	// Find aftermarket cross-references for a known OEM number
	testOEM := "26300-35505" // Oil filter
	fmt.Printf("\n=== Cross-refs for OEM %s ===\n", testOEM)

	// Try 1: direct oemNumber match
	xrefRows, err := db.Query("SELECT legacyArticleId, oemNumber, brandName FROM articlecrosses WHERE oemNumber = ? OR oemNumber = ?",
		testOEM, strings.ReplaceAll(testOEM, "-", ""))
	if err == nil {
		for xrefRows.Next() {
			var id int
			var oem, brand string
			xrefRows.Scan(&id, &oem, &brand)
			fmt.Printf("  ArticleID=%d OEM=%s Brand=%s\n", id, oem, brand)
		}
		xrefRows.Close()
	}

	// Try article_oe table if it exists
	fmt.Println("\n=== article_oe table ===")
	oeRows, err := db.Query("SELECT * FROM article_oe WHERE OENbr LIKE ? OR OENbr LIKE ? LIMIT 20",
		"%2630035505%", "%26300-35505%")
	if err == nil {
		cols, _ := oeRows.Columns()
		fmt.Printf("  Columns: %s\n", strings.Join(cols, ", "))
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for oeRows.Next() {
			oeRows.Scan(ptrs...)
			for i, c := range cols {
				fmt.Printf("    %s: %v\n", c, vals[i])
			}
			fmt.Println("    ---")
		}
		oeRows.Close()
	} else {
		fmt.Printf("  article_oe not found: %v\n", err)
	}

	// Check for a articles/parts table with brand info
	fmt.Println("\n=== articles table structure ===")
	for _, tbl := range []string{"articles", "article", "parts", "hk_parts_cache"} {
		colRows, err := db.Query(fmt.Sprintf("DESCRIBE %s", tbl))
		if err == nil {
			fmt.Printf("  Table %s:\n", tbl)
			for colRows.Next() {
				var field, typ, null, key string
				var dflt, extra sql.NullString
				colRows.Scan(&field, &typ, &null, &key, &dflt, &extra)
				fmt.Printf("    %s %s\n", field, typ)
			}
			colRows.Close()
			break
		}
	}

	// Check hk_parts_cache in MySQL for aftermarket brands
	fmt.Println("\n=== hk_parts_cache brands in MySQL ===")
	hkBrandRows, err := db.Query("SELECT DISTINCT brandName, COUNT(*) FROM hk_parts_cache GROUP BY brandName ORDER BY COUNT(*) DESC LIMIT 20")
	if err == nil {
		for hkBrandRows.Next() {
			var brand string
			var cnt int
			hkBrandRows.Scan(&brand, &cnt)
			fmt.Printf("  %s: %d\n", brand, cnt)
		}
		hkBrandRows.Close()
	}

	// Total count in MySQL articlecrosses
	var totalCross int
	db.QueryRow("SELECT COUNT(*) FROM articlecrosses").Scan(&totalCross)
	fmt.Printf("\n=== Total cross-refs in MySQL: %d ===\n", totalCross)

	// How many of those are non-HK brand?
	var nonHK int
	db.QueryRow("SELECT COUNT(*) FROM articlecrosses WHERE brandName NOT LIKE '%HYUNDAI%' AND brandName NOT LIKE '%KIA%'").Scan(&nonHK)
	fmt.Printf("  Non-HK cross-refs: %d\n", nonHK)

	// What non-HK brands exist?
	fmt.Println("\n=== Non-HK brands in articlecrosses ===")
	nhkRows, err := db.Query("SELECT brandName, COUNT(*) FROM articlecrosses WHERE brandName NOT LIKE '%HYUNDAI%' AND brandName NOT LIKE '%KIA%' GROUP BY brandName ORDER BY COUNT(*) DESC LIMIT 30")
	if err == nil {
		for nhkRows.Next() {
			var brand string
			var cnt int
			nhkRows.Scan(&brand, &cnt)
			fmt.Printf("  %s: %d\n", brand, cnt)
		}
		nhkRows.Close()
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

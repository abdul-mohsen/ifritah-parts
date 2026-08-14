package main

import (
	"database/sql"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"

	"parts-engine/internal/config"
)

// export reads hk_parts_cache from MySQL and writes it to a SQLite file.
// Also exports oem_search_index, platform map, nhtsa_tecdoc_bridge, and
// articlecrosses (HK subset) for full offline capability.
//
// Usage: go run ./cmd/export [-o output.db]
func main() {
	outPath := "data/hk_parts.db"
	if len(os.Args) > 2 && os.Args[1] == "-o" {
		outPath = os.Args[2]
	}

	// Connect MySQL
	cfg := config.Load()
	mysql, err := sql.Open("mysql", cfg.MySQLDSN())
	if err != nil {
		log.Fatalf("MySQL: %v", err)
	}
	defer mysql.Close()
	if err := mysql.Ping(); err != nil {
		log.Fatalf("MySQL unreachable: %v", err)
	}
	log.Println("✓ MySQL connected")

	// Remove old SQLite file
	os.Remove(outPath)

	// Create SQLite
	lite, err := sql.Open("sqlite", outPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("SQLite: %v", err)
	}
	defer lite.Close()

	// Disable sync for fast bulk insert
	lite.Exec("PRAGMA synchronous = OFF")
	lite.Exec("PRAGMA journal_mode = WAL")

	start := time.Now()

	// --- Table 1: hk_parts_cache ---
	exportPartsCache(mysql, lite)

	// --- Table 2: oem_search_index (HK subset) ---
	exportOEMIndex(mysql, lite)

	// --- Table 3: hk_platform_map ---
	exportPlatformMap(mysql, lite)

	// --- Table 4: nhtsa_tecdoc_bridge ---
	exportBridge(mysql, lite)

	// --- Table 5: articlecrosses (HK subset) ---
	exportCrossRefs(mysql, lite)

	// --- Table 6: vehicle_lookup (flattened bridge → modelseries → linkagetargets) ---
	exportVehicleLookup(mysql, lite)

	log.Printf("✓ Export complete in %v → %s", time.Since(start).Round(time.Second), outPath)

	// File size
	if fi, err := os.Stat(outPath); err == nil {
		log.Printf("  File size: %.1f MB", float64(fi.Size())/1024/1024)
	}
}

func exportPartsCache(mysql, lite *sql.DB) {
	log.Println("Exporting hk_parts_cache...")

	lite.Exec(`CREATE TABLE hk_parts_cache (
		linkingTargetId   INTEGER NOT NULL,
		legacyArticleId   INTEGER NOT NULL,
		assemblyGroupNodeId INTEGER NOT NULL DEFAULT 0,
		articleNumber     TEXT,
		genericArticleDesc TEXT,
		dataSupplierId    INTEGER,
		brandName         TEXT,
		categoryName      TEXT,
		vehicleDesc       TEXT,
		manuId            INTEGER,
		modelId           INTEGER,
		modelName         TEXT,
		beginYearMonth    TEXT,
		endYearMonth      TEXT,
		fuelType          TEXT,
		capacityCC        INTEGER,
		horsePowerFrom    INTEGER,
		PRIMARY KEY (linkingTargetId, legacyArticleId, assemblyGroupNodeId)
	)`)

	copyTable(mysql, lite,
		`SELECT linkingTargetId, legacyArticleId, assemblyGroupNodeId,
				articleNumber, genericArticleDesc, dataSupplierId, brandName,
				categoryName, vehicleDesc, manuId, modelId, modelName,
				beginYearMonth, endYearMonth, fuelType, capacityCC, horsePowerFrom
		 FROM hk_parts_cache`,
		`INSERT INTO hk_parts_cache VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		17)

	lite.Exec("CREATE INDEX idx_hk_article ON hk_parts_cache(legacyArticleId)")
	lite.Exec("CREATE INDEX idx_hk_model ON hk_parts_cache(manuId, modelId)")
	lite.Exec("CREATE INDEX idx_hk_artnum ON hk_parts_cache(articleNumber)")
	lite.Exec("CREATE INDEX idx_hk_desc ON hk_parts_cache(genericArticleDesc)")
	lite.Exec("CREATE INDEX idx_hk_brand ON hk_parts_cache(dataSupplierId)")
}

func exportOEMIndex(mysql, lite *sql.DB) {
	log.Println("Exporting oem_search_index (HK subset)...")

	lite.Exec(`CREATE TABLE oem_search_index (
		raw_number     TEXT,
		normalized     TEXT,
		legacyArticleId INTEGER,
		source_table   TEXT,
		mfr_name       TEXT,
		brand_name     TEXT,
		article_number TEXT,
		description    TEXT
	)`)

	copyTable(mysql, lite,
		`SELECT oi.raw_number, oi.normalized, oi.legacyArticleId, oi.source_table,
				oi.mfr_name, oi.brand_name, oi.article_number, oi.description
		 FROM oem_search_index oi
		 WHERE oi.legacyArticleId IN (SELECT DISTINCT legacyArticleId FROM hk_parts_cache)`,
		`INSERT INTO oem_search_index VALUES (?,?,?,?,?,?,?,?)`,
		8)

	lite.Exec("CREATE INDEX idx_oem_norm ON oem_search_index(normalized)")
	lite.Exec("CREATE INDEX idx_oem_article ON oem_search_index(legacyArticleId)")
}

func exportPlatformMap(mysql, lite *sql.DB) {
	log.Println("Exporting hk_platform_map...")

	lite.Exec(`CREATE TABLE hk_platform_map (
		hyundai_model TEXT,
		kia_model     TEXT,
		platform_code TEXT
	)`)

	copyTable(mysql, lite,
		`SELECT hyundai_model, kia_model, platform_code FROM hk_platform_map`,
		`INSERT INTO hk_platform_map VALUES (?,?,?)`,
		3)
}

func exportBridge(mysql, lite *sql.DB) {
	log.Println("Exporting nhtsa_tecdoc_bridge...")

	lite.Exec(`CREATE TABLE nhtsa_tecdoc_bridge (
		nhtsa_make      TEXT,
		nhtsa_model     TEXT,
		tecdoc_model_id INTEGER,
		year_from       INTEGER,
		year_to         INTEGER
	)`)

	copyTable(mysql, lite,
		`SELECT nhtsa_make, nhtsa_model, tecdoc_model_id, year_from, year_to FROM nhtsa_tecdoc_bridge`,
		`INSERT INTO nhtsa_tecdoc_bridge VALUES (?,?,?,?,?)`,
		5)

	lite.Exec("CREATE INDEX idx_bridge ON nhtsa_tecdoc_bridge(nhtsa_make, nhtsa_model, year_from, year_to)")
}

func exportCrossRefs(mysql, lite *sql.DB) {
	log.Println("Exporting articlecrosses (HK subset)...")

	lite.Exec(`CREATE TABLE articlecrosses (
		legacyArticleId INTEGER,
		oemNumber       TEXT,
		brandName       TEXT
	)`)

	copyTable(mysql, lite,
		`SELECT ac.legacyArticleId, ac.oemNumber, ac.brandName
		 FROM articlecrosses ac
		 WHERE ac.legacyArticleId IN (SELECT DISTINCT legacyArticleId FROM hk_parts_cache)`,
		`INSERT INTO articlecrosses VALUES (?,?,?)`,
		3)

	lite.Exec("CREATE INDEX idx_cross_article ON articlecrosses(legacyArticleId)")
	lite.Exec("CREATE INDEX idx_cross_oem ON articlecrosses(oemNumber)")
}

func exportVehicleLookup(mysql, lite *sql.DB) {
	log.Println("Exporting vehicle_lookup (flattened bridge → linkagetargets)...")

	lite.Exec(`CREATE TABLE vehicle_lookup (
		nhtsa_make      TEXT,
		nhtsa_model     TEXT,
		year_from       INTEGER,
		year_to         INTEGER,
		linkageTargetId INTEGER,
		description     TEXT,
		beginYearMonth  TEXT,
		endYearMonth    TEXT,
		fuelType        TEXT,
		capacityCC      INTEGER,
		horsePowerFrom  INTEGER
	)`)

	copyTable(mysql, lite,
		`SELECT DISTINCT b.nhtsa_make, b.nhtsa_model, b.year_from, b.year_to,
				lt.linkageTargetId, lt.description, lt.beginYearMonth, lt.endYearMonth,
				lt.fuelType, lt.capacityCC, lt.horsePowerFrom
		 FROM nhtsa_tecdoc_bridge b
		 JOIN modelseries ms ON ms.modelId = b.tecdoc_model_id
		 JOIN linkagetargets lt ON lt.vehicleModelSeriesId = ms.modelId AND lt.lang = 'en'
		 WHERE ms.manuId IN (183, 184)`,
		`INSERT INTO vehicle_lookup VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		11)

	lite.Exec("CREATE INDEX idx_vl_lookup ON vehicle_lookup(nhtsa_make, nhtsa_model, year_from, year_to)")
	lite.Exec("CREATE INDEX idx_vl_ltid ON vehicle_lookup(linkageTargetId)")
}

// copyTable streams rows from MySQL to SQLite in batches.
func copyTable(mysql, lite *sql.DB, selectSQL, insertSQL string, cols int) {
	rows, err := mysql.Query(selectSQL)
	if err != nil {
		log.Printf("  ⚠ Query error: %v", err)
		return
	}
	defer rows.Close()

	tx, _ := lite.Begin()
	stmt, _ := tx.Prepare(insertSQL)

	count := 0
	batchSize := 5000

	for rows.Next() {
		vals := make([]any, cols)
		ptrs := make([]any, cols)
		for i := range vals {
			ptrs[i] = &vals[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			log.Printf("  ⚠ Scan error at row %d: %v", count, err)
			continue
		}
		stmt.Exec(vals...)
		count++

		if count%batchSize == 0 {
			tx.Commit()
			tx, _ = lite.Begin()
			stmt, _ = tx.Prepare(insertSQL)
		}
		if count%500000 == 0 {
			log.Printf("  ... %d rows", count)
		}
	}

	tx.Commit()
	log.Printf("  ✓ %d rows exported", count)
}

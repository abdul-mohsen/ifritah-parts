package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

const dbPath = "data/hk_parts.db"

func main() {
	log.SetFlags(log.Ltime)
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Check discovery tables exist
	var cnt int
	if err := db.QueryRow("SELECT COUNT(*) FROM discovered_oem_parts").Scan(&cnt); err != nil {
		log.Fatal("No discovered_oem_parts table found. Run partsouq_crawl first.")
	}
	fmt.Printf("Found %d discovered OEM parts to import\n", cnt)

	if cnt == 0 {
		fmt.Println("Nothing to import.")
		return
	}

	// ── Phase 1: Import OEM parts into oem_search_index ─────────────
	fmt.Println("\n═══ Phase 1: Importing OEM parts into oem_search_index ═══")

	// Get the max legacyArticleId to continue from
	var maxArtId int
	db.QueryRow("SELECT COALESCE(MAX(legacyArticleId), 900000) FROM oem_search_index").Scan(&maxArtId)
	nextArtId := maxArtId + 1

	rows, err := db.Query(`SELECT part_number, description, category, make FROM discovered_oem_parts
		WHERE part_number NOT IN (SELECT raw_number FROM oem_search_index)
		AND description != ''`)
	if err != nil {
		log.Fatal("Query discovered parts:", err)
	}

	var oemImported int
	tx, _ := db.Begin()

	for rows.Next() {
		var pn, desc, category, make_ string
		rows.Scan(&pn, &desc, &category, &make_)

		norm := normalize(pn)
		brand := "HYUNDAI/KIA"
		if make_ != "" {
			brand = strings.ToUpper(strings.ReplaceAll(make_, " ", ""))
			if strings.Contains(brand, "HYUNDAI") || strings.Contains(brand, "KIA") {
				brand = "HYUNDAI/KIA"
			}
		}

		// Insert into oem_search_index
		_, err := tx.Exec(
			`INSERT OR IGNORE INTO oem_search_index (raw_number, normalized, legacyArticleId, source_table, mfr_name, brand_name, article_number, description)
			 VALUES (?, ?, ?, 'discovered', ?, ?, ?, ?)`,
			pn, norm, nextArtId, brand, brand, pn, desc)
		if err != nil {
			log.Printf("  WARN: oem_search_index insert %s: %v", pn, err)
			continue
		}

		// Insert into articlecrosses
		tx.Exec(`INSERT OR IGNORE INTO articlecrosses (legacyArticleId, oemNumber, brandName) VALUES (?, ?, ?)`,
			nextArtId, pn, brand)

		nextArtId++
		oemImported++
	}
	rows.Close()
	tx.Commit()

	fmt.Printf("Imported %d new OEM parts into oem_search_index\n", oemImported)

	// ── Phase 2: Import aftermarket into aftermarket_crossref ────────
	fmt.Println("\n═══ Phase 2: Importing aftermarket cross-references ═══")

	rows2, err := db.Query(`SELECT oem_number, brand, part_number, description FROM discovered_aftermarket
		WHERE (oem_number, brand, part_number) NOT IN (SELECT oem_number, brand, part_number FROM aftermarket_crossref)`)
	if err != nil {
		log.Fatal("Query discovered aftermarket:", err)
	}

	var amImported int
	tx2, _ := db.Begin()

	for rows2.Next() {
		var oemNum, brand, partNum, desc string
		rows2.Scan(&oemNum, &brand, &partNum, &desc)

		// Determine category from OEM prefix
		category := decodePrefix(oemNum)

		_, err := tx2.Exec(
			`INSERT OR IGNORE INTO aftermarket_crossref (oem_number, brand, part_number, description, category, verified)
			 VALUES (?, ?, ?, ?, ?, 0)`,
			oemNum, brand, partNum, desc, category)
		if err != nil {
			continue
		}
		amImported++
	}
	rows2.Close()
	tx2.Commit()

	fmt.Printf("Imported %d new aftermarket cross-references\n", amImported)

	// ── Phase 3: Import substitutions as additional OEM cross-refs ───
	fmt.Println("\n═══ Phase 3: Importing substitution chains ═══")

	rows3, err := db.Query(`SELECT from_part, to_part, description FROM discovered_substitutions
		WHERE to_part NOT IN (SELECT raw_number FROM oem_search_index)`)
	if err != nil {
		log.Fatal("Query discovered substitutions:", err)
	}

	var subImported int
	tx3, _ := db.Begin()

	for rows3.Next() {
		var fromPart, toPart, desc string
		rows3.Scan(&fromPart, &toPart, &desc)

		norm := normalize(toPart)
		if desc == "" {
			// Try to get description from the from_part
			db.QueryRow("SELECT description FROM oem_search_index WHERE raw_number=?", fromPart).Scan(&desc)
			if desc == "" {
				db.QueryRow("SELECT description FROM discovered_oem_parts WHERE part_number=?", fromPart).Scan(&desc)
			}
			if desc != "" {
				desc = desc + " (supersedes " + fromPart + ")"
			}
		}

		_, err := tx3.Exec(
			`INSERT OR IGNORE INTO oem_search_index (raw_number, normalized, legacyArticleId, source_table, mfr_name, brand_name, article_number, description)
			 VALUES (?, ?, ?, 'substitution', 'HYUNDAI/KIA', 'HYUNDAI/KIA', ?, ?)`,
			toPart, norm, nextArtId, toPart, desc)
		if err != nil {
			continue
		}

		tx3.Exec(`INSERT OR IGNORE INTO articlecrosses (legacyArticleId, oemNumber, brandName) VALUES (?, ?, 'HYUNDAI/KIA')`,
			nextArtId, toPart)

		nextArtId++
		subImported++
	}
	rows3.Close()
	tx3.Commit()

	fmt.Printf("Imported %d substitution parts as new OEM entries\n", subImported)

	// ── Summary ─────────────────────────────────────────────────────
	fmt.Println("\n═══ Import Summary ═══")
	fmt.Printf("New OEM parts added:           %d\n", oemImported)
	fmt.Printf("New aftermarket refs added:     %d\n", amImported)
	fmt.Printf("New substitution parts added:   %d\n", subImported)

	// Final counts
	var totalOEM, totalAM, totalCross int
	db.QueryRow("SELECT COUNT(DISTINCT raw_number) FROM oem_search_index").Scan(&totalOEM)
	db.QueryRow("SELECT COUNT(*) FROM aftermarket_crossref").Scan(&totalAM)
	db.QueryRow("SELECT COUNT(*) FROM articlecrosses").Scan(&totalCross)

	fmt.Printf("\nDatabase totals after import:\n")
	fmt.Printf("  OEM parts (oem_search_index):  %d\n", totalOEM)
	fmt.Printf("  Aftermarket cross-refs:         %d\n", totalAM)
	fmt.Printf("  Article crosses:                %d\n", totalCross)
}

func normalize(pn string) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(pn) {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func decodePrefix(pn string) string {
	norm := normalize(pn)
	if len(norm) < 3 {
		return ""
	}
	prefixes3 := map[string]string{
		"263": "Oil Filter", "281": "Radiator", "282": "Thermostat",
		"284": "Water Pump", "285": "Coolant Hose", "581": "Brake Pad",
		"582": "Brake Caliper", "583": "Rear Brake", "971": "A/C Compressor",
		"972": "Condenser", "973": "Evaporator", "921": "Headlight",
		"924": "Tail Light", "983": "Wiper Blades",
	}
	if cat, ok := prefixes3[norm[:3]]; ok {
		return cat
	}
	prefixes2 := map[string]string{
		"21": "Engine", "22": "Engine Mount", "23": "Cylinder Head",
		"24": "Intake/Exhaust", "25": "Fuel System", "26": "Oil System",
		"27": "Emissions", "28": "Cooling", "29": "Turbo",
		"35": "CV Joint", "36": "Starter/Charging", "37": "Ignition",
		"39": "Sensors", "41": "Clutch", "43": "Auto Transmission",
		"54": "Front Suspension", "55": "Rear Suspension",
		"56": "Steering", "57": "Wheel Hub", "58": "Brakes",
		"59": "ABS/ESC", "92": "Lighting", "97": "HVAC", "98": "Wipers",
	}
	if cat, ok := prefixes2[norm[:2]]; ok {
		return cat
	}
	return ""
}

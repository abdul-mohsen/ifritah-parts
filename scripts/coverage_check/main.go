package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "data/hk_parts.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var totalParts int
	db.QueryRow("SELECT COUNT(DISTINCT raw_number) FROM oem_search_index").Scan(&totalParts)
	fmt.Printf("Total OEM part numbers in DB: %d\n", totalParts)

	var totalArticles int
	db.QueryRow("SELECT COUNT(DISTINCT legacyArticleId) FROM oem_search_index").Scan(&totalArticles)
	fmt.Printf("Total unique articles: %d\n", totalArticles)

	rows, _ := db.Query("SELECT brand_name, COUNT(DISTINCT raw_number) FROM oem_search_index GROUP BY brand_name ORDER BY COUNT(DISTINCT raw_number) DESC")
	fmt.Println("\nOEM Parts by brand:")
	for rows.Next() {
		var brand string
		var cnt int
		rows.Scan(&brand, &cnt)
		fmt.Printf("  %-20s %d\n", brand, cnt)
	}
	rows.Close()

	var totalVehicles int
	db.QueryRow("SELECT COUNT(DISTINCT linkingTargetId) FROM hk_parts_cache").Scan(&totalVehicles)
	fmt.Printf("\nVehicle variants in DB: %d\n", totalVehicles)

	var totalModels int
	db.QueryRow("SELECT COUNT(DISTINCT nhtsa_model) FROM vehicle_lookup").Scan(&totalModels)
	fmt.Printf("Vehicle models: %d\n", totalModels)

	var amTotal, amOEM, amBrands, amCats int
	db.QueryRow("SELECT COUNT(*) FROM aftermarket_crossref").Scan(&amTotal)
	db.QueryRow("SELECT COUNT(DISTINCT oem_number) FROM aftermarket_crossref").Scan(&amOEM)
	db.QueryRow("SELECT COUNT(DISTINCT brand) FROM aftermarket_crossref").Scan(&amBrands)
	db.QueryRow("SELECT COUNT(DISTINCT category) FROM aftermarket_crossref").Scan(&amCats)
	fmt.Printf("\nAftermarket cross-references: %d\n", amTotal)
	fmt.Printf("OEM parts with aftermarket: %d / %d (%.1f%%)\n", amOEM, totalParts, float64(amOEM)/float64(totalParts)*100)
	fmt.Printf("Aftermarket brands: %d\n", amBrands)
	fmt.Printf("Categories: %d\n", amCats)

	rows2, _ := db.Query("SELECT category, COUNT(DISTINCT oem_number), COUNT(*), COUNT(DISTINCT brand) FROM aftermarket_crossref GROUP BY category ORDER BY COUNT(*) DESC")
	fmt.Println("\nAftermarket by category:")
	fmt.Printf("  %-22s %5s %5s %6s\n", "Category", "OEMs", "Refs", "Brands")
	for rows2.Next() {
		var cat string
		var oems, refs, brands int
		rows2.Scan(&cat, &oems, &refs, &brands)
		fmt.Printf("  %-22s %5d %5d %6d\n", cat, oems, refs, brands)
	}
	rows2.Close()

	var uncovered int
	db.QueryRow(`SELECT COUNT(DISTINCT o.raw_number) FROM oem_search_index o 
		WHERE NOT EXISTS (SELECT 1 FROM aftermarket_crossref a 
		WHERE LOWER(REPLACE(a.oem_number,'-','')) = LOWER(REPLACE(o.raw_number,'-','')))`).Scan(&uncovered)
	fmt.Printf("\nOEM parts WITHOUT aftermarket: %d / %d\n", uncovered, totalParts)

	// Categories in main parts cache
	rows3, err3 := db.Query(`SELECT assemblyGroupName, COUNT(DISTINCT legacyArticleId) 
		FROM hk_parts_cache GROUP BY assemblyGroupName 
		ORDER BY COUNT(DISTINCT legacyArticleId) DESC LIMIT 30`)
	if err3 == nil && rows3 != nil {
		fmt.Println("\nOEM parts by assembly group (top 30):")
		for rows3.Next() {
			var cat string
			var cnt int
			rows3.Scan(&cat, &cnt)
			fmt.Printf("  %-40s %d\n", cat, cnt)
		}
		rows3.Close()
	} else {
		fmt.Println("\n(hk_parts_cache assembly group query not available)")
	}

	// Vehicle models supported
	rows4, err4 := db.Query(`SELECT nhtsa_make, nhtsa_model, COUNT(DISTINCT linkageTargetId) 
		FROM vehicle_lookup GROUP BY nhtsa_make, nhtsa_model ORDER BY nhtsa_make, nhtsa_model`)
	if err4 == nil && rows4 != nil {
		fmt.Println("\nVehicle models & variants:")
		for rows4.Next() {
			var make, model string
			var cnt int
			rows4.Scan(&make, &model, &cnt)
			fmt.Printf("  %-12s %-15s %d variants\n", make, model, cnt)
		}
		rows4.Close()
	} else {
		fmt.Println("\n(vehicle_lookup query not available)")
	}

	var cached, notFound int
	db.QueryRow("SELECT COUNT(*) FROM online_parts_cache WHERE found=1").Scan(&cached)
	db.QueryRow("SELECT COUNT(*) FROM online_parts_cache WHERE found=0").Scan(&notFound)
	fmt.Printf("\nOnline scrape cache: %d found, %d not found\n", cached, notFound)

	// Estimate total Hyundai/KIA serviceable parts
	fmt.Println("\n=== COVERAGE ESTIMATE ===")
	fmt.Println("Hyundai/KIA typical model has ~2000-3000 unique part numbers")
	fmt.Println("A full catalog (all models, all years) has ~300,000+ part numbers")
	fmt.Println("Common service/wear parts per model: ~200-400")
	fmt.Printf("Our OEM DB: %d parts\n", totalParts)
	fmt.Printf("Our aftermarket coverage: %d OEM parts → %d alternatives from %d brands\n", amOEM, amTotal, amBrands)

	// How many of our aftermarket categories are common service/wear parts?
	var servicePartOEMs int
	db.QueryRow(`SELECT COUNT(DISTINCT oem_number) FROM aftermarket_crossref 
		WHERE category IN ('Oil Filter','Air Filter','Cabin Filter','Fuel Filter',
		'Brake Pads','Brake Disc','Spark Plug','Ignition Coil','Engine Mount',
		'Shock Absorber','Radiator','Water Pump','Alternator','Starter Motor',
		'Wiper Blades','Wheel Bearing','Thermostat','Drive Belt','A/C Compressor',
		'Timing Belt','Clutch Kit','Lambda Sensor','Tie Rod End','Control Arm','Stabilizer Link',
		'Belt Tensioner','Ball Joint','CV Joint','TPMS Sensor','Bulb')`).Scan(&servicePartOEMs)
	fmt.Printf("\nService/wear OEM parts with aftermarket: %d\n", servicePartOEMs)

	// Confidence by source
	fmt.Println("\n=== CONFIDENCE LEVELS ===")
	fmt.Println("Phase 1 (MANN, MAHLE, BOSCH, TRW, BREMBO, etc.): HIGH - industry standard numbers")
	fmt.Println("RockAuto verified (FVP, WIX 51334, DENSO, BECK/ARNLEY): HIGH - confirmed real")
	fmt.Println("oilfilter-crossreference.com (VAICO V32-0018, Filtron OP617): HIGH - confirmed")
	fmt.Println("JAPANPARTS/NIPPARTS/ASHIKA group: MEDIUM-HIGH - consistent numbering scheme")
	fmt.Println("PartsOuq OEM scraping: HIGH for OEM data, N/A for aftermarket (OEM-only source)")
}

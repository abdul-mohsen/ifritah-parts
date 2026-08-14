package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	"parts-engine/internal/service"
)

// test_offline creates a small in-memory SQLite DB with sample data
// and exercises all offline query paths to verify they work.
func main() {
	db, err := sql.Open("sqlite", ":memory:?_pragma=journal_mode(WAL)")
	if err != nil {
		fmt.Printf("FAIL: open SQLite: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create schema
	tables := []string{
		`CREATE TABLE hk_parts_cache (
			linkingTargetId INTEGER, legacyArticleId INTEGER, assemblyGroupNodeId INTEGER DEFAULT 0,
			articleNumber TEXT, genericArticleDesc TEXT, dataSupplierId INTEGER,
			brandName TEXT, categoryName TEXT, vehicleDesc TEXT,
			manuId INTEGER, modelId INTEGER, modelName TEXT,
			beginYearMonth TEXT, endYearMonth TEXT, fuelType TEXT,
			capacityCC INTEGER, horsePowerFrom INTEGER,
			PRIMARY KEY (linkingTargetId, legacyArticleId, assemblyGroupNodeId))`,
		`CREATE TABLE vehicle_lookup (
			nhtsa_make TEXT, nhtsa_model TEXT, year_from INTEGER, year_to INTEGER,
			linkageTargetId INTEGER, description TEXT, beginYearMonth TEXT, endYearMonth TEXT,
			fuelType TEXT, capacityCC INTEGER, horsePowerFrom INTEGER)`,
		`CREATE TABLE nhtsa_tecdoc_bridge (
			nhtsa_make TEXT, nhtsa_model TEXT, tecdoc_model_id INTEGER,
			year_from INTEGER, year_to INTEGER)`,
		`CREATE TABLE oem_search_index (
			raw_number TEXT, normalized TEXT, legacyArticleId INTEGER, source_table TEXT,
			mfr_name TEXT, brand_name TEXT, article_number TEXT, description TEXT)`,
		`CREATE TABLE articlecrosses (
			legacyArticleId INTEGER, oemNumber TEXT, brandName TEXT)`,
		`CREATE TABLE hk_platform_map (
			hyundai_model TEXT, kia_model TEXT, platform_code TEXT)`,
		`CREATE INDEX idx_hk_article ON hk_parts_cache(legacyArticleId)`,
		`CREATE INDEX idx_hk_artnum ON hk_parts_cache(articleNumber)`,
		`CREATE INDEX idx_vl_lookup ON vehicle_lookup(nhtsa_make, nhtsa_model, year_from, year_to)`,
		`CREATE INDEX idx_oem_norm ON oem_search_index(normalized)`,
		`CREATE INDEX idx_cross_oem ON articlecrosses(oemNumber)`,
		`CREATE INDEX idx_cross_article ON articlecrosses(legacyArticleId)`,
	}
	for _, t := range tables {
		if _, err := db.Exec(t); err != nil {
			fmt.Printf("FAIL: create table: %v\nSQL: %s\n", err, t)
			os.Exit(1)
		}
	}

	// Seed sample data
	seeds := []string{
		// Hyundai Tucson 2020, 2.0L Petrol, linkageTargetId=1001
		`INSERT INTO vehicle_lookup VALUES ('HYUNDAI','TUCSON',2018,2024,1001,'2.0 MPI','201805','202412','Petrol',1999,150)`,
		// Hyundai Tucson 2020, 1.6T Diesel, linkageTargetId=1002
		`INSERT INTO vehicle_lookup VALUES ('HYUNDAI','TUCSON',2018,2024,1002,'1.6 CRDi','201805','202412','Diesel',1598,136)`,
		// Kia Sportage 2020, 2.0L Petrol, linkageTargetId=2001
		`INSERT INTO vehicle_lookup VALUES ('KIA','SPORTAGE',2018,2024,2001,'2.0 GDI','201805','202412','Petrol',1999,155)`,

		// Bridge
		`INSERT INTO nhtsa_tecdoc_bridge VALUES ('HYUNDAI','TUCSON',100,2018,2024)`,
		`INSERT INTO nhtsa_tecdoc_bridge VALUES ('KIA','SPORTAGE',200,2018,2024)`,

		// Parts for Tucson 2.0L (engine-specific)
		`INSERT INTO hk_parts_cache VALUES (1001,5001,0,'OC570','Oil Filter',10,'MAHLE','Engine / Oil Filter','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,
		`INSERT INTO hk_parts_cache VALUES (1001,5002,0,'LA123','Air Filter',10,'MAHLE','Engine / Air Filter','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,

		// Parts for Tucson 1.6T (engine-specific)
		`INSERT INTO hk_parts_cache VALUES (1002,5003,0,'OC571','Oil Filter',11,'BOSCH','Engine / Oil Filter','Tucson 1.6',183,100,'TUCSON','201805','202412','Diesel',1598,136)`,

		// Universal part (fits both)
		`INSERT INTO hk_parts_cache VALUES (1001,5010,0,'WB100','Wiper Blade',20,'BOSCH','Body / Wipers','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,
		`INSERT INTO hk_parts_cache VALUES (1002,5010,0,'WB100','Wiper Blade',20,'BOSCH','Body / Wipers','Tucson 1.6',183,100,'TUCSON','201805','202412','Diesel',1598,136)`,

		// Kia Sportage parts (shared platform)
		`INSERT INTO hk_parts_cache VALUES (2001,5001,0,'OC570','Oil Filter',10,'MAHLE','Engine / Oil Filter','Sportage 2.0',184,200,'SPORTAGE','201805','202412','Petrol',1999,155)`,
		`INSERT INTO hk_parts_cache VALUES (2001,5010,0,'WB100','Wiper Blade',20,'BOSCH','Body / Wipers','Sportage 2.0',184,200,'SPORTAGE','201805','202412','Petrol',1999,155)`,

		// OEM cross-refs
		`INSERT INTO articlecrosses VALUES (5001,'26300-35503','HYUNDAI/KIA')`,
		`INSERT INTO articlecrosses VALUES (5001,'26300-35504','HYUNDAI/KIA')`,
		`INSERT INTO articlecrosses VALUES (5002,'28113-2S000','HYUNDAI')`,

		// OEM search index
		`INSERT INTO oem_search_index VALUES ('26300-35503','2630035503',5001,'articlecrosses','HYUNDAI/KIA','MAHLE','OC570','Oil Filter')`,

		// Platform map
		`INSERT INTO hk_platform_map VALUES ('TUCSON','SPORTAGE','NX4/NQ5')`,
	}
	for _, s := range seeds {
		if _, err := db.Exec(s); err != nil {
			fmt.Printf("FAIL: seed: %v\nSQL: %s\n", err, s)
			os.Exit(1)
		}
	}

	pass := 0
	fail := 0

	check := func(name string, ok bool, detail string) {
		if ok {
			fmt.Printf("  ✓ %s\n", name)
			pass++
		} else {
			fmt.Printf("  ✗ %s — %s\n", name, detail)
			fail++
		}
	}

	// --- Test 1: PartsLookup (offline=true) ---
	fmt.Println("\n=== PartsLookup ===")
	pl := service.NewPartsLookup(db, true)

	parts, total, err := pl.FindByLinkageTarget(1001, "", 1, 20)
	check("FindByLinkageTarget", err == nil && total == 3, fmt.Sprintf("err=%v total=%d", err, total))

	parts, total, err = pl.FindByLinkageTarget(1001, "Oil", 1, 20)
	check("FindByLinkageTarget+category", err == nil && total == 1 && len(parts) == 1, fmt.Sprintf("err=%v total=%d parts=%d", err, total, len(parts)))

	vehicles, err := pl.ResolveLinkageTargets("HYUNDAI", "TUCSON", 2020)
	check("ResolveLinkageTargets", err == nil && len(vehicles) == 2, fmt.Sprintf("err=%v count=%d", err, len(vehicles)))

	best, err := pl.BestLinkageTargetWithHints("HYUNDAI", "TUCSON", 2020, 2000, "Petrol")
	check("BestLinkageTarget+hints(2.0 Petrol)", err == nil && best != nil && best.LinkageTargetId == 1001,
		fmt.Sprintf("err=%v best=%+v", err, best))

	best, err = pl.BestLinkageTargetWithHints("HYUNDAI", "TUCSON", 2020, 1600, "Diesel")
	check("BestLinkageTarget+hints(1.6 Diesel)", err == nil && best != nil && best.LinkageTargetId == 1002,
		fmt.Sprintf("err=%v best=%+v", err, best))

	revVehicles, err := pl.ReverseByArticle(5001, 50)
	check("ReverseByArticle", err == nil && len(revVehicles) >= 2, fmt.Sprintf("err=%v count=%d", err, len(revVehicles)))

	revVehicles, err = pl.ReverseByArticle(5010, 50)
	check("ReverseByArticle(universal)", err == nil && len(revVehicles) >= 3, fmt.Sprintf("err=%v count=%d", err, len(revVehicles)))

	// --- Test 2: CrossRef (offline=true) ---
	fmt.Println("\n=== CrossRef ===")
	cr := service.NewCrossRef(db, true)

	oems, err := cr.FindOEMNumbers(5001)
	check("FindOEMNumbers", err == nil && len(oems) >= 2, fmt.Sprintf("err=%v count=%d", err, len(oems)))

	byOEM, err := cr.FindByOEM("26300-35503", 20)
	check("FindByOEM", err == nil && len(byOEM) >= 1, fmt.Sprintf("err=%v count=%d", err, len(byOEM)))

	// --- Test 3: OEMLookup ---
	fmt.Println("\n=== OEMLookup ===")
	ol := service.NewOEMLookup(db)
	result, err := ol.Search("26300-35503", 20)
	check("OEMLookup.Search", err == nil && result != nil && len(result.Results) >= 1, fmt.Sprintf("err=%v results=%+v", err, result))

	// --- Test 4: Platform ---
	fmt.Println("\n=== Platform ===")
	pf := service.NewPlatform(db)

	siblings, err := pf.FindSiblings("HYUNDAI", "TUCSON")
	check("FindSiblings(Hyundai→Kia)", err == nil && len(siblings) >= 1 && siblings[0].SiblingModel == "SPORTAGE",
		fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	siblings, err = pf.FindSiblings("KIA", "SPORTAGE")
	check("FindSiblings(Kia→Hyundai)", err == nil && len(siblings) >= 1 && siblings[0].SiblingModel == "TUCSON",
		fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// Test fallback (model not in DB table but in hardcoded list)
	siblings, err = pf.FindSiblings("HYUNDAI", "ELANTRA")
	check("FindSiblings(fallback:Elantra)", err == nil && len(siblings) >= 1,
		fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// --- Test 5: SmartSearch (offline=true) ---
	fmt.Println("\n=== SmartSearch ===")
	ss := service.NewSmartSearch(db, pl, cr, ol, pf, nil, true)

	// Text search
	sr, err := ss.Search("Oil Filter", 0, 0, "", "", 1, 20)
	check("SmartSearch.text(Oil Filter)", err == nil && sr != nil && len(sr.Results) >= 1, fmt.Sprintf("err=%v results=%+v", err, sr))

	// Article search
	sr, err = ss.Search("OC570", 0, 0, "", "", 1, 20)
	check("SmartSearch.article(OC570)", err == nil && sr != nil && len(sr.Results) >= 1 && sr.SearchStrategy == "article_lookup",
		fmt.Sprintf("err=%v results=%+v", err, sr))

	// OEM search
	sr, err = ss.Search("26300-35503", 0, 0, "", "", 1, 20)
	check("SmartSearch.oem(26300-35503)", err == nil && sr != nil && len(sr.Results) >= 1,
		fmt.Sprintf("err=%v results=%+v", err, sr))

	// Vehicle search
	sr, err = ss.Search("", 1001, 1999, "Petrol", "", 1, 20)
	check("SmartSearch.vehicle(1001)", err == nil && sr != nil && len(sr.Results) >= 3,
		fmt.Sprintf("err=%v results=%d", err, func() int {
			if sr != nil {
				return len(sr.Results)
			}
			return 0
		}()))

	// --- Test 6: OEM Prefix Decoder ---
	fmt.Println("\n=== OEM Prefix Decoder ===")
	cat := service.DecodeOEMPrefix("26300-35503")
	check("DecodeOEMPrefix(263→Oil Filter)", cat != nil && cat.Category == "Oil Filter",
		fmt.Sprintf("got %+v", cat))

	cat = service.DecodeOEMPrefix("58101-C1A00")
	check("DecodeOEMPrefix(581→Front Brake)", cat != nil && cat.System == "Brakes",
		fmt.Sprintf("got %+v", cat))

	cat = service.DecodeOEMPrefix("92101-D3100")
	check("DecodeOEMPrefix(921→Headlight)", cat != nil && cat.Category == "Headlight Assembly",
		fmt.Sprintf("got %+v", cat))

	cat = service.DecodeOEMPrefix("ZZZZZ")
	check("DecodeOEMPrefix(unknown→nil)", cat == nil, fmt.Sprintf("got %+v", cat))

	// Summary
	fmt.Printf("\n========================================\n")
	fmt.Printf("  PASS: %d  |  FAIL: %d  |  TOTAL: %d\n", pass, fail, pass+fail)
	fmt.Printf("========================================\n")

	if fail > 0 {
		os.Exit(1)
	}

	_ = parts // suppress unused
}

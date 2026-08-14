package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

// seed_db creates data/hk_parts.db with realistic Hyundai/Kia parts catalog data
// organized by assembly groups (like TecDoc), enabling the full catalog browsing experience.
//
// This is used when MySQL (dev_ifritah) is not available.
// Usage: go run ./scripts/seed_db/main.go

func main() {
	outPath := "data/hk_parts.db"
	os.Remove(outPath)

	db, err := sql.Open("sqlite", outPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("SQLite open: %v", err)
	}
	defer db.Close()
	db.Exec("PRAGMA synchronous = OFF")

	// Create tables matching export schema
	for _, ddl := range []string{
		`CREATE TABLE hk_parts_cache (
			linkingTargetId INTEGER NOT NULL,
			legacyArticleId INTEGER NOT NULL,
			assemblyGroupNodeId INTEGER NOT NULL DEFAULT 0,
			articleNumber TEXT,
			genericArticleDesc TEXT,
			dataSupplierId INTEGER,
			brandName TEXT,
			categoryName TEXT,
			vehicleDesc TEXT,
			manuId INTEGER,
			modelId INTEGER,
			modelName TEXT,
			beginYearMonth TEXT,
			endYearMonth TEXT,
			fuelType TEXT,
			capacityCC INTEGER,
			horsePowerFrom INTEGER,
			PRIMARY KEY (linkingTargetId, legacyArticleId, assemblyGroupNodeId))`,
		`CREATE TABLE oem_search_index (
			raw_number TEXT, normalized TEXT, legacyArticleId INTEGER,
			source_table TEXT, mfr_name TEXT, brand_name TEXT,
			article_number TEXT, description TEXT)`,
		`CREATE TABLE articlecrosses (
			legacyArticleId INTEGER, oemNumber TEXT, brandName TEXT)`,
		`CREATE TABLE hk_platform_map (
			hyundai_model TEXT, kia_model TEXT, platform_code TEXT)`,
		`CREATE TABLE nhtsa_tecdoc_bridge (
			nhtsa_make TEXT, nhtsa_model TEXT, tecdoc_model_id INTEGER,
			year_from INTEGER, year_to INTEGER)`,
		`CREATE TABLE vehicle_lookup (
			nhtsa_make TEXT, nhtsa_model TEXT, year_from INTEGER, year_to INTEGER,
			linkageTargetId INTEGER, description TEXT, beginYearMonth TEXT,
			endYearMonth TEXT, fuelType TEXT, capacityCC INTEGER, horsePowerFrom INTEGER)`,
	} {
		mustExec(db, ddl)
	}

	// ── VEHICLES ──────────────────────────────────────────────────────────
	type vehicle struct {
		vid               int
		make, model, desc string
		fuel              string
		cc, hp, modelId   int
		manuId            int
		yearFrom, yearTo  int
	}

	vehicles := []vehicle{
		// HYUNDAI TUCSON
		{10001, "HYUNDAI", "TUCSON", "TUCSON 2.0 MPI (TL) 2015-2018", "Petrol", 1999, 155, 100, 183, 2015, 2018},
		{10002, "HYUNDAI", "TUCSON", "TUCSON 1.6 T-GDI (TL) 2015-2018", "Petrol", 1591, 177, 100, 183, 2015, 2018},
		{10003, "HYUNDAI", "TUCSON", "TUCSON 2.0 CRDi (TL) 2015-2018", "Diesel", 1995, 185, 100, 183, 2015, 2018},
		{10004, "HYUNDAI", "TUCSON", "TUCSON 2.5 GDI (NX4) 2021-2024", "Petrol", 2497, 190, 101, 183, 2021, 2024},
		{10005, "HYUNDAI", "TUCSON", "TUCSON 1.6 T-GDI HEV (NX4) 2021-2024", "Hybrid", 1598, 230, 101, 183, 2021, 2024},
		// KIA SPORTAGE
		{20001, "KIA", "SPORTAGE", "SPORTAGE 2.0 MPI (QL) 2016-2018", "Petrol", 1999, 155, 200, 184, 2016, 2018},
		{20002, "KIA", "SPORTAGE", "SPORTAGE 1.6 T-GDI (QL) 2016-2018", "Petrol", 1591, 177, 200, 184, 2016, 2018},
		{20003, "KIA", "SPORTAGE", "SPORTAGE 2.0 CRDi (QL) 2016-2018", "Diesel", 1995, 185, 200, 184, 2016, 2018},
		{20004, "KIA", "SPORTAGE", "SPORTAGE 2.5 GDI (NQ5) 2022-2025", "Petrol", 2497, 190, 201, 184, 2022, 2025},
		{20005, "KIA", "SPORTAGE", "SPORTAGE 1.6 T-GDI HEV (NQ5) 2022-2025", "Hybrid", 1598, 230, 201, 184, 2022, 2025},
		// HYUNDAI ELANTRA
		{10101, "HYUNDAI", "ELANTRA", "ELANTRA 2.0 MPI (AD) 2016-2020", "Petrol", 1999, 147, 110, 183, 2016, 2020},
		{10102, "HYUNDAI", "ELANTRA", "ELANTRA 1.6 Turbo (AD) 2017-2020", "Petrol", 1591, 201, 110, 183, 2017, 2020},
		{10103, "HYUNDAI", "ELANTRA", "ELANTRA 2.0 MPI (CN7) 2021-2025", "Petrol", 1999, 147, 111, 183, 2021, 2025},
		// KIA FORTE / CERATO
		{20101, "KIA", "FORTE", "FORTE 2.0 MPI (BD) 2019-2023", "Petrol", 1999, 147, 210, 184, 2019, 2023},
		{20102, "KIA", "FORTE", "FORTE 1.6 Turbo (BD) 2020-2023", "Petrol", 1591, 201, 210, 184, 2020, 2023},
		// HYUNDAI SONATA
		{10201, "HYUNDAI", "SONATA", "SONATA 2.5 MPI (DN8) 2020-2024", "Petrol", 2497, 191, 120, 183, 2020, 2024},
		{10202, "HYUNDAI", "SONATA", "SONATA 1.6 T-GDI (DN8) 2020-2024", "Petrol", 1598, 180, 120, 183, 2020, 2024},
		// KIA K5
		{20201, "KIA", "K5", "K5 2.5 GDI (DL3) 2021-2025", "Petrol", 2497, 191, 220, 184, 2021, 2025},
		{20202, "KIA", "K5", "K5 1.6 T-GDI (DL3) 2021-2025", "Petrol", 1598, 180, 220, 184, 2021, 2025},
		// HYUNDAI SANTA FE
		{10301, "HYUNDAI", "SANTA FE", "SANTA FE 2.5 GDI (TM) 2019-2023", "Petrol", 2497, 191, 130, 183, 2019, 2023},
		{10302, "HYUNDAI", "SANTA FE", "SANTA FE 2.2 CRDi (TM) 2019-2023", "Diesel", 2199, 202, 130, 183, 2019, 2023},
		// KIA SORENTO
		{20301, "KIA", "SORENTO", "SORENTO 2.5 GDI (MQ4) 2021-2025", "Petrol", 2497, 191, 230, 184, 2021, 2025},
		{20302, "KIA", "SORENTO", "SORENTO 2.2 CRDi (MQ4) 2021-2025", "Diesel", 2199, 202, 230, 184, 2021, 2025},
		// HYUNDAI KONA
		{10401, "HYUNDAI", "KONA", "KONA 2.0 MPI (OS) 2018-2023", "Petrol", 1999, 147, 140, 183, 2018, 2023},
		{10402, "HYUNDAI", "KONA", "KONA 1.6 T-GDI (OS) 2018-2023", "Petrol", 1591, 195, 140, 183, 2018, 2023},
		// KIA SELTOS
		{20401, "KIA", "SELTOS", "SELTOS 2.0 MPI (SP2) 2020-2024", "Petrol", 1999, 146, 240, 184, 2020, 2024},
		{20402, "KIA", "SELTOS", "SELTOS 1.6 T-GDI (SP2) 2020-2024", "Petrol", 1591, 195, 240, 184, 2020, 2024},
	}

	// Insert vehicles
	mustExec(db, "BEGIN")
	for _, v := range vehicles {
		ym := fmt.Sprintf("%d01", v.yearFrom)
		ymEnd := fmt.Sprintf("%d12", v.yearTo)
		mustExec(db, fmt.Sprintf(
			`INSERT INTO vehicle_lookup VALUES ('%s','%s',%d,%d,%d,'%s','%s','%s','%s',%d,%d)`,
			v.make, esc(v.model), v.yearFrom, v.yearTo, v.vid, esc(v.desc), ym, ymEnd, v.fuel, v.cc, v.hp))
		mustExec(db, fmt.Sprintf(
			`INSERT OR IGNORE INTO nhtsa_tecdoc_bridge VALUES ('%s','%s',%d,%d,%d)`,
			v.make, esc(v.model), v.modelId, v.yearFrom, v.yearTo))
	}
	mustExec(db, "COMMIT")

	// ── PLATFORM MAP ──────────────────────────────────────────────────────
	for _, pm := range [][3]string{
		{"TUCSON", "SPORTAGE", "NX4/NQ5"},
		{"ELANTRA", "FORTE", "CN7/BD"},
		{"SONATA", "K5", "DN8/DL3"},
		{"SANTA FE", "SORENTO", "TM/MQ4"},
		{"KONA", "SELTOS", "OS/SP2"},
	} {
		mustExec(db, fmt.Sprintf(`INSERT INTO hk_platform_map VALUES ('%s','%s','%s')`, pm[0], pm[1], pm[2]))
	}

	// ── ASSEMBLY GROUPS (TecDoc-style) ────────────────────────────────────
	type asmGroup struct {
		id   int
		name string
	}
	groups := []asmGroup{
		{10100, "Engine Oil & Filters"},
		{10200, "Air Intake & Filters"},
		{10300, "Ignition System"},
		{10400, "Cooling System"},
		{10500, "Fuel System"},
		{10600, "Exhaust System"},
		{10700, "Timing / Drive Belt"},
		{10800, "Engine Mounts"},
		{10900, "Sensors"},
		{20100, "Front Brake System"},
		{20200, "Rear Brake System"},
		{20300, "Brake Hydraulics"},
		{30100, "Front Suspension"},
		{30200, "Rear Suspension"},
		{30300, "Steering"},
		{40100, "Headlights"},
		{40200, "Rear Lights"},
		{40300, "Body Panels"},
		{40400, "Mirrors & Glass"},
		{40500, "Wipers"},
		{50100, "Clutch"},
		{50200, "Drive Shafts"},
		{50300, "Transmission Mounts"},
		{60100, "HVAC / Climate"},
		{60200, "Cabin Filter & Blower"},
		{70100, "Electrical"},
		{70200, "ABS / Wheel Speed"},
	}

	// ── PARTS ──────────────────────────────────────────────────────────────
	type partDef struct {
		artId     int
		artNum    string
		desc      string
		brand     string
		suppId    int
		groupId   int
		groupName string
		oems      []string
		// which vehicle IDs this part fits (nil = all)
		vehicles []int
		ccDep    bool
	}

	// Helper to get all vehicle IDs from vehicles
	allVids := func() []int {
		var ids []int
		for _, v := range vehicles {
			ids = append(ids, v.vid)
		}
		return ids
	}

	// Hyundai-only and Kia-only vehicle IDs
	hyundaiVids := func() []int {
		var ids []int
		for _, v := range vehicles {
			if v.manuId == 183 {
				ids = append(ids, v.vid)
			}
		}
		return ids
	}
	kiaVids := func() []int {
		var ids []int
		for _, v := range vehicles {
			if v.manuId == 184 {
				ids = append(ids, v.vid)
			}
		}
		return ids
	}

	// Tucson variants only
	tucsonVids := []int{10001, 10002, 10003, 10004, 10005}
	sportageVids := []int{20001, 20002, 20003, 20004, 20005}
	elantraVids := []int{10101, 10102, 10103}
	forteVids := []int{20101, 20102}
	sonataVids := []int{10201, 10202}
	k5Vids := []int{20201, 20202}
	santaFeVids := []int{10301, 10302}
	sorentoVids := []int{20301, 20302}
	konaVids := []int{10401, 10402}
	seltosVids := []int{20401, 20402}

	_ = groups

	parts := []partDef{
		// ── ENGINE OIL & FILTERS (10100) ──
		{100001, "26300-35505", "FILTER ASSY-ENGINE OIL", "HYUNDAI/KIA", 10, 10100, "Engine Oil & Filters",
			[]string{"26300-35505", "26300-35504", "26300-35503"}, allVids(), false},
		{100002, "OC 205", "FILTER ASSY-ENGINE OIL", "MAHLE", 11, 10100, "Engine Oil & Filters",
			[]string{"26300-35505"}, allVids(), false},
		{100003, "W 811/80", "FILTER ASSY-ENGINE OIL", "MANN-FILTER", 12, 10100, "Engine Oil & Filters",
			[]string{"26300-35505"}, allVids(), false},
		{100004, "0 986 AF1 014", "FILTER ASSY-ENGINE OIL", "BOSCH", 13, 10100, "Engine Oil & Filters",
			[]string{"26300-35505"}, allVids(), false},
		{100005, "LS 932", "FILTER ASSY-ENGINE OIL", "PURFLUX", 14, 10100, "Engine Oil & Filters",
			[]string{"26300-35505"}, allVids(), false},
		{100006, "26300-35530", "FILTER ASSY-ENGINE OIL", "HYUNDAI/KIA", 10, 10100, "Engine Oil & Filters",
			[]string{"26300-35530"}, allVids(), true},

		// ── AIR INTAKE & FILTERS (10200) ──
		{100101, "28113-D3100", "FILTER-AIR CLEANER", "HYUNDAI/KIA", 10, 10200, "Air Intake & Filters",
			[]string{"28113-D3100"}, append(tucsonVids, sportageVids...), false},
		{100102, "C 26 013", "FILTER-AIR CLEANER", "MANN-FILTER", 12, 10200, "Air Intake & Filters",
			[]string{"28113-D3100"}, append(tucsonVids, sportageVids...), false},
		{100103, "LX 3778", "FILTER-AIR CLEANER", "MAHLE", 11, 10200, "Air Intake & Filters",
			[]string{"28113-D3100"}, append(tucsonVids, sportageVids...), false},
		{100104, "28113-F2100", "Air Filter Element", "HYUNDAI/KIA", 10, 10200, "Air Intake & Filters",
			[]string{"28113-F2100"}, append(elantraVids, forteVids...), false},
		{100105, "28113-L1100", "Air Filter Element", "HYUNDAI/KIA", 10, 10200, "Air Intake & Filters",
			[]string{"28113-L1100"}, append(sonataVids, k5Vids...), false},
		{100106, "28113-S8100", "Air Filter Element", "HYUNDAI/KIA", 10, 10200, "Air Intake & Filters",
			[]string{"28113-S8100"}, append(santaFeVids, sorentoVids...), false},

		// ── IGNITION SYSTEM (10300) ──
		{100201, "27301-2B100", "COIL ASSY-IGNITION", "HYUNDAI/KIA", 10, 10300, "Ignition System",
			[]string{"27301-2B100"}, allVids(), true},
		{100202, "U5055", "Ignition Coil", "NGK", 15, 10300, "Ignition System",
			[]string{"27301-2B100"}, allVids(), true},
		{100203, "18855-10080", "Spark Plug LZKR6B-10E", "HYUNDAI/KIA", 10, 10300, "Ignition System",
			[]string{"18855-10080"}, allVids(), true},
		{100204, "LZKR6B-10E", "Spark Plug", "NGK", 15, 10300, "Ignition System",
			[]string{"18855-10080"}, allVids(), true},
		{100205, "K16TT", "Spark Plug", "DENSO", 16, 10300, "Ignition System",
			[]string{"18855-10080"}, allVids(), true},

		// ── COOLING SYSTEM (10400) ──
		{100301, "25100-2E100", "PUMP ASSY-WATER", "HYUNDAI/KIA", 10, 10400, "Cooling System",
			[]string{"25100-2E100"}, allVids(), true},
		{100302, "VKPC 85316", "Water Pump", "SKF", 17, 10400, "Cooling System",
			[]string{"25100-2E100"}, allVids(), true},
		{100303, "25500-2B100", "Thermostat Assembly", "HYUNDAI/KIA", 10, 10400, "Cooling System",
			[]string{"25500-2B100"}, allVids(), true},
		{100304, "25310-2S500", "RADIATOR ASSY", "HYUNDAI/KIA", 10, 10400, "Cooling System",
			[]string{"25310-2S500"}, append(tucsonVids, sportageVids...), true},
		{100305, "DRM40034", "Radiator Assembly", "DENSO", 16, 10400, "Cooling System",
			[]string{"25310-2S500"}, append(tucsonVids, sportageVids...), true},
		{100306, "25380-2S500", "Radiator Fan Motor", "HYUNDAI/KIA", 10, 10400, "Cooling System",
			[]string{"25380-2S500"}, append(tucsonVids, sportageVids...), true},
		{100307, "97133-D3000", "FILTER-AIR", "HYUNDAI/KIA", 10, 60200, "Cabin Filter & Blower",
			[]string{"97133-D3000"}, allVids(), true},

		// ── FUEL SYSTEM (10500) ──
		{100401, "31112-D3000", "PUMP MODULE ASSY-FUEL", "HYUNDAI/KIA", 10, 10500, "Fuel System",
			[]string{"31112-D3000"}, append(tucsonVids, sportageVids...), true},
		{100402, "35310-2S000", "INJECTORASSY-FUEL", "HYUNDAI/KIA", 10, 10500, "Fuel System",
			[]string{"35310-2S000"}, allVids(), true},
		{100403, "0 280 158 257", "Fuel Injector", "BOSCH", 13, 10500, "Fuel System",
			[]string{"35310-2S000"}, allVids(), true},

		// ── EXHAUST SYSTEM (10600) ──
		{100501, "28510-2S500", "Catalytic Converter", "HYUNDAI/KIA", 10, 10600, "Exhaust System",
			[]string{"28510-2S500"}, append(tucsonVids, sportageVids...), true},
		{100502, "28410-2B100", "EGR Valve", "HYUNDAI/KIA", 10, 10600, "Exhaust System",
			[]string{"28410-2B100"}, allVids(), true},
		{100503, "28830-2U000", "Exhaust Muffler Rear", "HYUNDAI/KIA", 10, 10600, "Exhaust System",
			[]string{"28830-2U000"}, append(tucsonVids, sportageVids...), true},

		// ── TIMING / DRIVE BELT (10700) ──
		{100601, "24312-2B000", "Timing Chain Kit", "HYUNDAI/KIA", 10, 10700, "Timing / Drive Belt",
			[]string{"24312-2B000"}, allVids(), true},
		{100602, "25212-2B020", "Serpentine Belt", "HYUNDAI/KIA", 10, 10700, "Timing / Drive Belt",
			[]string{"25212-2B020"}, allVids(), true},
		{100603, "6PK2050", "Serpentine Belt", "GATES", 18, 10700, "Timing / Drive Belt",
			[]string{"25212-2B020"}, allVids(), true},

		// ── ENGINE MOUNTS (10800) ──
		{100701, "21810-2S000", "BRACKET ASSY-ENGINE MTG", "HYUNDAI/KIA", 10, 10800, "Engine Mounts",
			[]string{"21810-2S000"}, append(tucsonVids, sportageVids...), true},
		{100702, "21930-2S200", "Engine Mount Rear", "HYUNDAI/KIA", 10, 10800, "Engine Mounts",
			[]string{"21930-2S200"}, append(tucsonVids, sportageVids...), true},

		// ── SENSORS (10900) ──
		{100801, "39210-2B100", "SENSOR ASSY-OXYGEN RR", "HYUNDAI/KIA", 10, 10900, "Sensors",
			[]string{"39210-2B100"}, allVids(), true},
		{100802, "0 258 006 537", "Oxygen Sensor", "BOSCH", 13, 10900, "Sensors",
			[]string{"39210-2B100"}, allVids(), true},
		{100803, "39350-2B100", "Crankshaft Position Sensor", "HYUNDAI/KIA", 10, 10900, "Sensors",
			[]string{"39350-2B100"}, allVids(), true},
		{100804, "39180-2B000", "Camshaft Position Sensor", "HYUNDAI/KIA", 10, 10900, "Sensors",
			[]string{"39180-2B000"}, allVids(), true},
		{100805, "39450-2S500", "Vehicle Speed Sensor", "HYUNDAI/KIA", 10, 10900, "Sensors",
			[]string{"39450-2S500"}, allVids(), false},

		// ── FRONT BRAKE SYSTEM (20100) ──
		{200001, "58101-D3A70", "Brake Pad Set Front", "HYUNDAI/KIA", 10, 20100, "Front Brake System",
			[]string{"58101-D3A70"}, append(tucsonVids, sportageVids...), false},
		{200002, "GDB 3627", "Brake Pad Set Front", "TRW", 19, 20100, "Front Brake System",
			[]string{"58101-D3A70"}, append(tucsonVids, sportageVids...), false},
		{200003, "P 30 069", "Brake Pad Set Front", "BREMBO", 20, 20100, "Front Brake System",
			[]string{"58101-D3A70"}, append(tucsonVids, sportageVids...), false},
		{200004, "51712-D3100", "Brake Disc Front", "HYUNDAI/KIA", 10, 20100, "Front Brake System",
			[]string{"51712-D3100"}, append(tucsonVids, sportageVids...), false},
		{200005, "09.C399.13", "Brake Disc Front", "BREMBO", 20, 20100, "Front Brake System",
			[]string{"51712-D3100"}, append(tucsonVids, sportageVids...), false},
		{200006, "58101-F2A00", "Brake Pad Set Front", "HYUNDAI/KIA", 10, 20100, "Front Brake System",
			[]string{"58101-F2A00"}, append(elantraVids, forteVids...), false},
		{200007, "51712-F2100", "Brake Disc Front", "HYUNDAI/KIA", 10, 20100, "Front Brake System",
			[]string{"51712-F2100"}, append(elantraVids, forteVids...), false},
		{200008, "58101-S8A70", "Brake Pad Set Front", "HYUNDAI/KIA", 10, 20100, "Front Brake System",
			[]string{"58101-S8A70"}, append(santaFeVids, sorentoVids...), false},

		// ── REAR BRAKE SYSTEM (20200) ──
		{200101, "58302-D3A70", "Brake Pad Set Rear", "HYUNDAI/KIA", 10, 20200, "Rear Brake System",
			[]string{"58302-D3A70"}, append(tucsonVids, sportageVids...), false},
		{200102, "GDB 3628", "Brake Pad Set Rear", "TRW", 19, 20200, "Rear Brake System",
			[]string{"58302-D3A70"}, append(tucsonVids, sportageVids...), false},
		{200103, "58411-D3100", "Brake Disc Rear", "HYUNDAI/KIA", 10, 20200, "Rear Brake System",
			[]string{"58411-D3100"}, append(tucsonVids, sportageVids...), false},
		{200104, "58411-F2100", "Brake Disc Rear", "HYUNDAI/KIA", 10, 20200, "Rear Brake System",
			[]string{"58411-F2100"}, append(elantraVids, forteVids...), false},

		// ── BRAKE HYDRAULICS (20300) ──
		{200201, "58510-2S300", "Brake Master Cylinder", "HYUNDAI/KIA", 10, 20300, "Brake Hydraulics",
			[]string{"58510-2S300"}, append(tucsonVids, sportageVids...), false},
		{200202, "58732-2S000", "Brake Hose Front", "HYUNDAI/KIA", 10, 20300, "Brake Hydraulics",
			[]string{"58732-2S000"}, allVids(), false},

		// ── FRONT SUSPENSION (30100) ──
		{300001, "54651-D3000", "Shock Absorber Front", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"54651-D3000"}, append(tucsonVids, sportageVids...), false},
		{300002, "KYB 339403", "Shock Absorber Front", "KYB", 21, 30100, "Front Suspension",
			[]string{"54651-D3000"}, append(tucsonVids, sportageVids...), false},
		{300003, "54530-D3000", "Ball Joint Lower", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"54530-D3000"}, append(tucsonVids, sportageVids...), false},
		{300004, "54500-D3000", "Control Arm Lower Front Left", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"54500-D3000"}, append(tucsonVids, sportageVids...), false},
		{300005, "54501-D3000", "Control Arm Lower Front Right", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"54501-D3000"}, append(tucsonVids, sportageVids...), false},
		{300006, "54830-D3000", "Stabilizer Link Front", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"54830-D3000"}, append(tucsonVids, sportageVids...), false},
		{300007, "JTS1171", "Stabilizer Link Front", "TRW", 19, 30100, "Front Suspension",
			[]string{"54830-D3000"}, append(tucsonVids, sportageVids...), false},
		{300008, "51720-D3000", "Wheel Bearing Front", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"51720-D3000"}, append(tucsonVids, sportageVids...), false},

		// ── REAR SUSPENSION (30200) ──
		{300101, "55300-D3000", "Shock Absorber Rear", "HYUNDAI/KIA", 10, 30200, "Rear Suspension",
			[]string{"55300-D3000"}, append(tucsonVids, sportageVids...), false},
		{300102, "KYB 349209", "Shock Absorber Rear", "KYB", 21, 30200, "Rear Suspension",
			[]string{"55300-D3000"}, append(tucsonVids, sportageVids...), false},
		{300103, "55530-D3000", "Stabilizer Link Rear", "HYUNDAI/KIA", 10, 30200, "Rear Suspension",
			[]string{"55530-D3000"}, append(tucsonVids, sportageVids...), false},

		// ── STEERING (30300) ──
		{300201, "56820-D3000", "END ASSY-TIE ROD,LH", "HYUNDAI/KIA", 10, 30300, "Steering",
			[]string{"56820-D3000"}, append(tucsonVids, sportageVids...), false},
		{300202, "JTE1660", "Tie Rod End Outer", "TRW", 19, 30300, "Steering",
			[]string{"56820-D3000"}, append(tucsonVids, sportageVids...), false},
		{300203, "57724-D3000", "Steering Rack Boot", "HYUNDAI/KIA", 10, 30300, "Steering",
			[]string{"57724-D3000"}, append(tucsonVids, sportageVids...), false},

		// ── HEADLIGHTS (40100) ──
		{400001, "92101-D3100", "LAMP ASSY-HEAD,LH", "HYUNDAI/KIA", 10, 40100, "Headlights",
			[]string{"92101-D3100"}, tucsonVids, false},
		{400002, "92102-D3100", "Headlight Assembly Right", "HYUNDAI/KIA", 10, 40100, "Headlights",
			[]string{"92102-D3100"}, tucsonVids, false},
		{400003, "92101-Q5100", "Headlight Assembly Left", "HYUNDAI/KIA", 10, 40100, "Headlights",
			[]string{"92101-Q5100"}, sportageVids, false},
		{400004, "92102-Q5100", "Headlight Assembly Right", "HYUNDAI/KIA", 10, 40100, "Headlights",
			[]string{"92102-Q5100"}, sportageVids, false},
		{400005, "92101-F2020", "Headlight Assembly Left", "HYUNDAI/KIA", 10, 40100, "Headlights",
			[]string{"92101-F2020"}, elantraVids, false},
		{400006, "92102-F2020", "Headlight Assembly Right", "HYUNDAI/KIA", 10, 40100, "Headlights",
			[]string{"92102-F2020"}, elantraVids, false},

		// ── REAR LIGHTS (40200) ──
		{400101, "92401-D3100", "LAMP ASSY-REAR COMB OUTSIDE,LH", "HYUNDAI/KIA", 10, 40200, "Rear Lights",
			[]string{"92401-D3100"}, tucsonVids, false},
		{400102, "92402-D3100", "Tail Light Right", "HYUNDAI/KIA", 10, 40200, "Rear Lights",
			[]string{"92402-D3100"}, tucsonVids, false},
		{400103, "L0K2AV4406B6", "Tail Light Left", "DEPO", 22, 40200, "Rear Lights",
			[]string{"92401-D3100"}, tucsonVids, false},
		{400104, "92403-BD010", "Tail Light Left", "HYUNDAI/KIA", 10, 40200, "Rear Lights",
			[]string{"92403-BD010"}, forteVids, false},

		// ── BODY PANELS (40300) ──
		{400201, "86511-D3100", "COVER-FR BUMPER UPR", "HYUNDAI/KIA", 10, 40300, "Body Panels",
			[]string{"86511-D3100"}, tucsonVids, false},
		{400202, "86611-D3100", "Rear Bumper Cover", "HYUNDAI/KIA", 10, 40300, "Body Panels",
			[]string{"86611-D3100"}, tucsonVids, false},
		{400203, "86350-D3100", "GRILLE ASSY-RADIATOR", "HYUNDAI/KIA", 10, 40300, "Body Panels",
			[]string{"86350-D3100"}, tucsonVids, false},
		{400204, "66311-D3100", "Fender Left", "HYUNDAI/KIA", 10, 40300, "Body Panels",
			[]string{"66311-D3100"}, tucsonVids, false},
		{400205, "66321-D3100", "Fender Right", "HYUNDAI/KIA", 10, 40300, "Body Panels",
			[]string{"66321-D3100"}, tucsonVids, false},
		{400206, "66400-D3100", "PANEL ASSY-HOOD", "HYUNDAI/KIA", 10, 40300, "Body Panels",
			[]string{"66400-D3100"}, tucsonVids, false},
		{400207, "86511-Q5000", "Front Bumper Cover", "HYUNDAI/KIA", 10, 40300, "Body Panels",
			[]string{"86511-Q5000"}, sportageVids, false},

		// ── MIRRORS & GLASS (40400) ──
		{400301, "87610-D3100", "Door Mirror Left Electric", "HYUNDAI/KIA", 10, 40400, "Mirrors & Glass",
			[]string{"87610-D3100"}, tucsonVids, false},
		{400302, "87620-D3100", "Door Mirror Right Electric", "HYUNDAI/KIA", 10, 40400, "Mirrors & Glass",
			[]string{"87620-D3100"}, tucsonVids, false},

		// ── WIPERS (40500) ──
		{400401, "98350-D3100", "Wiper Blade Set", "HYUNDAI/KIA", 10, 40500, "Wipers",
			[]string{"98350-D3100"}, append(tucsonVids, sportageVids...), false},
		{400402, "A 297 S", "Wiper Blade Set", "BOSCH", 13, 40500, "Wipers",
			[]string{"98350-D3100"}, append(tucsonVids, sportageVids...), false},
		{400403, "98100-D3100", "Wiper Motor Front", "HYUNDAI/KIA", 10, 40500, "Wipers",
			[]string{"98100-D3100"}, append(tucsonVids, sportageVids...), false},

		// ── CLUTCH (50100) ──
		{500001, "41100-2D100", "Clutch Kit 3pc", "HYUNDAI/KIA", 10, 50100, "Clutch",
			[]string{"41100-2D100"}, allVids(), false},
		{500002, "3000 990 264", "Clutch Kit", "SACHS", 23, 50100, "Clutch",
			[]string{"41100-2D100"}, allVids(), false},

		// ── DRIVE SHAFTS (50200) ──
		{500101, "49500-D3600", "Drive Shaft Front Left", "HYUNDAI/KIA", 10, 50200, "Drive Shafts",
			[]string{"49500-D3600"}, append(tucsonVids, sportageVids...), false},
		{500102, "49501-D3600", "Drive Shaft Front Right", "HYUNDAI/KIA", 10, 50200, "Drive Shafts",
			[]string{"49501-D3600"}, append(tucsonVids, sportageVids...), false},
		{500103, "49590-D3000", "CV Joint Kit", "HYUNDAI/KIA", 10, 50200, "Drive Shafts",
			[]string{"49590-D3000"}, append(tucsonVids, sportageVids...), false},

		// ── TRANSMISSION MOUNTS (50300) ──
		{500201, "21830-2S200", "Transmission Mount", "HYUNDAI/KIA", 10, 50300, "Transmission Mounts",
			[]string{"21830-2S200"}, append(tucsonVids, sportageVids...), false},

		// ── HVAC (60100) ──
		{600001, "97701-D3000", "COMPRESSOR ASSY", "HYUNDAI/KIA", 10, 60100, "HVAC / Climate",
			[]string{"97701-D3000"}, append(tucsonVids, sportageVids...), false},
		{600002, "97606-D3000", "A/C Condenser", "HYUNDAI/KIA", 10, 60100, "HVAC / Climate",
			[]string{"97606-D3000"}, append(tucsonVids, sportageVids...), false},

		// ── CABIN FILTER & BLOWER (60200) ──
		// NOTE: 97133-D3000 is already seeded as ID 100307 in Cooling section (allVids). Skip dupe here.
		{600102, "CUK 26 013", "Cabin Filter", "MANN-FILTER", 12, 60200, "Cabin Filter & Blower",
			[]string{"97133-D3000"}, append(tucsonVids, sportageVids...), false},
		{600103, "97113-D3000", "Heater Core", "HYUNDAI/KIA", 10, 60200, "Cabin Filter & Blower",
			[]string{"97113-D3000"}, append(tucsonVids, sportageVids...), false},
		{600104, "97115-D3000", "Blower Motor", "HYUNDAI/KIA", 10, 60200, "Cabin Filter & Blower",
			[]string{"97115-D3000"}, append(tucsonVids, sportageVids...), false},
		{600105, "97133-F2000", "Cabin Filter", "HYUNDAI/KIA", 10, 60200, "Cabin Filter & Blower",
			[]string{"97133-F2000"}, append(elantraVids, forteVids...), false},

		// ── ELECTRICAL (70100) ──
		{700001, "18640-11080", "Bulb H7 55W Low Beam", "HYUNDAI/KIA", 10, 70100, "Electrical",
			[]string{"18640-11080"}, allVids(), false},
		{700002, "64211", "Bulb H7 55W", "OSRAM", 24, 70100, "Electrical",
			[]string{"18640-11080"}, allVids(), false},
		{700003, "12972PRC1", "Bulb H7 55W", "PHILIPS", 25, 70100, "Electrical",
			[]string{"18640-11080"}, allVids(), false},
		{700004, "96610-D3100", "Horn Assembly", "HYUNDAI/KIA", 10, 70100, "Electrical",
			[]string{"96610-D3100"}, allVids(), false},
		{700005, "37300-2B100", "Alternator", "HYUNDAI/KIA", 10, 70100, "Electrical",
			[]string{"37300-2B100"}, allVids(), true},
		{700006, "36100-2B100", "Starter Motor", "HYUNDAI/KIA", 10, 70100, "Electrical",
			[]string{"36100-2B100"}, allVids(), true},

		// ── ABS / WHEEL SPEED (70200) ──
		{700101, "59830-D3000", "ABS Speed Sensor Front", "HYUNDAI/KIA", 10, 70200, "ABS / Wheel Speed",
			[]string{"59830-D3000"}, append(tucsonVids, sportageVids...), false},
		{700102, "59930-D3000", "ABS Speed Sensor Rear", "HYUNDAI/KIA", 10, 70200, "ABS / Wheel Speed",
			[]string{"59930-D3000"}, append(tucsonVids, sportageVids...), false},

		// More model-specific parts for Kona/Seltos/Sonata/SantaFe/Sorento
		{800001, "97133-J9000", "Cabin Filter", "HYUNDAI/KIA", 10, 60200, "Cabin Filter & Blower",
			[]string{"97133-J9000"}, append(konaVids, seltosVids...), false},
		{800002, "54651-J9000", "Shock Absorber Front", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"54651-J9000"}, append(konaVids, seltosVids...), false},
		{800003, "58101-J9A00", "Brake Pad Set Front", "HYUNDAI/KIA", 10, 20100, "Front Brake System",
			[]string{"58101-J9A00"}, append(konaVids, seltosVids...), false},
		{800004, "54651-L1000", "Shock Absorber Front", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"54651-L1000"}, append(sonataVids, k5Vids...), false},
		{800005, "58101-L0A00", "Brake Pad Set Front", "HYUNDAI/KIA", 10, 20100, "Front Brake System",
			[]string{"58101-L0A00"}, append(sonataVids, k5Vids...), false},
		{800006, "54651-S1000", "Shock Absorber Front", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"54651-S1000"}, append(santaFeVids, sorentoVids...), false},

		// ── MOBILITY KIT / TIRE PRESSURE (70200) ──
		{800101, "52933-1P000", "MOBILITY KIT-TIRE", "HYUNDAI/KIA", 10, 70200, "ABS / Wheel Speed",
			[]string{"52933-1P000", "52933-B1100", "52933-3X300"}, allVids(), false},
		{800102, "52933-D4100", "MOBILITY KIT-TIRE", "HYUNDAI/KIA", 10, 70200, "ABS / Wheel Speed",
			[]string{"52933-D4100"}, append(tucsonVids, sportageVids...), false},
		{800103, "52933-3X300", "MOBILITY KIT-TIRE", "HYUNDAI/KIA", 10, 70200, "ABS / Wheel Speed",
			[]string{"52933-3X300", "52933-1P000"}, allVids(), false},
		{800104, "SE10004", "MOBILITY KIT-TIRE", "SCHRADER", 26, 70200, "ABS / Wheel Speed",
			[]string{"52933-1P000", "52933-D4100"}, allVids(), false},

		// ── ADDITIONAL COMMON SEARCHES ──
		// Window regulator
		{800201, "82401-D3010", "Window Regulator Front Left", "HYUNDAI/KIA", 10, 70100, "Electrical",
			[]string{"82401-D3010"}, tucsonVids, false},
		{800202, "82402-D3010", "Window Regulator Front Right", "HYUNDAI/KIA", 10, 70100, "Electrical",
			[]string{"82402-D3010"}, tucsonVids, false},
		// Side mirror
		{800301, "87610-D3520", "Side Mirror Left (w/ Signal)", "HYUNDAI/KIA", 10, 40400, "Mirrors & Glass",
			[]string{"87610-D3520"}, tucsonVids, false},
		// Wheel hub / bearing
		{800401, "51750-D3000", "Wheel Hub Front", "HYUNDAI/KIA", 10, 30100, "Front Suspension",
			[]string{"51750-D3000"}, append(tucsonVids, sportageVids...), false},
		{800402, "52730-D3100", "Wheel Hub Rear", "HYUNDAI/KIA", 10, 30200, "Rear Suspension",
			[]string{"52730-D3100"}, append(tucsonVids, sportageVids...), false},
		// Radiator hose
		{800501, "25411-D3100", "Radiator Upper Hose", "HYUNDAI/KIA", 10, 10400, "Cooling System",
			[]string{"25411-D3100"}, tucsonVids, true},
		{800502, "25412-D3100", "Radiator Lower Hose", "HYUNDAI/KIA", 10, 10400, "Cooling System",
			[]string{"25412-D3100"}, tucsonVids, true},
	}
	_ = hyundaiVids
	_ = kiaVids

	// Insert all parts
	log.Printf("Seeding %d parts across %d vehicles...", len(parts), len(vehicles))
	mustExec(db, "BEGIN")

	vMap := map[int]vehicle{}
	for _, v := range vehicles {
		vMap[v.vid] = v
	}

	for _, p := range parts {
		for _, vid := range p.vehicles {
			vi := vMap[vid]
			ym := fmt.Sprintf("%d01", vi.yearFrom)
			ymEnd := fmt.Sprintf("%d12", vi.yearTo)
			mustExec(db, fmt.Sprintf(
				`INSERT OR IGNORE INTO hk_parts_cache VALUES (%d,%d,%d,'%s','%s',%d,'%s','%s','%s',%d,%d,'%s','%s','%s','%s',%d,%d)`,
				vid, p.artId, p.groupId, esc(p.artNum), esc(p.desc), p.suppId,
				esc(p.brand), esc(p.groupName), esc(vi.desc),
				vi.manuId, vi.modelId, vi.model, ym, ymEnd, vi.fuel, vi.cc, vi.hp))
		}

		// OEM cross-refs
		mfr := "HYUNDAI/KIA"
		for _, oem := range p.oems {
			mustExec(db, fmt.Sprintf(`INSERT INTO articlecrosses VALUES (%d,'%s','%s')`, p.artId, oem, mfr))
			norm := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(oem, "-", ""), " ", ""))
			mustExec(db, fmt.Sprintf(
				`INSERT INTO oem_search_index VALUES ('%s','%s',%d,'articlecrosses','%s','%s','%s','%s')`,
				oem, norm, p.artId, mfr, esc(p.brand), esc(p.artNum), esc(p.desc)))
		}
	}

	mustExec(db, "COMMIT")

	// Create indexes
	for _, idx := range []string{
		"CREATE INDEX idx_hk_article ON hk_parts_cache(legacyArticleId)",
		"CREATE INDEX idx_hk_artnum ON hk_parts_cache(articleNumber)",
		"CREATE INDEX idx_hk_desc ON hk_parts_cache(genericArticleDesc)",
		"CREATE INDEX idx_hk_brand ON hk_parts_cache(dataSupplierId)",
		"CREATE INDEX idx_hk_model ON hk_parts_cache(manuId, modelId)",
		"CREATE INDEX idx_hk_asm ON hk_parts_cache(assemblyGroupNodeId)",
		"CREATE INDEX idx_oem_norm ON oem_search_index(normalized)",
		"CREATE INDEX idx_oem_article ON oem_search_index(legacyArticleId)",
		"CREATE INDEX idx_cross_article ON articlecrosses(legacyArticleId)",
		"CREATE INDEX idx_cross_oem ON articlecrosses(oemNumber)",
		"CREATE INDEX idx_bridge ON nhtsa_tecdoc_bridge(nhtsa_make, nhtsa_model, year_from, year_to)",
		"CREATE INDEX idx_vl_lookup ON vehicle_lookup(nhtsa_make, nhtsa_model, year_from, year_to)",
		"CREATE INDEX idx_vl_ltid ON vehicle_lookup(linkageTargetId)",
	} {
		mustExec(db, idx)
	}

	// Stats
	var partCount, vehCount, oemCount int
	db.QueryRow("SELECT COUNT(DISTINCT legacyArticleId) FROM hk_parts_cache").Scan(&partCount)
	db.QueryRow("SELECT COUNT(DISTINCT linkingTargetId) FROM hk_parts_cache").Scan(&vehCount)
	db.QueryRow("SELECT COUNT(*) FROM articlecrosses").Scan(&oemCount)

	if fi, err := os.Stat(outPath); err == nil {
		log.Printf("✓ Seed complete → %s (%.1f MB)", outPath, float64(fi.Size())/1024/1024)
	}
	log.Printf("  Parts: %d | Vehicles: %d | OEM cross-refs: %d", partCount, vehCount, oemCount)
}

func esc(s string) string { return strings.ReplaceAll(s, "'", "''") }
func mustExec(db *sql.DB, s string) {
	if _, err := db.Exec(s); err != nil {
		log.Fatalf("SQL error: %v\nSQL: %s", err, s)
	}
}

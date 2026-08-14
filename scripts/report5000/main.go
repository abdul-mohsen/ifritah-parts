package main

// 5000-Part Detailed Report
// Tests 5000 real Hyundai/KIA OEM part numbers against the parts engine API.
// Sources: all local DB parts + scraped from hyundaipartsdeal.com / kiapartsnow.com

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	baseURL    = "http://localhost:8080"
	targetSize = 5000
	dbPath     = "data/hk_parts.db"
)

type SmartSearchResponse struct {
	Query          string        `json:"query"`
	Results        []SmartResult `json:"results"`
	Total          int           `json:"total"`
	SearchStrategy string        `json:"searchStrategy"`
}

type SmartResult struct {
	LegacyArticleId         int                `json:"legacyArticleId"`
	ArticleNumber           string             `json:"articleNumber"`
	Description             string             `json:"description"`
	BrandName               string             `json:"brandName"`
	Substitutions           []SubstitutionPart `json:"substitutions"`
	AftermarketAlternatives []AftermarketPart  `json:"aftermarketAlternatives"`
	Compatibility           []string           `json:"compatibility"`
}

type AftermarketPart struct {
	PartNumber  string `json:"partNumber"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

type SubstitutionPart struct {
	PartNumber  string `json:"partNumber"`
	Description string `json:"description"`
	Make        string `json:"make"`
}

type PartInfo struct {
	PartNumber string
	Source     string
	Category   string
}

// Helper to generate paginated URLs for both sites
func hyundaiPages(slug, cat string, pages int) []struct{ URL, Category, Source string } {
	var out []struct{ URL, Category, Source string }
	out = append(out, struct{ URL, Category, Source string }{
		"https://www.hyundaipartsdeal.com/oem-hyundai-" + slug + ".html", cat, "hyundai"})
	for p := 2; p <= pages; p++ {
		out = append(out, struct{ URL, Category, Source string }{
			fmt.Sprintf("https://www.hyundaipartsdeal.com/oem-hyundai-%s.html?p=%d", slug, p), cat, "hyundai"})
	}
	return out
}

func kiaPages(slug, cat string, pages int) []struct{ URL, Category, Source string } {
	var out []struct{ URL, Category, Source string }
	out = append(out, struct{ URL, Category, Source string }{
		"https://www.kiapartsnow.com/oem-kia-" + slug + ".html", cat, "kia"})
	for p := 2; p <= pages; p++ {
		out = append(out, struct{ URL, Category, Source string }{
			fmt.Sprintf("https://www.kiapartsnow.com/oem-kia-%s.html?p=%d", slug, p), cat, "kia"})
	}
	return out
}

type catPage struct {
	URL      string
	Category string
	Source   string
}

func buildCategoryPages() []catPage {
	var pages []catPage
	add := func(entries []struct{ URL, Category, Source string }) {
		for _, e := range entries {
			pages = append(pages, catPage{e.URL, e.Category, e.Source})
		}
	}

	// ═══════════════════════════════════════════════════
	// HYUNDAI — Engine (deep pagination)
	add(hyundaiPages("oil_filter", "Oil Filter", 8))
	add(hyundaiPages("air_filter", "Air Filter", 8))
	add(hyundaiPages("spark_plug", "Spark Plug", 6))
	add(hyundaiPages("water_pump", "Water Pump", 6))
	add(hyundaiPages("ignition_coil", "Ignition Coil", 4))
	add(hyundaiPages("fuel_pump", "Fuel Pump", 5))
	add(hyundaiPages("fuel_injector", "Fuel Injector", 5))
	add(hyundaiPages("oxygen_sensor", "Oxygen Sensor", 5))
	add(hyundaiPages("catalytic_converter", "Catalytic Converter", 5))
	add(hyundaiPages("starter", "Starter Motor", 4))
	add(hyundaiPages("thermostat", "Thermostat", 4))
	add(hyundaiPages("valve_cover_gasket", "Valve Cover Gasket", 3))
	add(hyundaiPages("timing_belt", "Timing Belt", 4))
	add(hyundaiPages("timing_chain", "Timing Chain", 3))
	add(hyundaiPages("rod_bearing", "Rod Bearing", 3))
	add(hyundaiPages("engine_control_module", "Engine Control Module", 3))
	add(hyundaiPages("piston", "Piston", 3))
	add(hyundaiPages("crankshaft", "Crankshaft", 2))
	add(hyundaiPages("camshaft", "Camshaft", 3))
	add(hyundaiPages("oil_pan", "Oil Pan", 3))
	add(hyundaiPages("intake_manifold", "Intake Manifold", 3))
	add(hyundaiPages("exhaust_manifold", "Exhaust Manifold", 3))
	add(hyundaiPages("serpentine_belt", "Serpentine Belt", 3))
	add(hyundaiPages("drive_belt", "Drive Belt", 3))
	add(hyundaiPages("engine_mount", "Engine Mount", 4))
	add(hyundaiPages("turbocharger", "Turbocharger", 3))

	// HYUNDAI — Chassis & Brakes
	add(hyundaiPages("brake_pad_set", "Brake Pad Set", 8))
	add(hyundaiPages("brake_disc", "Brake Disc", 8))
	add(hyundaiPages("shock_absorber", "Shock Absorber", 6))
	add(hyundaiPages("coil_springs", "Coil Springs", 5))
	add(hyundaiPages("control_arm", "Control Arm", 5))
	add(hyundaiPages("brake_caliper", "Brake Caliper", 5))
	add(hyundaiPages("wheel_bearing", "Wheel Bearing", 4))
	add(hyundaiPages("hub_assembly", "Hub Assembly", 4))
	add(hyundaiPages("tie_rod", "Tie Rod", 5))
	add(hyundaiPages("ball_joint", "Ball Joint", 4))
	add(hyundaiPages("power_steering_pump", "Power Steering Pump", 3))
	add(hyundaiPages("steering_rack", "Steering Rack", 3))
	add(hyundaiPages("sway_bar_link", "Sway Bar Link", 3))
	add(hyundaiPages("cv_joint", "CV Joint", 3))
	add(hyundaiPages("brake_master_cylinder", "Brake Master Cylinder", 3))
	add(hyundaiPages("brake_hose", "Brake Hose", 3))
	add(hyundaiPages("wheel_cylinder", "Wheel Cylinder", 2))

	// HYUNDAI — Electrical & Lighting
	add(hyundaiPages("headlight", "Headlight", 6))
	add(hyundaiPages("tail_light", "Tail Light", 6))
	add(hyundaiPages("fog_light", "Fog Light", 4))
	add(hyundaiPages("wiper_blade", "Wiper Blade", 4))
	add(hyundaiPages("cabin_air_filter", "Cabin Air Filter", 5))
	add(hyundaiPages("alternator", "Alternator", 5))
	add(hyundaiPages("window_regulator", "Window Regulator", 5))
	add(hyundaiPages("power_window_switch", "Power Window Switch", 3))
	add(hyundaiPages("relay", "Relay", 3))
	add(hyundaiPages("antenna", "Antenna", 2))
	add(hyundaiPages("horn", "Horn", 2))
	add(hyundaiPages("turn_signal_light", "Turn Signal", 3))
	add(hyundaiPages("side_marker_light", "Side Marker", 2))
	add(hyundaiPages("battery", "Battery", 2))
	add(hyundaiPages("fuse_box", "Fuse Box", 2))

	// HYUNDAI — Body
	add(hyundaiPages("fender", "Fender", 5))
	add(hyundaiPages("bumper", "Bumper", 6))
	add(hyundaiPages("hood", "Hood", 4))
	add(hyundaiPages("grille", "Grille", 4))
	add(hyundaiPages("car_mirror", "Mirror", 5))
	add(hyundaiPages("door_handle", "Door Handle", 5))
	add(hyundaiPages("door_lock_cylinder", "Door Lock", 3))
	add(hyundaiPages("door_hinge", "Door Hinge", 3))
	add(hyundaiPages("hood_hinge", "Hood Hinge", 2))
	add(hyundaiPages("radiator_support", "Radiator Support", 3))
	add(hyundaiPages("trunk_lid", "Trunk Lid", 3))
	add(hyundaiPages("quarter_panel", "Quarter Panel", 3))
	add(hyundaiPages("rocker_panel", "Rocker Panel", 2))
	add(hyundaiPages("windshield", "Windshield", 3))
	add(hyundaiPages("weatherstripping", "Weatherstrip", 3))

	// HYUNDAI — Transmission & Drivetrain
	add(hyundaiPages("axle_shaft", "Axle Shaft", 5))
	add(hyundaiPages("shift_cable", "Shift Cable", 3))
	add(hyundaiPages("torque_converter", "Torque Converter", 3))
	add(hyundaiPages("clutch_fork", "Clutch", 3))
	add(hyundaiPages("clutch_disc", "Clutch Disc", 3))
	add(hyundaiPages("flywheel", "Flywheel", 2))
	add(hyundaiPages("differential", "Differential", 2))
	add(hyundaiPages("transfer_case", "Transfer Case", 2))
	add(hyundaiPages("transmission_mount", "Trans Mount", 3))

	// HYUNDAI — Cooling & HVAC
	add(hyundaiPages("radiator", "Radiator", 5))
	add(hyundaiPages("ac_compressor", "AC Compressor", 5))
	add(hyundaiPages("condenser", "Condenser", 4))
	add(hyundaiPages("blower_motor", "Blower Motor", 3))
	add(hyundaiPages("heater_core", "Heater Core", 3))
	add(hyundaiPages("radiator_hose", "Radiator Hose", 4))
	add(hyundaiPages("expansion_valve", "Expansion Valve", 2))
	add(hyundaiPages("fan_clutch", "Fan Clutch", 2))
	add(hyundaiPages("coolant_reservoir", "Coolant Reservoir", 2))

	// ═══════════════════════════════════════════════════
	// KIA — Engine
	add(kiaPages("oil_filter", "Oil Filter", 8))
	add(kiaPages("air_filter", "Air Filter", 8))
	add(kiaPages("spark_plug", "Spark Plug", 6))
	add(kiaPages("water_pump", "Water Pump", 6))
	add(kiaPages("ignition_coil", "Ignition Coil", 4))
	add(kiaPages("fuel_pump", "Fuel Pump", 5))
	add(kiaPages("fuel_injector", "Fuel Injector", 5))
	add(kiaPages("oxygen_sensor", "Oxygen Sensor", 5))
	add(kiaPages("catalytic_converter", "Catalytic Converter", 4))
	add(kiaPages("starter", "Starter Motor", 4))
	add(kiaPages("thermostat", "Thermostat", 4))
	add(kiaPages("timing_belt", "Timing Belt", 3))
	add(kiaPages("timing_chain", "Timing Chain", 2))
	add(kiaPages("engine_control_module", "Engine Control Module", 3))
	add(kiaPages("camshaft", "Camshaft", 2))
	add(kiaPages("oil_pan", "Oil Pan", 2))
	add(kiaPages("intake_manifold", "Intake Manifold", 3))
	add(kiaPages("exhaust_manifold", "Exhaust Manifold", 2))
	add(kiaPages("serpentine_belt", "Serpentine Belt", 2))
	add(kiaPages("engine_mount", "Engine Mount", 3))
	add(kiaPages("turbocharger", "Turbocharger", 2))

	// KIA — Chassis & Brakes
	add(kiaPages("brake_pad_set", "Brake Pad Set", 8))
	add(kiaPages("brake_disc", "Brake Disc", 6))
	add(kiaPages("shock_absorber", "Shock Absorber", 6))
	add(kiaPages("coil_springs", "Coil Springs", 4))
	add(kiaPages("control_arm", "Control Arm", 5))
	add(kiaPages("brake_caliper", "Brake Caliper", 4))
	add(kiaPages("wheel_bearing", "Wheel Bearing", 4))
	add(kiaPages("hub_assembly", "Hub Assembly", 3))
	add(kiaPages("tie_rod", "Tie Rod", 4))
	add(kiaPages("ball_joint", "Ball Joint", 4))
	add(kiaPages("power_steering_pump", "Power Steering Pump", 3))
	add(kiaPages("steering_rack", "Steering Rack", 2))
	add(kiaPages("sway_bar_link", "Sway Bar Link", 2))
	add(kiaPages("cv_joint", "CV Joint", 2))
	add(kiaPages("brake_master_cylinder", "Brake Master Cylinder", 2))

	// KIA — Electrical & Lighting
	add(kiaPages("headlight", "Headlight", 6))
	add(kiaPages("tail_light", "Tail Light", 6))
	add(kiaPages("fog_light", "Fog Light", 3))
	add(kiaPages("wiper_blade", "Wiper Blade", 3))
	add(kiaPages("cabin_air_filter", "Cabin Air Filter", 4))
	add(kiaPages("alternator", "Alternator", 4))
	add(kiaPages("window_regulator", "Window Regulator", 4))
	add(kiaPages("power_window_switch", "Power Window Switch", 2))
	add(kiaPages("antenna", "Antenna", 2))
	add(kiaPages("horn", "Horn", 2))
	add(kiaPages("turn_signal_light", "Turn Signal", 2))
	add(kiaPages("instrument_cluster", "Instrument Cluster", 2))
	add(kiaPages("speed_sensor", "Speed Sensor", 3))

	// KIA — Body
	add(kiaPages("fender", "Fender", 4))
	add(kiaPages("bumper", "Bumper", 6))
	add(kiaPages("hood", "Hood", 3))
	add(kiaPages("grille", "Grille", 3))
	add(kiaPages("door_handle", "Door Handle", 4))
	add(kiaPages("door_hinge", "Door Hinge", 2))
	add(kiaPages("door_lock_actuator", "Door Lock", 3))
	add(kiaPages("car_mirror", "Mirror", 4))
	add(kiaPages("trunk_lid", "Trunk Lid", 2))
	add(kiaPages("quarter_panel", "Quarter Panel", 2))
	add(kiaPages("windshield", "Windshield", 2))

	// KIA — Transmission & Drivetrain
	add(kiaPages("axle_shaft", "Axle Shaft", 4))
	add(kiaPages("drive_shaft", "Drive Shaft", 3))
	add(kiaPages("shift_cable", "Shift Cable", 2))
	add(kiaPages("clutch_disc", "Clutch", 3))
	add(kiaPages("flywheel", "Flywheel", 2))
	add(kiaPages("transfer_case", "Transfer Case", 2))

	// KIA — Cooling & HVAC
	add(kiaPages("radiator", "Radiator", 4))
	add(kiaPages("condenser", "Condenser", 3))
	add(kiaPages("ac_compressor", "AC Compressor", 3))
	add(kiaPages("blower_motor", "Blower Motor", 2))
	add(kiaPages("heater_core", "Heater Core", 2))
	add(kiaPages("radiator_hose", "Radiator Hose", 3))

	return pages
}

var rePartURL = regexp.MustCompile(`(?i)~([A-Z0-9][A-Z0-9\-]{4,})\.html`)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║               PARTS ENGINE — 5000 PART DETAILED REPORT                      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	if !waitForServer(3) {
		fmt.Println("FATAL: Server not reachable at", baseURL)
		os.Exit(1)
	}

	// Phase 1: Collect parts from local DB
	fmt.Println("Phase 1: Loading parts from local database...")
	localParts := loadLocalParts()
	fmt.Printf("  Local DB parts: %d\n\n", len(localParts))

	// Phase 2: Scrape additional parts from dealer sites
	categoryPages := buildCategoryPages()
	fmt.Printf("Phase 2: Scraping real OEM parts from %d category pages...\n", len(categoryPages))
	scrapedParts := scrapeExternalParts(localParts, categoryPages)
	fmt.Printf("  Scraped parts: %d\n\n", len(scrapedParts))

	// Phase 3: Combine, deduplicate, and sample
	allParts := append(localParts, scrapedParts...)
	if len(allParts) > targetSize {
		rand.Shuffle(len(scrapedParts), func(i, j int) {
			scrapedParts[i], scrapedParts[j] = scrapedParts[j], scrapedParts[i]
		})
		needed := targetSize - len(localParts)
		if needed > len(scrapedParts) {
			needed = len(scrapedParts)
		}
		allParts = append(localParts, scrapedParts[:needed]...)
	}
	fmt.Printf("Phase 3: Testing %d parts total (%d local + %d scraped)\n\n", len(allParts), len(localParts), len(allParts)-len(localParts))

	// Phase 4: Test each part and collect stats
	stats := &Stats{strategies: make(map[string]int), brands: make(map[string]int), categories: make(map[string]int)}
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("  %-6s %-16s %-8s %-5s %-25s %-15s %s\n", "#", "PART NUMBER", "SOURCE", "FOUND", "DESCRIPTION", "BRAND", "AFTERMARKET")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	for i, part := range allParts {
		testPart(i+1, part, stats)
	}

	// Phase 5: Summary
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                              SUMMARY                                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("  Total parts tested:     %d\n", stats.total)
	fmt.Printf("  Found:                  %d (%.1f%%)\n", stats.found, pct(stats.found, stats.total))
	fmt.Printf("  Not found:              %d (%.1f%%)\n", stats.notFound, pct(stats.notFound, stats.total))
	fmt.Printf("  Errors:                 %d\n", stats.errors)
	fmt.Println()
	fmt.Printf("  With aftermarket:       %d (%.1f%% of found)\n", stats.withAftermarket, pct(stats.withAftermarket, stats.found))
	fmt.Printf("  With substitutions:     %d (%.1f%% of found)\n", stats.withSubstitutions, pct(stats.withSubstitutions, stats.found))
	fmt.Printf("  With compatibility:     %d (%.1f%% of found)\n", stats.withCompatibility, pct(stats.withCompatibility, stats.found))
	fmt.Println()
	fmt.Println("  Strategy breakdown:")
	for strat, count := range stats.strategies {
		fmt.Printf("    %-35s %d (%.1f%%)\n", strat, count, pct(count, stats.found))
	}
	fmt.Println()
	fmt.Println("  Top brands:")
	for brand, count := range stats.brands {
		if count >= 2 {
			fmt.Printf("    %-30s %d\n", brand, count)
		}
	}
	fmt.Println()
	fmt.Printf("  Total aftermarket parts:  %d across %d OEM parts\n", stats.totalAftermarket, stats.withAftermarket)
	fmt.Printf("  Total result rows:        %d for %d queries\n", stats.totalResults, stats.total)
	fmt.Println()

	// Source breakdown
	fmt.Println("  Source breakdown:")
	fmt.Printf("    Local DB:     %d tested, %d found (%.1f%%)\n", stats.localTotal, stats.localFound, pct(stats.localFound, stats.localTotal))
	fmt.Printf("    Hyundai:      %d tested, %d found (%.1f%%)\n", stats.hyundaiTotal, stats.hyundaiFound, pct(stats.hyundaiFound, stats.hyundaiTotal))
	fmt.Printf("    KIA:          %d tested, %d found (%.1f%%)\n", stats.kiaTotal, stats.kiaFound, pct(stats.kiaFound, stats.kiaTotal))

	// Category hit rates
	fmt.Println()
	fmt.Println("  Category hit rates (top 20):")
	type catStat struct {
		cat   string
		found int
		total int
	}
	var catStats []catStat
	for cat, total := range stats.categories {
		found := stats.catFound[cat]
		catStats = append(catStats, catStat{cat, found, total})
	}
	// Sort by total descending
	for i := 0; i < len(catStats); i++ {
		for j := i + 1; j < len(catStats); j++ {
			if catStats[j].total > catStats[i].total {
				catStats[i], catStats[j] = catStats[j], catStats[i]
			}
		}
	}
	shown := 0
	for _, cs := range catStats {
		if shown >= 20 {
			break
		}
		fmt.Printf("    %-25s %d/%d (%.0f%%)\n", cs.cat, cs.found, cs.total, pct(cs.found, cs.total))
		shown++
	}

	score := int(pct(stats.found, stats.total))
	fmt.Printf("\n  SCORE: %d / 100\n", score)
}

type Stats struct {
	total             int
	found             int
	notFound          int
	errors            int
	withAftermarket   int
	withSubstitutions int
	withCompatibility int
	totalAftermarket  int
	totalResults      int
	localTotal        int
	localFound        int
	hyundaiTotal      int
	hyundaiFound      int
	kiaTotal          int
	kiaFound          int
	strategies        map[string]int
	brands            map[string]int
	categories        map[string]int
	catFound          map[string]int
}

func testPart(idx int, part PartInfo, stats *Stats) {
	if stats.catFound == nil {
		stats.catFound = make(map[string]int)
	}
	stats.total++
	stats.categories[part.Category]++

	switch part.Source {
	case "local":
		stats.localTotal++
	case "hyundai":
		stats.hyundaiTotal++
	case "kia":
		stats.kiaTotal++
	}

	resp, err := http.Get(baseURL + "/api/search?q=" + url.QueryEscape(part.PartNumber) + "&limit=20")
	if err != nil {
		fmt.Printf("  %-6d %-16s %-8s ERROR  %v\n", idx, part.PartNumber, part.Source, err)
		stats.errors++
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var r SmartSearchResponse
	json.Unmarshal(body, &r)

	stats.totalResults += r.Total

	if r.Total == 0 {
		fmt.Printf("  %-6d %-16s %-8s NO    -                         -               -\n", idx, part.PartNumber, part.Source)
		stats.notFound++
		return
	}

	stats.found++
	stats.catFound[part.Category]++
	stats.strategies[r.SearchStrategy]++

	switch part.Source {
	case "local":
		stats.localFound++
	case "hyundai":
		stats.hyundaiFound++
	case "kia":
		stats.kiaFound++
	}

	res := r.Results[0]
	stats.brands[res.BrandName]++

	// Count aftermarket
	afterCount := 0
	afterBrands := []string{}
	for _, result := range r.Results {
		afterCount += len(result.AftermarketAlternatives)
		for _, alt := range result.AftermarketAlternatives {
			afterBrands = append(afterBrands, alt.Brand)
		}
		brand := strings.ToUpper(result.BrandName)
		if brand != "" && brand != "HYUNDAI/KIA" && brand != "HYUNDAI / KIA" {
			afterCount++
			afterBrands = append(afterBrands, result.BrandName)
		}
	}

	if afterCount > 0 {
		stats.withAftermarket++
		stats.totalAftermarket += afterCount
	}

	hasSubs := false
	hasCompat := false
	for _, result := range r.Results {
		if len(result.Substitutions) > 0 {
			hasSubs = true
		}
		if len(result.Compatibility) > 0 {
			hasCompat = true
		}
	}
	if hasSubs {
		stats.withSubstitutions++
	}
	if hasCompat {
		stats.withCompatibility++
	}

	desc := res.Description
	if len(desc) > 23 {
		desc = desc[:20] + "..."
	}

	afterStr := "-"
	if afterCount > 0 {
		if len(afterBrands) <= 3 {
			afterStr = strings.Join(afterBrands, ",")
		} else {
			afterStr = fmt.Sprintf("%d brands", len(afterBrands))
		}
	}

	fmt.Printf("  %-6d %-16s %-8s YES   %-25s %-15s %s\n", idx, part.PartNumber, part.Source, desc, res.BrandName, afterStr)
}

func loadLocalParts() []PartInfo {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Printf("  WARN: Cannot open DB: %v\n", err)
		return nil
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT DISTINCT raw_number, description, brand_name 
		FROM oem_search_index 
		WHERE brand_name IN ('HYUNDAI/KIA', 'Hyundai / KIA')
		ORDER BY raw_number`)
	if err != nil {
		fmt.Printf("  WARN: Query failed: %v\n", err)
		return nil
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var parts []PartInfo
	for rows.Next() {
		var raw, desc, brand string
		rows.Scan(&raw, &desc, &brand)
		key := strings.ToUpper(strings.ReplaceAll(raw, "-", ""))
		if seen[key] {
			continue
		}
		seen[key] = true
		parts = append(parts, PartInfo{PartNumber: raw, Source: "local", Category: desc})
	}

	rows2, err := db.Query(`
		SELECT DISTINCT articleNumber, genericArticleDesc, brandName
		FROM hk_parts_cache
		WHERE brandName NOT IN ('HYUNDAI/KIA', 'Hyundai / KIA')
		ORDER BY articleNumber`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var artNum, desc, brand string
			rows2.Scan(&artNum, &desc, &brand)
			key := strings.ToUpper(strings.ReplaceAll(artNum, " ", ""))
			if seen[key] {
				continue
			}
			seen[key] = true
			parts = append(parts, PartInfo{PartNumber: artNum, Source: "local", Category: desc + " (" + brand + ")"})
		}
	}

	return parts
}

func scrapeExternalParts(existing []PartInfo, categoryPages []catPage) []PartInfo {
	seen := make(map[string]bool)
	for _, p := range existing {
		key := strings.ToUpper(strings.ReplaceAll(p.PartNumber, "-", ""))
		seen[key] = true
	}

	client := &http.Client{Timeout: 20 * time.Second}
	var all []PartInfo

	for i, page := range categoryPages {
		parts := scrapeOnePage(client, page.URL, page.Category, page.Source)
		for _, p := range parts {
			key := strings.ToUpper(strings.ReplaceAll(p.PartNumber, "-", ""))
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, p)
		}

		// Progress every 50 pages
		if (i+1)%50 == 0 {
			fmt.Printf("  ... scraped %d pages, %d unique parts so far\n", i+1, len(all))
		}

		time.Sleep(350 * time.Millisecond)
	}

	return all
}

func scrapeOnePage(client *http.Client, pageURL, category, source string) []PartInfo {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil
	}
	html := string(body)

	matches := rePartURL.FindAllStringSubmatch(html, 200)
	seen := make(map[string]bool)
	var parts []PartInfo
	for _, m := range matches {
		pn := strings.ToUpper(m[1])
		if seen[pn] || len(pn) < 5 {
			continue
		}
		seen[pn] = true
		parts = append(parts, PartInfo{PartNumber: pn, Source: source, Category: category})
		if len(parts) >= 30 {
			break
		}
	}

	if len(parts) > 0 {
		fmt.Printf("    %-25s (%s p%s): %d parts\n", category, source, pageNum(pageURL), len(parts))
	}
	return parts
}

func pageNum(u string) string {
	if i := strings.Index(u, "?p="); i >= 0 {
		return u[i+3:]
	}
	return "1"
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}

func waitForServer(seconds int) bool {
	for i := 0; i < seconds; i++ {
		resp, err := http.Get(baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

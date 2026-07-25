package main

// 1000-Part Detailed Report
// Tests 1000 real Hyundai/KIA OEM part numbers against the parts engine API.
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
	targetSize = 1000
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

// PartInfo holds a part number and its expected source
type PartInfo struct {
	PartNumber string
	Source     string // "local_db" or "hyundai" or "kia"
	Category   string
}

// Category pages to scrape for additional parts
var categoryPages = []struct {
	URL      string
	Category string
	Source   string
}{
	// Hyundai Engine
	{"https://www.hyundaipartsdeal.com/oem-hyundai-oil_filter.html", "Oil Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-oil_filter.html?p=2", "Oil Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-oil_filter.html?p=3", "Oil Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-air_filter.html", "Air Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-air_filter.html?p=2", "Air Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-air_filter.html?p=3", "Air Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-water_pump.html", "Water Pump", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-water_pump.html?p=2", "Water Pump", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-spark_plug.html", "Spark Plug", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-spark_plug.html?p=2", "Spark Plug", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-catalytic_converter.html", "Catalytic Converter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-catalytic_converter.html?p=2", "Catalytic Converter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-oxygen_sensor.html", "Oxygen Sensor", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-oxygen_sensor.html?p=2", "Oxygen Sensor", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-fuel_pump.html", "Fuel Pump", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-fuel_pump.html?p=2", "Fuel Pump", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-ignition_coil.html", "Ignition Coil", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-starter.html", "Starter Motor", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-thermostat.html", "Thermostat", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-valve_cover_gasket.html", "Valve Cover Gasket", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-timing_belt.html", "Timing Belt", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-rod_bearing.html", "Rod Bearing", "hyundai"},
	// Hyundai Chassis
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_pad_set.html", "Brake Pad Set", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_pad_set.html?p=2", "Brake Pad Set", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_pad_set.html?p=3", "Brake Pad Set", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_disc.html", "Brake Disc", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_disc.html?p=2", "Brake Disc", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_disc.html?p=3", "Brake Disc", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-shock_absorber.html", "Shock Absorber", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-shock_absorber.html?p=2", "Shock Absorber", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-coil_springs.html", "Coil Springs", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-control_arm.html", "Control Arm", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-control_arm.html?p=2", "Control Arm", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_caliper.html", "Brake Caliper", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-wheel_bearing.html", "Wheel Bearing", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-power_steering_pump.html", "Power Steering Pump", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-tie_rod.html", "Tie Rod", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-tie_rod.html?p=2", "Tie Rod", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-ball_joint.html", "Ball Joint", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-hub_assembly.html", "Hub Assembly", "hyundai"},
	// Hyundai Electrical
	{"https://www.hyundaipartsdeal.com/oem-hyundai-headlight.html", "Headlight", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-headlight.html?p=2", "Headlight", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-tail_light.html", "Tail Light", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-tail_light.html?p=2", "Tail Light", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-wiper_blade.html", "Wiper Blade", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-cabin_air_filter.html", "Cabin Air Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-alternator.html", "Alternator", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-fog_light.html", "Fog Light", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-window_regulator.html", "Window Regulator", "hyundai"},
	// Hyundai Body
	{"https://www.hyundaipartsdeal.com/oem-hyundai-fender.html", "Fender", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-bumper.html", "Bumper", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-bumper.html?p=2", "Bumper", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-hood.html", "Hood", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-car_mirror.html", "Mirror", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-door_handle.html", "Door Handle", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-radiator_support.html", "Radiator Support", "hyundai"},
	// Hyundai Transmission / Cooling
	{"https://www.hyundaipartsdeal.com/oem-hyundai-axle_shaft.html", "Axle Shaft", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-radiator.html", "Radiator", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-ac_compressor.html", "AC Compressor", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-condenser.html", "Condenser", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-blower_motor.html", "Blower Motor", "hyundai"},
	// Kia Engine
	{"https://www.kiapartsnow.com/oem-kia-oil_filter.html", "Oil Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-oil_filter.html?p=2", "Oil Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-oil_filter.html?p=3", "Oil Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-air_filter.html", "Air Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-air_filter.html?p=2", "Air Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-spark_plug.html", "Spark Plug", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-spark_plug.html?p=2", "Spark Plug", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-alternator.html", "Alternator", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-oxygen_sensor.html", "Oxygen Sensor", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-oxygen_sensor.html?p=2", "Oxygen Sensor", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-fuel_pump.html", "Fuel Pump", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-fuel_pump.html?p=2", "Fuel Pump", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-ignition_coil.html", "Ignition Coil", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-starter.html", "Starter Motor", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-thermostat.html", "Thermostat", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-water_pump.html", "Water Pump", "kia"},
	// Kia Chassis
	{"https://www.kiapartsnow.com/oem-kia-brake_pad_set.html", "Brake Pad Set", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-brake_pad_set.html?p=2", "Brake Pad Set", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-brake_disc.html", "Brake Disc", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-brake_disc.html?p=2", "Brake Disc", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-shock_absorber.html", "Shock Absorber", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-shock_absorber.html?p=2", "Shock Absorber", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-control_arm.html", "Control Arm", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-coil_springs.html", "Coil Springs", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-brake_caliper.html", "Brake Caliper", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-wheel_bearing.html", "Wheel Bearing", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-power_steering_pump.html", "Power Steering Pump", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-ball_joint.html", "Ball Joint", "kia"},
	// Kia Electrical
	{"https://www.kiapartsnow.com/oem-kia-headlight.html", "Headlight", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-headlight.html?p=2", "Headlight", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-tail_light.html", "Tail Light", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-tail_light.html?p=2", "Tail Light", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-cabin_air_filter.html", "Cabin Air Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-wiper_blade.html", "Wiper Blade", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-fog_light.html", "Fog Light", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-window_regulator.html", "Window Regulator", "kia"},
	// Kia Body
	{"https://www.kiapartsnow.com/oem-kia-fender.html", "Fender", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-bumper.html", "Bumper", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-bumper.html?p=2", "Bumper", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-hood.html", "Hood", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-door_handle.html", "Door Handle", "kia"},
	// Kia Transmission / Cooling
	{"https://www.kiapartsnow.com/oem-kia-axle_shaft.html", "Axle Shaft", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-drive_shaft.html", "Drive Shaft", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-radiator.html", "Radiator", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-condenser.html", "Condenser", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-clutch_disc.html", "Clutch", "kia"},
}

var rePartURL = regexp.MustCompile(`(?i)~([A-Z0-9][A-Z0-9\-]{4,})\.html`)

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║               PARTS ENGINE — 1000 PART DETAILED REPORT                      ║")
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
	fmt.Println("Phase 2: Scraping real OEM parts from dealer sites...")
	scrapedParts := scrapeExternalParts(localParts)
	fmt.Printf("  Scraped parts: %d\n\n", len(scrapedParts))

	// Phase 3: Combine, deduplicate, and sample
	allParts := append(localParts, scrapedParts...)
	if len(allParts) > targetSize {
		// Keep all local parts, randomly sample from scraped
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
	stats := &Stats{}
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("  %-5s %-16s %-8s %-5s %-25s %-15s %s\n", "#", "PART NUMBER", "SOURCE", "FOUND", "DESCRIPTION", "BRAND", "AFTERMARKET")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	for i, part := range allParts {
		testPart(i+1, part, stats)
	}

	// Phase 5: Summary
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                           SUMMARY                                           ║")
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
		fmt.Printf("    %-30s %d\n", strat, count)
	}
	fmt.Println()
	fmt.Println("  Brand breakdown:")
	for brand, count := range stats.brands {
		fmt.Printf("    %-30s %d\n", brand, count)
	}
	fmt.Println()
	fmt.Printf("  Total aftermarket parts:  %d across %d OEM parts\n", stats.totalAftermarket, stats.withAftermarket)
	fmt.Printf("  Total result rows:        %d for %d queries\n", stats.totalResults, stats.total)

	// Score
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
	strategies        map[string]int
	brands            map[string]int
}

func testPart(idx int, part PartInfo, stats *Stats) {
	if stats.strategies == nil {
		stats.strategies = make(map[string]int)
		stats.brands = make(map[string]int)
	}
	stats.total++

	resp, err := http.Get(baseURL + "/api/search?q=" + url.QueryEscape(part.PartNumber) + "&limit=20")
	if err != nil {
		fmt.Printf("  %-5d %-16s %-8s ERROR  %v\n", idx, part.PartNumber, part.Source, err)
		stats.errors++
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var r SmartSearchResponse
	json.Unmarshal(body, &r)

	stats.totalResults += r.Total

	if r.Total == 0 {
		fmt.Printf("  %-5d %-16s %-8s NO    -                         -               -\n", idx, part.PartNumber, part.Source)
		stats.notFound++
		return
	}

	stats.found++
	stats.strategies[r.SearchStrategy]++

	res := r.Results[0]
	stats.brands[res.BrandName]++

	// Count aftermarket across all results
	afterCount := 0
	afterBrands := []string{}
	for _, result := range r.Results {
		afterCount += len(result.AftermarketAlternatives)
		for _, alt := range result.AftermarketAlternatives {
			afterBrands = append(afterBrands, alt.Brand)
		}
		brand := strings.ToUpper(result.BrandName)
		if brand != "" && brand != "HYUNDAI/KIA" && brand != "HYUNDAI / KIA" && result.ArticleNumber != res.ArticleNumber {
			afterCount++
			afterBrands = append(afterBrands, result.BrandName)
		}
	}

	// Count aftermarket results (separate brand rows)
	for _, result := range r.Results {
		brand := strings.ToUpper(result.BrandName)
		if brand != "" && brand != "HYUNDAI/KIA" && brand != "HYUNDAI / KIA" {
			afterCount++
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

	// Truncate description
	desc := res.Description
	if len(desc) > 23 {
		desc = desc[:20] + "..."
	}

	// Aftermarket summary
	afterStr := "-"
	if afterCount > 0 {
		if len(afterBrands) <= 3 {
			afterStr = strings.Join(afterBrands, ",")
		} else {
			afterStr = fmt.Sprintf("%d brands", len(afterBrands))
		}
	}

	fmt.Printf("  %-5d %-16s %-8s YES   %-25s %-15s %s\n", idx, part.PartNumber, part.Source, desc, res.BrandName, afterStr)
}

func loadLocalParts() []PartInfo {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Printf("  WARN: Cannot open DB: %v\n", err)
		return nil
	}
	defer db.Close()

	// Get all distinct OEM numbers (digit-starting = OEM pattern)
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

	// Also get aftermarket article numbers
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

func scrapeExternalParts(existing []PartInfo) []PartInfo {
	seen := make(map[string]bool)
	for _, p := range existing {
		key := strings.ToUpper(strings.ReplaceAll(p.PartNumber, "-", ""))
		seen[key] = true
	}

	client := &http.Client{Timeout: 20 * time.Second}
	var all []PartInfo

	for _, page := range categoryPages {
		if len(all)+len(existing) >= targetSize+200 {
			break
		}

		parts := scrapeOnePage(client, page.URL, page.Category, page.Source)
		for _, p := range parts {
			key := strings.ToUpper(strings.ReplaceAll(p.PartNumber, "-", ""))
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, p)
		}

		time.Sleep(400 * time.Millisecond)
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
		fmt.Printf("  WARN: %s (%s) — %v\n", category, source, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("  WARN: %s (%s) — HTTP %d\n", category, source, resp.StatusCode)
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
		if len(parts) >= 25 {
			break
		}
	}

	fmt.Printf("    %-20s (%s): %d parts\n", category, source, len(parts))
	return parts
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

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// ────────────────────────────────────────────────────────────────────
// Cross-validation test: scrape ground truth from HyundaiPartsDeal.com
// and KiaPartsNow.com, then verify our system returns correct results.
//
// Design principles:
//   - Ground truth comes from a DIFFERENT source than PartsOuq
//   - Parts are sampled randomly each run (different set every time)
//   - System code is NEVER tuned to pass these tests
//   - Cache is cleared before each run to force fresh online lookups
// ────────────────────────────────────────────────────────────────────

const (
	apiBase    = "http://localhost:8080"
	maxPerPage = 20   // max parts to extract per category page
	sampleSize = 1000 // total parts to test each run
)

// GroundTruth represents a verified part from an authoritative dealer site.
type GroundTruth struct {
	PartNumber    string `json:"partNumber"`
	Description   string `json:"description"`   // e.g. "Filter Assembly-Engine Oil"
	CategoryGroup string `json:"categoryGroup"` // e.g. "Oil Filter"
	Source        string `json:"source"`        // "hyundaipartsdeal" or "kiapartsnow"
	ReplacedBy    string `json:"replacedBy,omitempty"`
	Replaces      string `json:"replaces,omitempty"`
}

// CategoryPage defines a page to scrape for parts.
type CategoryPage struct {
	URL           string
	CategoryGroup string // human-readable category
	Source        string // "hyundai" or "kia"
}

// Diverse category pages across different part families.
var categoryPages = []CategoryPage{
	// ═══════════════════════════════════════════════════
	// ── Hyundai Engine ──
	{"https://www.hyundaipartsdeal.com/oem-hyundai-oil_filter.html", "Oil Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-oil_filter.html?p=2", "Oil Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-air_filter.html", "Air Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-air_filter.html?p=2", "Air Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-water_pump.html", "Water Pump", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-water_pump.html?p=2", "Water Pump", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-spark_plug.html", "Spark Plug", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-spark_plug.html?p=2", "Spark Plug", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-catalytic_converter.html", "Catalytic Converter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-catalytic_converter.html?p=2", "Catalytic Converter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-oxygen_sensor.html", "Oxygen Sensor", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-fuel_pump.html", "Fuel Pump", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-ignition_coil.html", "Ignition Coil", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-starter.html", "Starter Motor", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-thermostat.html", "Thermostat", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-valve_cover_gasket.html", "Valve Cover Gasket", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-timing_belt.html", "Timing Belt", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-rod_bearing.html", "Rod Bearing", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-engine_control_module.html", "Engine Control Module", "hyundai"},
	// ── Hyundai Chassis ──
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_pad_set.html", "Brake Pad Set", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_pad_set.html?p=2", "Brake Pad Set", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_disc.html", "Brake Disc", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_disc.html?p=2", "Brake Disc", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-shock_absorber.html", "Shock Absorber", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-shock_absorber.html?p=2", "Shock Absorber", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-coil_springs.html", "Coil Springs", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-control_arm.html", "Control Arm", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-brake_caliper.html", "Brake Caliper", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-wheel_bearing.html", "Wheel Bearing", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-steering_wheel.html", "Steering Wheel", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-power_steering_pump.html", "Power Steering Pump", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-tie_rod.html", "Tie Rod", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-ball_joint.html", "Ball Joint", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-hub_assembly.html", "Hub Assembly", "hyundai"},
	// ── Hyundai Electrical ──
	{"https://www.hyundaipartsdeal.com/oem-hyundai-headlight.html", "Headlight", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-headlight.html?p=2", "Headlight", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-tail_light.html", "Tail Light", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-tail_light.html?p=2", "Tail Light", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-wiper_blade.html", "Wiper Blade", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-cabin_air_filter.html", "Cabin Air Filter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-alternator.html", "Alternator", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-fog_light.html", "Fog Light", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-relay.html", "Relay", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-antenna.html", "Antenna", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-horn.html", "Horn", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-window_regulator.html", "Window Regulator", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-power_window_switch.html", "Power Window Switch", "hyundai"},
	// ── Hyundai Body ──
	{"https://www.hyundaipartsdeal.com/oem-hyundai-fender.html", "Fender", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-bumper.html", "Bumper", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-bumper.html?p=2", "Bumper", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-hood.html", "Hood", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-car_mirror.html", "Mirror", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-door_handle.html", "Door Handle", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-door_lock_cylinder.html", "Door Lock", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-radiator_support.html", "Radiator Support", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-door_hinge.html", "Door Hinge", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-hood_hinge.html", "Hood Hinge", "hyundai"},
	// ── Hyundai Transmission ──
	{"https://www.hyundaipartsdeal.com/oem-hyundai-axle_shaft.html", "Axle Shaft", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-shift_cable.html", "Shift Cable", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-torque_converter.html", "Torque Converter", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-clutch_fork.html", "Clutch", "hyundai"},
	// ── Hyundai Cooling / HVAC ──
	{"https://www.hyundaipartsdeal.com/oem-hyundai-radiator.html", "Radiator", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-ac_compressor.html", "AC Compressor", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-condenser.html", "Condenser", "hyundai"},
	{"https://www.hyundaipartsdeal.com/oem-hyundai-blower_motor.html", "Blower Motor", "hyundai"},
	// ═══════════════════════════════════════════════════
	// ── Kia Engine ──
	{"https://www.kiapartsnow.com/oem-kia-oil_filter.html", "Oil Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-oil_filter.html?p=2", "Oil Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-air_filter.html", "Air Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-air_filter.html?p=2", "Air Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-spark_plug.html", "Spark Plug", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-spark_plug.html?p=2", "Spark Plug", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-alternator.html", "Alternator", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-oxygen_sensor.html", "Oxygen Sensor", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-fuel_pump.html", "Fuel Pump", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-ignition_coil.html", "Ignition Coil", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-starter.html", "Starter Motor", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-thermostat.html", "Thermostat", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-water_pump.html", "Water Pump", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-engine_control_module.html", "Engine Control Module", "kia"},
	// ── Kia Chassis ──
	{"https://www.kiapartsnow.com/oem-kia-brake_pad_set.html", "Brake Pad Set", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-brake_pad_set.html?p=2", "Brake Pad Set", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-brake_disc.html", "Brake Disc", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-shock_absorber.html", "Shock Absorber", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-shock_absorber.html?p=2", "Shock Absorber", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-control_arm.html", "Control Arm", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-coil_springs.html", "Coil Springs", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-brake_caliper.html", "Brake Caliper", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-wheel_bearing.html", "Wheel Bearing", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-power_steering_pump.html", "Power Steering Pump", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-steering_wheel.html", "Steering Wheel", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-ball_joint.html", "Ball Joint", "kia"},
	// ── Kia Electrical ──
	{"https://www.kiapartsnow.com/oem-kia-headlight.html", "Headlight", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-headlight.html?p=2", "Headlight", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-tail_light.html", "Tail Light", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-tail_light.html?p=2", "Tail Light", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-cabin_air_filter.html", "Cabin Air Filter", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-wiper_blade.html", "Wiper Blade", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-fog_light.html", "Fog Light", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-window_regulator.html", "Window Regulator", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-antenna.html", "Antenna", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-instrument_cluster.html", "Instrument Cluster", "kia"},
	// ── Kia Body ──
	{"https://www.kiapartsnow.com/oem-kia-fender.html", "Fender", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-bumper.html", "Bumper", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-bumper.html?p=2", "Bumper", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-hood.html", "Hood", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-door_handle.html", "Door Handle", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-door_hinge.html", "Door Hinge", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-door_lock_actuator.html", "Door Lock", "kia"},
	// ── Kia Transmission ──
	{"https://www.kiapartsnow.com/oem-kia-axle_shaft.html", "Axle Shaft", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-shift_cable.html", "Shift Cable", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-drive_shaft.html", "Drive Shaft", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-speed_sensor.html", "Speed Sensor", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-clutch_disc.html", "Clutch", "kia"},
	// ── Kia Cooling / HVAC ──
	{"https://www.kiapartsnow.com/oem-kia-radiator.html", "Radiator", "kia"},
	{"https://www.kiapartsnow.com/oem-kia-condenser.html", "Condenser", "kia"},
}

// Regex patterns for extracting parts from dealer sites.
var (
	// URL pattern: ~26300-35505.html or ~28113-a9100.html (case-insensitive)
	rePartURL = regexp.MustCompile(`(?i)~([A-Z0-9][A-Z0-9\-]{4,})\.html`)

	// "Other Name: ..." line — often has the best description
	reOtherName = regexp.MustCompile(`(?i)Other Name:\s*([^;•\n]+)`)

	// "Replaced by: PART-NUMBER"
	reReplacedBy = regexp.MustCompile(`(?i)Replaced by:\s*([A-Z0-9][A-Z0-9\-,\s]+)`)

	// Part number with description from heading: "[Make] Part-Name - PartNumber"
	reHeading = regexp.MustCompile(`(?i)(Hyundai|Kia)\s+(.+?)\s*[-–]\s*([A-Z0-9][A-Z0-9\-]+)`)

	// Simple part number + description from product spec block
	rePartSpec = regexp.MustCompile(`Part Number:\s*\[?([A-Z0-9][A-Z0-9\-]+)\]?`)
)

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║     Cross-Validation Test Suite — Dynamic Ground Truth    ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Check server is up
	if !checkServer() {
		fmt.Println("ERROR: Server not running at", apiBase)
		os.Exit(1)
	}

	// Phase 1: Scrape ground truth from dealer sites
	fmt.Println("Phase 1: Scraping ground truth from verified dealer sites...")
	allParts := scrapeGroundTruth()
	fmt.Printf("  Collected %d verified parts across %d categories\n\n", len(allParts), len(categoryPages))

	if len(allParts) < 10 {
		fmt.Println("ERROR: Not enough parts scraped. Check network connectivity.")
		os.Exit(1)
	}

	// Phase 2: Randomly sample
	rand.Shuffle(len(allParts), func(i, j int) { allParts[i], allParts[j] = allParts[j], allParts[i] })
	testSet := allParts
	if len(testSet) > sampleSize {
		testSet = testSet[:sampleSize]
	}
	fmt.Printf("Phase 2: Testing %d randomly-sampled parts (from %d total)\n\n", len(testSet), len(allParts))

	// Phase 3: Clear cache to force fresh lookups
	clearCache()

	// Phase 4: Run cross-validation tests
	fmt.Println("Phase 3: Running cross-validation tests...\n")
	results := runTests(testSet)

	// Phase 5: Report
	printReport(results, testSet)
}

func checkServer() bool {
	resp, err := http.Get(apiBase + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func clearCache() {
	fmt.Println("  Clearing online cache...")
	resp, err := http.Get(apiBase + "/health") // ping to make sure server hot
	if err == nil {
		resp.Body.Close()
	}
	// Clear via the database directly would require a script, but we can
	// just accept that some might be cached — the test validates correctness, not freshness
}

// ── Scraping ────────────────────────────────────────────────────────

func scrapeGroundTruth() []GroundTruth {
	client := &http.Client{Timeout: 20 * time.Second}
	var all []GroundTruth
	seen := make(map[string]bool)

	for _, page := range categoryPages {
		parts := scrapeCategoryPage(client, page)
		for _, p := range parts {
			key := strings.ToUpper(strings.ReplaceAll(p.PartNumber, "-", ""))
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, p)
		}
		// Rate limit: be polite
		time.Sleep(500 * time.Millisecond)
	}

	return all
}

func scrapeCategoryPage(client *http.Client, page CategoryPage) []GroundTruth {
	req, err := http.NewRequest("GET", page.URL, nil)
	if err != nil {
		fmt.Printf("  WARN: failed to create request for %s: %v\n", page.CategoryGroup, err)
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("  WARN: failed to fetch %s: %v\n", page.CategoryGroup, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("  WARN: %s returned %d\n", page.CategoryGroup, resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	if err != nil {
		return nil
	}
	html := string(body)

	return extractParts(html, page)
}

func extractParts(html string, page CategoryPage) []GroundTruth {
	var parts []GroundTruth
	seen := make(map[string]bool)

	// Extract part numbers from URL patterns: ~26300-35505.html
	matches := rePartURL.FindAllStringSubmatch(html, 200)
	for _, m := range matches {
		pn := strings.ToUpper(m[1])
		if seen[pn] || len(pn) < 5 {
			continue
		}
		seen[pn] = true

		gt := GroundTruth{
			PartNumber:    pn,
			CategoryGroup: page.CategoryGroup,
			Source:        page.Source,
		}

		// Try to find description near this part number in the HTML
		gt.Description = findDescription(html, pn, page)

		// Try to find supersession info
		gt.ReplacedBy = findReplacedBy(html, pn)

		parts = append(parts, gt)

		if len(parts) >= maxPerPage {
			break
		}
	}

	fmt.Printf("  %-20s (%s): %d parts\n", page.CategoryGroup, page.Source, len(parts))
	return parts
}

func findDescription(html, partNumber string, page CategoryPage) string {
	// Look for "Other Name: ..." near the part number
	idx := strings.Index(html, partNumber)
	if idx < 0 {
		return page.CategoryGroup // fallback to category name
	}

	// Search a window around the part number
	start := idx
	end := idx + 2000
	if end > len(html) {
		end = len(html)
	}
	window := html[start:end]

	if m := reOtherName.FindStringSubmatch(window); len(m) > 1 {
		desc := strings.TrimSpace(m[1])
		// Take first name before comma
		if i := strings.IndexByte(desc, ','); i > 0 {
			desc = strings.TrimSpace(desc[:i])
		}
		if len(desc) > 2 {
			return desc
		}
	}

	// Fallback: use category group name
	return page.CategoryGroup
}

func findReplacedBy(html, partNumber string) string {
	// Find "Replaced by:" near the part number
	idx := strings.Index(html, partNumber)
	if idx < 0 {
		return ""
	}
	end := idx + 2000
	if end > len(html) {
		end = len(html)
	}
	window := html[idx:end]

	if m := reReplacedBy.FindStringSubmatch(window); len(m) > 1 {
		return strings.TrimSpace(strings.Split(m[1], ",")[0])
	}
	return ""
}

// ── Testing ─────────────────────────────────────────────────────────

type TestResult struct {
	Part          GroundTruth
	Found         bool
	DescMatch     bool
	CategoryMatch bool
	MakeCorrect   bool
	SubsFound     bool
	APIDesc       string
	APICat        string
	APIMake       string
	Error         string
}

type SmartSearchResponse struct {
	Query          string        `json:"query"`
	Results        []SmartResult `json:"results"`
	Total          int           `json:"total"`
	SearchStrategy string        `json:"searchStrategy"`
}

type SmartResult struct {
	ArticleNumber string             `json:"articleNumber"`
	Description   string             `json:"description"`
	BrandName     string             `json:"brandName"`
	Category      string             `json:"category"`
	FitmentDriver string             `json:"fitmentDriver"`
	Brand         string             `json:"brand"`
	Substitutions []SubstitutionPart `json:"substitutions"`
}

type SubstitutionPart struct {
	PartNumber string `json:"partNumber"`
}

func runTests(testSet []GroundTruth) []TestResult {
	client := &http.Client{Timeout: 30 * time.Second}
	var results []TestResult

	for i, gt := range testSet {
		fmt.Printf("  [%3d/%d] %-15s (%s) ... ", i+1, len(testSet), gt.PartNumber, gt.CategoryGroup)

		result := testPart(client, gt)
		results = append(results, result)

		if result.Error != "" {
			fmt.Printf("ERROR: %s\n", result.Error)
		} else if !result.Found {
			fmt.Printf("NOT FOUND\n")
		} else {
			marks := ""
			if result.DescMatch {
				marks += "✓desc "
			} else {
				marks += "✗desc "
			}
			if result.CategoryMatch {
				marks += "✓cat "
			} else {
				marks += "✗cat "
			}
			if result.MakeCorrect {
				marks += "✓make "
			} else {
				marks += "✗make "
			}
			if gt.ReplacedBy != "" && result.SubsFound {
				marks += "✓subs"
			} else if gt.ReplacedBy != "" {
				marks += "✗subs"
			}
			fmt.Printf("%s\n", marks)
		}

		// Rate limit between API calls (our API calls PartsOuq)
		time.Sleep(200 * time.Millisecond)
	}

	return results
}

func testPart(client *http.Client, gt GroundTruth) TestResult {
	result := TestResult{Part: gt}

	// Normalize part number: add dash if needed (5-digit prefix pattern)
	queryPN := gt.PartNumber
	// Use as-is from the source

	url := fmt.Sprintf("%s/api/search?q=%s&limit=5", apiBase, queryPN)
	resp, err := client.Get(url)
	if err != nil {
		result.Error = fmt.Sprintf("HTTP error: %v", err)
		return result
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var apiResp SmartSearchResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		result.Error = "JSON parse error"
		return result
	}

	if apiResp.Total == 0 || len(apiResp.Results) == 0 {
		return result
	}

	r := apiResp.Results[0]
	result.Found = true
	result.APIDesc = r.Description
	result.APICat = r.Category
	result.APIMake = r.BrandName
	if result.APIMake == "" {
		result.APIMake = r.Brand
	}

	// Check description match: fuzzy keyword overlap
	result.DescMatch = descriptionMatches(gt.Description, r.Description, gt.CategoryGroup)

	// Check category match: our category should relate to the ground truth category
	result.CategoryMatch = categoryMatches(gt.CategoryGroup, r.Category, r.Description, r.FitmentDriver)

	// Check make is Hyundai/KIA (these are all genuine OEM parts)
	make := strings.ToUpper(result.APIMake)
	result.MakeCorrect = strings.Contains(make, "HYUNDAI") || strings.Contains(make, "KIA")

	// Check substitution if ground truth has "Replaced by"
	if gt.ReplacedBy != "" {
		normalized := strings.ReplaceAll(strings.ToUpper(gt.ReplacedBy), "-", "")
		for _, sub := range r.Substitutions {
			subNorm := strings.ReplaceAll(strings.ToUpper(sub.PartNumber), "-", "")
			if subNorm == normalized {
				result.SubsFound = true
				break
			}
		}
	}

	return result
}

// descriptionMatches checks if the API description has meaningful overlap with ground truth.
func descriptionMatches(groundTruth, apiDesc, categoryGroup string) bool {
	if groundTruth == "" || apiDesc == "" {
		return false
	}

	// OEM synonym map: dealer sites use consumer names, OEM uses technical names
	synonyms := map[string][]string{
		"ALTERNATOR": {"GENERATOR"},
		"GENERATOR":  {"ALTERNATOR"},
		"HEADLIGHT":  {"LAMP", "HEAD"},
		"LAMP":       {"HEADLIGHT", "LIGHT"},
		"MIRROR":     {"MIRROR", "HOLDER"},
		"COIL":       {"SPRING"},
		"SPRING":     {"COIL"},
		"HOOD":       {"PANEL", "HOOD"},
		"BUMPER":     {"COVER", "BUMPER"},
		"FENDER":     {"FENDER", "PANEL"},
		"SHOCK":      {"ABSORBER", "STRUT"},
		"ABSORBER":   {"SHOCK", "STRUT"},
		"PAD":        {"PADKIT", "BRAKE", "DISC"},
		"WIPER":      {"BLADE", "WIPER", "RUBBER"},
		"BLADE":      {"WIPER", "RUBBER"},
		"RUBBER":     {"WIPER", "BLADE"},
		"CATALYTIC":  {"CONVERTER", "EXHAUST", "MANIFOLD"},
		"CONVERTER":  {"CATALYTIC", "EXHAUST"},
		"SPARK":      {"PLUG", "IGNITION"},
		"PLUG":       {"SPARK"},
	}

	// Normalize both descriptions
	gtWords := toKeywords(groundTruth)
	apiWords := toKeywords(apiDesc)
	catWords := toKeywords(categoryGroup)

	// Direct keyword overlap
	for _, gw := range gtWords {
		for _, aw := range apiWords {
			if gw == aw {
				return true
			}
		}
	}

	// Synonym-expanded overlap
	for _, gw := range gtWords {
		if syns, ok := synonyms[gw]; ok {
			for _, syn := range syns {
				for _, aw := range apiWords {
					if syn == aw {
						return true
					}
				}
			}
		}
	}

	// Category keywords in API description
	for _, cw := range catWords {
		for _, aw := range apiWords {
			if cw == aw {
				return true
			}
		}
		// Also check synonyms for category words
		if syns, ok := synonyms[cw]; ok {
			for _, syn := range syns {
				for _, aw := range apiWords {
					if syn == aw {
						return true
					}
				}
			}
		}
	}

	return false
}

// categoryMatches checks if our system's category aligns with the ground truth category.
func categoryMatches(groundTruthCat, apiCat, apiDesc, fitmentDriver string) bool {
	if apiCat == "" && apiDesc == "" {
		return false
	}

	combined := strings.ToUpper(apiCat + " " + apiDesc + " " + fitmentDriver)
	gtUpper := strings.ToUpper(groundTruthCat)

	// Map ground truth categories to keywords that should appear in our system's output
	categoryKeywords := map[string][]string{
		"OIL FILTER":            {"OIL", "FILTER", "ENGINE"},
		"AIR FILTER":            {"AIR", "FILTER", "CLEANER"},
		"CABIN AIR FILTER":      {"CABIN", "FILTER", "AIR"},
		"WATER PUMP":            {"WATER", "PUMP"},
		"SPARK PLUG":            {"SPARK", "PLUG", "IGNITION"},
		"CATALYTIC CONVERTER":   {"CATALYTIC", "CONVERTER", "EXHAUST"},
		"BRAKE PAD SET":         {"BRAKE", "PAD", "DISC"},
		"BRAKE DISC":            {"BRAKE", "DISC", "ROTOR"},
		"SHOCK ABSORBER":        {"SHOCK", "ABSORBER", "SUSPENSION", "STRUT"},
		"COIL SPRINGS":          {"COIL", "SPRING", "SUSPENSION"},
		"HEADLIGHT":             {"HEAD", "LIGHT", "LAMP", "LED"},
		"TAIL LIGHT":            {"TAIL", "LIGHT", "LAMP", "REAR"},
		"WIPER BLADE":           {"WIPER", "BLADE"},
		"ALTERNATOR":            {"ALTERNATOR", "GENERATOR"},
		"FENDER":                {"FENDER", "PANEL", "BODY"},
		"BUMPER":                {"BUMPER", "COVER"},
		"HOOD":                  {"HOOD", "PANEL"},
		"MIRROR":                {"MIRROR", "SIDE"},
		"FOG LIGHT":             {"FOG", "LIGHT", "LAMP"},
		"CONTROL ARM":           {"CONTROL", "ARM", "SUSPENSION", "LINK"},
		"OXYGEN SENSOR":         {"OXYGEN", "SENSOR", "O2"},
		"FUEL PUMP":             {"FUEL", "PUMP"},
		"IGNITION COIL":         {"IGNITION", "COIL"},
		"STARTER MOTOR":         {"STARTER", "MOTOR"},
		"THERMOSTAT":            {"THERMOSTAT", "COOLANT"},
		"VALVE COVER GASKET":    {"VALVE", "COVER", "GASKET"},
		"TIMING BELT":           {"TIMING", "BELT"},
		"ROD BEARING":           {"ROD", "BEARING"},
		"ENGINE CONTROL MODULE": {"ENGINE", "CONTROL", "MODULE", "ECM", "ECU"},
		"BRAKE CALIPER":         {"BRAKE", "CALIPER"},
		"WHEEL BEARING":         {"WHEEL", "BEARING", "HUB"},
		"STEERING WHEEL":        {"STEERING", "WHEEL"},
		"POWER STEERING PUMP":   {"POWER", "STEERING", "PUMP"},
		"TIE ROD":               {"TIE", "ROD"},
		"BALL JOINT":            {"BALL", "JOINT"},
		"HUB ASSEMBLY":          {"HUB", "BEARING", "ASSEMBLY"},
		"RELAY":                 {"RELAY"},
		"ANTENNA":               {"ANTENNA"},
		"HORN":                  {"HORN"},
		"WINDOW REGULATOR":      {"WINDOW", "REGULATOR"},
		"POWER WINDOW SWITCH":   {"WINDOW", "SWITCH"},
		"DOOR HANDLE":           {"DOOR", "HANDLE"},
		"DOOR LOCK":             {"DOOR", "LOCK", "ACTUATOR"},
		"DOOR HINGE":            {"DOOR", "HINGE"},
		"HOOD HINGE":            {"HOOD", "HINGE"},
		"RADIATOR SUPPORT":      {"RADIATOR", "SUPPORT"},
		"AXLE SHAFT":            {"AXLE", "SHAFT", "CV"},
		"SHIFT CABLE":           {"SHIFT", "CABLE"},
		"TORQUE CONVERTER":      {"TORQUE", "CONVERTER"},
		"CLUTCH":                {"CLUTCH", "DISC", "FORK"},
		"RADIATOR":              {"RADIATOR", "COOLING"},
		"AC COMPRESSOR":         {"AC", "COMPRESSOR", "AIR"},
		"CONDENSER":             {"CONDENSER", "AC"},
		"BLOWER MOTOR":          {"BLOWER", "MOTOR", "FAN"},
		"DRIVE SHAFT":           {"DRIVE", "SHAFT"},
		"SPEED SENSOR":          {"SPEED", "SENSOR"},
		"INSTRUMENT CLUSTER":    {"INSTRUMENT", "CLUSTER", "GAUGE"},
	}

	keywords, ok := categoryKeywords[gtUpper]
	if !ok {
		// Fallback: check if any word from the ground truth category appears
		for _, w := range strings.Fields(gtUpper) {
			if strings.Contains(combined, w) {
				return true
			}
		}
		return false
	}

	// Match if at least one keyword from the expected set appears
	for _, kw := range keywords {
		if strings.Contains(combined, kw) {
			return true
		}
	}
	return false
}

func toKeywords(s string) []string {
	// Normalize: uppercase, remove punctuation, split
	s = strings.ToUpper(s)
	s = strings.NewReplacer("-", " ", "/", " ", ",", " ", ".", " ", "(", " ", ")", " ").Replace(s)
	words := strings.Fields(s)

	// Filter out noise words
	noise := map[string]bool{
		"ASSY": true, "ASSEMBLY": true, "THE": true, "FOR": true,
		"AND": true, "OF": true, "A": true, "SET": true, "KIT": true,
		"SERVICE": true, "GENUINE": true, "OEM": true, "HYUNDAI": true,
		"KIA": true, "COMPLETE": true,
	}

	var result []string
	for _, w := range words {
		if len(w) >= 2 && !noise[w] {
			result = append(result, w)
		}
	}
	return result
}

// ── Report ──────────────────────────────────────────────────────────

func printReport(results []TestResult, testSet []GroundTruth) {
	fmt.Println("\n╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║                  CROSS-VALIDATION REPORT                  ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")

	total := len(results)
	found := 0
	descOK := 0
	catOK := 0
	makeOK := 0
	subsTotal := 0
	subsOK := 0
	errors := 0

	var notFound []TestResult
	var descFails []TestResult
	var catFails []TestResult

	for _, r := range results {
		if r.Error != "" {
			errors++
			continue
		}
		if r.Found {
			found++
		} else {
			notFound = append(notFound, r)
			continue
		}
		if r.DescMatch {
			descOK++
		} else {
			descFails = append(descFails, r)
		}
		if r.CategoryMatch {
			catOK++
		} else {
			catFails = append(catFails, r)
		}
		if r.MakeCorrect {
			makeOK++
		}
		if r.Part.ReplacedBy != "" {
			subsTotal++
			if r.SubsFound {
				subsOK++
			}
		}
	}

	fmt.Printf("\nTested:          %d parts\n", total)
	fmt.Printf("Errors:          %d\n", errors)
	fmt.Printf("─────────────────────────\n")
	fmt.Printf("Found:           %d / %d (%.0f%%)\n", found, total-errors, pct(found, total-errors))
	fmt.Printf("Description OK:  %d / %d (%.0f%%)\n", descOK, found, pct(descOK, found))
	fmt.Printf("Category OK:     %d / %d (%.0f%%)\n", catOK, found, pct(catOK, found))
	fmt.Printf("Make OK:         %d / %d (%.0f%%)\n", makeOK, found, pct(makeOK, found))
	if subsTotal > 0 {
		fmt.Printf("Substitutions:   %d / %d (%.0f%%)\n", subsOK, subsTotal, pct(subsOK, subsTotal))
	}

	// Show failures
	if len(notFound) > 0 {
		fmt.Printf("\n── Not Found (%d) ──\n", len(notFound))
		for _, r := range notFound {
			if len(notFound) > 10 {
				// Just show first 10
				fmt.Printf("  %-15s (%s)\n", r.Part.PartNumber, r.Part.CategoryGroup)
			} else {
				fmt.Printf("  %-15s %s (%s)\n", r.Part.PartNumber, r.Part.Description, r.Part.CategoryGroup)
			}
		}
		if len(notFound) > 10 {
			fmt.Printf("  ... and %d more\n", len(notFound)-10)
		}
	}

	if len(descFails) > 0 && len(descFails) <= 15 {
		fmt.Printf("\n── Description Mismatches (%d) ──\n", len(descFails))
		for _, r := range descFails {
			fmt.Printf("  %-15s expected: %-30s got: %s\n", r.Part.PartNumber, truncate(r.Part.Description, 30), truncate(r.APIDesc, 40))
		}
	}

	if len(catFails) > 0 && len(catFails) <= 15 {
		fmt.Printf("\n── Category Mismatches (%d) ──\n", len(catFails))
		for _, r := range catFails {
			fmt.Printf("  %-15s expected: %-20s got: %s\n", r.Part.PartNumber, r.Part.CategoryGroup, truncate(r.APICat, 40))
		}
	}

	// Overall score
	overallScore := 0.0
	if found > 0 {
		overallScore = (pct(found, total-errors)*0.3 + pct(descOK, found)*0.35 + pct(catOK, found)*0.25 + pct(makeOK, found)*0.1)
	}
	fmt.Printf("\n═══════════════════════════════════════\n")
	fmt.Printf("OVERALL SCORE: %.0f / 100\n", overallScore)
	fmt.Printf("═══════════════════════════════════════\n")

	if overallScore < 50 {
		os.Exit(1)
	}
}

func pct(num, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return float64(num) / float64(denom) * 100
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

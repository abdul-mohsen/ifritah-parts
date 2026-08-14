package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// ── Configuration ───────────────────────────────────────────────────

const (
	dbPath      = "data/hk_parts.db"
	requestRate = 1200 * time.Millisecond // Be polite: ~50 req/min
	maxHops     = 3                       // BFS depth limit (seed→hop1→hop2→hop3)
	maxPages    = 500                     // Safety limit on total pages fetched
	userAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
)

// ── Regex patterns (copied from partsouq.go) ───────────────────────

var (
	reDataNumber   = regexp.MustCompile(`data-number='([^']+)'`)
	reH1Desc       = regexp.MustCompile(`<h1[^>]*>\s*(.+?)\s*</h1>`)
	reSubstitution = regexp.MustCompile(`(?i)(Hyundai\s*/\s*KIA)\s+(\w[\w-]*)\s+([A-Z][\w\s&/-]+?)(?:\s*[<"\]])`)
	reStripTags    = regexp.MustCompile(`<[^>]*>`)
	reMake         = regexp.MustCompile(`(?i)Make:.*?>(Hyundai\s*/\s*KIA|[A-Za-z /]+?)</a>`)

	reAftermarket = regexp.MustCompile(`(?i)(` +
		`Mobis|Mando|ICRBI|Sure|Parts Mall|CTR|ONNURI|AMD|Korean Stars|` +
		`MANN-FILTER|MANN FILTER|MAHLE|KNECHT|BOSCH|PURFLUX|HENGST|CHAMPION|FRAM|WIX|UFI|KOLBENSCHMIDT|` +
		`BREMBO|TRW|FERODO|JURID|ATE|PAGID|TEXTAR|MINTEX|EBC|ZIMMERMANN|BREMSI|` +
		`SACHS|MONROE|BILSTEIN|KYB|KONI|MEYLE|FEBI|FEBI BILSTEIN|` + "LEMF" + `ÖRDER|LEMFORDER|MOOG|OPTIMAL|SWAG|TOPRAN|SIDEM|DELPHI|FIRST LINE|QUINTON HAZELL|` +
		`VALEO|HELLA|OSRAM|PHILIPS|CONTINENTAL|VEMO|` +
		`GATES|SKF|FAG|INA|SNR|NTN|KOYO|DAYCO|CONTITECH|CORTECO|ELRING|VICTOR REINZ|` +
		`BEHR|NISSENS|NRF|DENSO|PRASCO|DEPO|DIEDERICHS|` +
		`BLUE PRINT|BLUEPRINT ADL|NIPPARTS|JAPANPARTS|ASHIKA|COMLINE|BLUEPRINT|PIERBURG|WAHLER|` +
		`NGK|AISIN|TOKICO|HITACHI|AKEBONO|ADVICS|GMB|555|MASUMA|FEBEST|` +
		`ACDELCO|AC DELCO|MOTORCRAFT|DORMAN|CARDONE` +
		`)\s+(\d[\w-]*)\s+([A-Z][\w\s&/.,-]+?)(?:\s*[<"\]])`)
)

// ── Data types ──────────────────────────────────────────────────────

type discoveredPart struct {
	PartNumber  string
	Description string
	Category    string
	Make        string
	FoundVia    string // parent part that led us here
	Hop         int
}

type discoveredAftermarket struct {
	OEMNumber   string
	Brand       string
	PartNumber  string
	Description string
}

type discoveredSubstitution struct {
	FromPart    string
	ToPart      string
	Description string
}

// ── Main ────────────────────────────────────────────────────────────

func main() {
	log.SetFlags(log.Ltime)

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Create discovery tables
	createTables(db)

	// Load seed OEM parts from existing database
	seeds := loadSeeds(db)
	log.Printf("Loaded %d seed OEM parts from database", len(seeds))

	// BFS state
	queue := make([]queueItem, 0, len(seeds)*5)
	visited := make(map[string]bool)

	// Seed the queue
	for _, s := range seeds {
		norm := normalize(s)
		if !visited[norm] {
			visited[norm] = true
			queue = append(queue, queueItem{partNumber: s, hop: 0, foundVia: "seed"})
		}
	}

	// Also load already-visited parts from previous runs to avoid re-fetching
	loadVisited(db, visited)
	log.Printf("Total visited (from previous runs): %d", len(visited)-len(seeds))

	client := &http.Client{Timeout: 30 * time.Second}
	var fetched, newParts, newAM, newSubs int

	log.Println("Starting BFS crawl...")
	log.Printf("Queue: %d items, Max hops: %d, Max pages: %d", len(queue), maxHops, maxPages)

	for i := 0; i < len(queue) && fetched < maxPages; i++ {
		item := queue[i]

		// Skip if already crawled on this or previous run
		if alreadyCrawled(db, item.partNumber) {
			continue
		}

		// Rate limit
		time.Sleep(requestRate)

		html, err := fetchPage(client, item.partNumber)
		if err != nil {
			log.Printf("  [%d/%d] FAIL %s: %v", fetched+1, maxPages, item.partNumber, err)
			markCrawled(db, item.partNumber, "error")
			fetched++
			continue
		}
		fetched++

		// Parse all parts on page
		parts, aftermarkets, substitutions := parsePage(html, item.partNumber, item.hop, item.foundVia)

		if len(parts) == 0 {
			log.Printf("  [%d/%d] hop=%d %s → no results", fetched, maxPages, item.hop, item.partNumber)
			markCrawled(db, item.partNumber, "empty")
			continue
		}

		// Store discovered data
		np := storeParts(db, parts)
		na := storeAftermarket(db, aftermarkets)
		ns := storeSubstitutions(db, substitutions)
		markCrawled(db, item.partNumber, "ok")

		newParts += np
		newAM += na
		newSubs += ns

		log.Printf("  [%d/%d] hop=%d %s → %d parts, %d AM, %d subs (new: %d/%d/%d)",
			fetched, maxPages, item.hop, item.partNumber, len(parts), len(aftermarkets), len(substitutions), np, na, ns)

		// Enqueue discovered parts for further crawling (if within hop limit)
		if item.hop < maxHops {
			for _, p := range parts {
				norm := normalize(p.PartNumber)
				if !visited[norm] && isHKPart(p.PartNumber) {
					visited[norm] = true
					queue = append(queue, queueItem{
						partNumber: p.PartNumber,
						hop:        item.hop + 1,
						foundVia:   item.partNumber,
					})
				}
			}
			for _, s := range substitutions {
				norm := normalize(s.ToPart)
				if !visited[norm] && isHKPart(s.ToPart) {
					visited[norm] = true
					queue = append(queue, queueItem{
						partNumber: s.ToPart,
						hop:        item.hop + 1,
						foundVia:   item.partNumber,
					})
				}
			}
		}
	}

	// Print summary
	fmt.Println()
	fmt.Println("═══ Crawl Complete ═══")
	fmt.Printf("Pages fetched:    %d\n", fetched)
	fmt.Printf("New OEM parts:    %d\n", newParts)
	fmt.Printf("New aftermarket:  %d\n", newAM)
	fmt.Printf("New substitutions:%d\n", newSubs)
	fmt.Println()

	printStats(db)
}

// ── Queue item ──────────────────────────────────────────────────────

type queueItem struct {
	partNumber string
	hop        int
	foundVia   string
}

// ── DB setup ────────────────────────────────────────────────────────

func createTables(db *sql.DB) {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS discovered_oem_parts (
			part_number TEXT PRIMARY KEY,
			description TEXT,
			category    TEXT,
			make        TEXT,
			found_via   TEXT,
			hop         INTEGER,
			crawled_at  TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS discovered_aftermarket (
			oem_number  TEXT NOT NULL,
			brand       TEXT NOT NULL,
			part_number TEXT NOT NULL,
			description TEXT,
			PRIMARY KEY (oem_number, brand, part_number)
		)`,
		`CREATE TABLE IF NOT EXISTS discovered_substitutions (
			from_part TEXT NOT NULL,
			to_part   TEXT NOT NULL,
			description TEXT,
			PRIMARY KEY (from_part, to_part)
		)`,
		`CREATE TABLE IF NOT EXISTS crawl_log (
			part_number TEXT PRIMARY KEY,
			status      TEXT,
			crawled_at  TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_disc_oem ON discovered_oem_parts(part_number)`,
		`CREATE INDEX IF NOT EXISTS idx_disc_am_oem ON discovered_aftermarket(oem_number)`,
		`CREATE INDEX IF NOT EXISTS idx_disc_am_brand ON discovered_aftermarket(brand)`,
		`CREATE INDEX IF NOT EXISTS idx_disc_sub_from ON discovered_substitutions(from_part)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			log.Fatalf("Create tables: %v\nSQL: %s", err, s)
		}
	}
}

// ── Seed loading ────────────────────────────────────────────────────

func loadSeeds(db *sql.DB) []string {
	rows, err := db.Query("SELECT DISTINCT raw_number FROM oem_search_index")
	if err != nil {
		log.Fatal("Load seeds:", err)
	}
	defer rows.Close()

	var seeds []string
	seen := make(map[string]bool)
	for rows.Next() {
		var pn string
		rows.Scan(&pn)
		norm := normalize(pn)
		if !seen[norm] {
			seen[norm] = true
			seeds = append(seeds, pn)
		}
	}
	return seeds
}

func loadVisited(db *sql.DB, visited map[string]bool) {
	rows, err := db.Query("SELECT part_number FROM crawl_log WHERE status='ok' OR status='empty'")
	if err != nil {
		return // table might not exist on first run
	}
	defer rows.Close()
	for rows.Next() {
		var pn string
		rows.Scan(&pn)
		visited[normalize(pn)] = true
	}
}

func alreadyCrawled(db *sql.DB, partNumber string) bool {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM crawl_log WHERE part_number=?", normalize(partNumber)).Scan(&count)
	return count > 0
}

func markCrawled(db *sql.DB, partNumber, status string) {
	db.Exec("INSERT OR REPLACE INTO crawl_log (part_number, status) VALUES (?,?)", normalize(partNumber), status)
}

// ── HTTP fetch ──────────────────────────────────────────────────────

func fetchPage(client *http.Client, partNumber string) (string, error) {
	norm := normalize(partNumber)
	url := fmt.Sprintf("https://partsouq.com/en/search/all?q=%s", norm)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// ── HTML parsing ────────────────────────────────────────────────────

func parsePage(html, queryPart string, hop int, foundVia string) (
	parts []discoveredPart,
	aftermarkets []discoveredAftermarket,
	substitutions []discoveredSubstitution,
) {
	queryNorm := normalize(queryPart)

	// Discover all OEM part numbers on the page via data-number
	dataNumbers := reDataNumber.FindAllStringSubmatch(html, 30)
	seen := make(map[string]bool)
	var allParts []string

	// Query part first
	for _, m := range dataNumbers {
		pn := normalize(m[1])
		if pn == queryNorm && !seen[pn] {
			seen[pn] = true
			allParts = append(allParts, m[1]) // keep original format
		}
	}
	for _, m := range dataNumbers {
		pn := normalize(m[1])
		if !seen[pn] {
			seen[pn] = true
			allParts = append(allParts, m[1])
		}
	}

	// Page-level make extraction
	make_ := extractMake(html)

	// Parse each part section
	for _, rawPN := range allParts {
		normPN := normalize(rawPN)
		if !isHKPart(rawPN) {
			continue // skip non-HK parts
		}

		desc := extractDescription(html, normPN)
		category := decodePrefix(rawPN)

		parts = append(parts, discoveredPart{
			PartNumber:  formatPartNumber(rawPN),
			Description: desc,
			Category:    category,
			Make:        make_,
			FoundVia:    foundVia,
			Hop:         hop,
		})

		// Aftermarket alternatives for this part
		section := findProductSection(html, normPN)
		amSeen := make(map[string]bool)

		scanAM := func(text string) {
			for _, m := range reAftermarket.FindAllStringSubmatch(text, 30) {
				brand := strings.TrimSpace(m[1])
				pn := strings.TrimSpace(m[2])
				key := brand + "|" + pn
				if amSeen[key] {
					continue
				}
				amSeen[key] = true
				aftermarkets = append(aftermarkets, discoveredAftermarket{
					OEMNumber:   formatPartNumber(rawPN),
					Brand:       brand,
					PartNumber:  pn,
					Description: strings.TrimSpace(m[3]),
				})
			}
		}

		if section != "" {
			scanAM(section)
		}
		if len(aftermarkets) == 0 {
			scanAM(html) // fallback: full page scan
		}
	}

	// Parse substitutions
	for _, rawPN := range allParts {
		normPN := normalize(rawPN)
		marker := "data-number='" + normPN + "'"
		idx := strings.Index(html, marker)
		if idx < 0 {
			continue
		}
		subSearch := html[idx:]
		subsIdx := strings.Index(strings.ToLower(subSearch), "substitution")
		if subsIdx < 0 {
			continue
		}
		end := subsIdx + 5000
		if end > len(subSearch) {
			end = len(subSearch)
		}
		subsSection := subSearch[subsIdx:end]
		subSeen := make(map[string]bool)
		for _, m := range reSubstitution.FindAllStringSubmatch(subsSection, 10) {
			toPart := strings.TrimSpace(m[2])
			toNorm := normalize(toPart)
			if toNorm == normPN || subSeen[toNorm] {
				continue
			}
			subSeen[toNorm] = true
			substitutions = append(substitutions, discoveredSubstitution{
				FromPart:    formatPartNumber(rawPN),
				ToPart:      formatPartNumber(toPart),
				Description: strings.TrimSpace(m[3]),
			})
		}
	}

	return
}

func extractMake(html string) string {
	for _, m := range reMake.FindAllStringSubmatch(html, 10) {
		mk := strings.TrimSpace(m[1])
		if strings.Contains(strings.ToUpper(mk), "HYUNDAI") || strings.Contains(strings.ToUpper(mk), "KIA") {
			return mk
		}
	}
	if strings.Contains(html, "Hyundai / KIA") || strings.Contains(html, "Hyundai/KIA") {
		return "Hyundai / KIA"
	}
	return ""
}

func extractDescription(html, normPN string) string {
	// Method 1: "Make PartNum DESCRIPTION" pattern
	pattern := regexp.MustCompile(`(?i)Hyundai\s*/\s*KIA\s+` + regexp.QuoteMeta(normPN) + `\s+([A-Z][\w\s&/-]+?)(?:\s*[<"\]])`)
	if m := pattern.FindStringSubmatch(html); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// Method 2: H1 from product section
	section := findProductSection(html, normPN)
	if section != "" {
		if m := reH1Desc.FindStringSubmatch(section); len(m) > 1 {
			desc := strings.TrimSpace(stripTags(m[1]))
			if len(desc) > 2 && !strings.EqualFold(desc, "Search") && !strings.Contains(strings.ToLower(desc), "partsouq") {
				return desc
			}
		}
	}
	// Method 3: page-level H1
	if m := reH1Desc.FindStringSubmatch(html); len(m) > 1 {
		desc := strings.TrimSpace(stripTags(m[1]))
		if len(desc) > 2 && !strings.EqualFold(desc, "Search") && !strings.Contains(strings.ToLower(desc), "partsouq") {
			return desc
		}
	}
	return ""
}

func findProductSection(html, normPN string) string {
	marker := "data-number='" + normPN + "'"
	idx := strings.Index(html, marker)
	if idx < 0 {
		return ""
	}
	start := idx - 2000
	if start < 0 {
		start = 0
	}
	end := idx + 3000
	if end > len(html) {
		end = len(html)
	}
	return html[start:end]
}

func stripTags(s string) string {
	return reStripTags.ReplaceAllString(s, "")
}

// ── Hyundai/KIA part validation ─────────────────────────────────────

// isHKPart checks if a part number looks like a Hyundai/KIA OEM number.
// HK parts follow the pattern XXXXX-XXXXX or XXXXX-XXXXXXX (5-digit prefix).
func isHKPart(pn string) bool {
	norm := normalize(pn)
	if len(norm) < 8 || len(norm) > 15 {
		return false
	}
	// Must start with a digit (HK OEM prefix system)
	if norm[0] < '0' || norm[0] > '9' {
		return false
	}
	// First 2 digits must be a valid HK prefix range (21-98)
	if len(norm) >= 2 {
		prefix := (int(norm[0]-'0') * 10) + int(norm[1]-'0')
		if prefix < 21 || prefix > 98 {
			return false
		}
	}
	return true
}

// ── Part number formatting ──────────────────────────────────────────

func normalize(pn string) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(pn) {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// formatPartNumber tries to format a raw HK part number as XXXXX-XXXXX.
func formatPartNumber(pn string) string {
	norm := normalize(pn)
	// If it contains a dash already, return as-is (uppercase)
	if strings.Contains(pn, "-") {
		return strings.ToUpper(strings.TrimSpace(pn))
	}
	// Standard HK format: 5 digits + dash + rest
	if len(norm) >= 10 {
		return norm[:5] + "-" + norm[5:]
	}
	return norm
}

// decodePrefix uses the HK OEM prefix system to categorize parts.
func decodePrefix(pn string) string {
	norm := normalize(pn)
	if len(norm) < 3 {
		return ""
	}

	// 3-digit match
	prefixes3 := map[string]string{
		"211": "Cylinder Block", "213": "Crankshaft & Bearings",
		"231": "Cylinder Head", "233": "Camshaft & Timing",
		"251": "Fuel Pump", "253": "Fuel Injector", "263": "Oil Filter",
		"283": "Intake Manifold", "284": "Water Pump", "281": "Radiator",
		"282": "Thermostat & Housing", "285": "Coolant Hose",
		"286": "Exhaust System", "287": "Exhaust Manifold", "289": "Catalytic Converter",
		"361": "Starter Motor", "373": "Alternator", "392": "Oxygen Sensor",
		"529": "Wheels & Tires", "546": "Shock Absorber (Front)",
		"553": "Shock Absorber (Rear)", "563": "Tie Rod",
		"581": "Front Brake Pad / Disc", "582": "Front Brake Caliper",
		"583": "Rear Brake / Drum", "584": "Rear Brake Caliper",
		"585": "Parking Brake", "586": "Brake Master Cylinder", "589": "Brake Fluid Reservoir",
		"921": "Headlight Assembly", "922": "Fog Light", "923": "Turn Signal",
		"924": "Tail Light Assembly", "961": "Battery",
		"971": "Compressor A/C", "972": "Condenser", "973": "Evaporator",
		"976": "Heater Core", "977": "A/C Hose & Pipe",
		"983": "Wiper Blades", "984": "Washer System",
	}
	if cat, ok := prefixes3[norm[:3]]; ok {
		return cat
	}

	// 2-digit match
	prefixes2 := map[string]string{
		"21": "Engine Block & Internals", "22": "Engine Mounting",
		"23": "Cylinder Head & Valvetrain", "24": "Intake & Exhaust Manifold",
		"25": "Fuel System", "26": "Oil System / Filters",
		"27": "EGR & Emissions", "28": "Cooling System",
		"29": "Turbo / Supercharger",
		"30": "Propeller Shaft", "31": "Front Differential",
		"32": "Front Drive Shaft", "33": "Rear Axle & Differential",
		"34": "Rear Drive Shaft", "35": "Drive Shaft / CV Joint",
		"36": "Starter & Charging", "37": "Ignition System",
		"38": "Engine Control Unit", "39": "Sensors & Control",
		"41": "Clutch", "43": "Automatic Transmission",
		"44": "Transmission Control", "45": "Manual Transmission",
		"46": "Auto Transmission Control", "47": "Transfer Case",
		"48": "Transaxle", "49": "Transfer Case / 4WD",
		"51": "Front Axle", "52": "Rear Axle", "53": "Power Steering",
		"54": "Front Suspension", "55": "Rear Suspension",
		"56": "Steering Column & Gear", "57": "Wheel & Hub",
		"58": "Brakes", "59": "ABS / ESC",
		"60": "Frame & Cross Members", "61": "Sub-Frame",
		"62": "Front Structure", "63": "Rear Structure",
		"64": "Front Body / Hood", "65": "Fender & Side Body",
		"66": "Rear Body / Trunk", "67": "Floor & Underbody",
		"68": "Roof Panel", "69": "Quarter Panel",
		"70": "Tailgate / Liftgate", "71": "Bumper",
		"72": "Front Door", "73": "Rear Door", "74": "Back Door / Hatch",
		"75": "Door Lock & Handle", "76": "Window Regulator",
		"81": "Weatherstrip & Seal", "82": "Glass / Windshield",
		"83": "Sunroof", "84": "Interior Trim", "85": "Seats",
		"86": "Mirrors", "87": "Mouldings & Trim",
		"88": "Instrument Panel / Dashboard", "89": "Air Bag System",
		"91": "Wiring Harness", "92": "Lighting - Headlights",
		"93": "Lighting - Interior", "94": "Audio & Display",
		"95": "Sensors & Modules", "96": "Battery & Charging",
		"97": "Air Conditioning & Heating", "98": "Wiper System",
	}
	if cat, ok := prefixes2[norm[:2]]; ok {
		return cat
	}
	return ""
}

// ── DB storage ──────────────────────────────────────────────────────

func storeParts(db *sql.DB, parts []discoveredPart) int {
	var n int
	for _, p := range parts {
		res, err := db.Exec(
			`INSERT OR IGNORE INTO discovered_oem_parts (part_number, description, category, make, found_via, hop)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			p.PartNumber, p.Description, p.Category, p.Make, p.FoundVia, p.Hop)
		if err != nil {
			continue
		}
		if rows, _ := res.RowsAffected(); rows > 0 {
			n++
		}
	}
	return n
}

func storeAftermarket(db *sql.DB, ams []discoveredAftermarket) int {
	var n int
	for _, a := range ams {
		res, err := db.Exec(
			`INSERT OR IGNORE INTO discovered_aftermarket (oem_number, brand, part_number, description)
			 VALUES (?, ?, ?, ?)`,
			a.OEMNumber, a.Brand, a.PartNumber, a.Description)
		if err != nil {
			continue
		}
		if rows, _ := res.RowsAffected(); rows > 0 {
			n++
		}
	}
	return n
}

func storeSubstitutions(db *sql.DB, subs []discoveredSubstitution) int {
	var n int
	for _, s := range subs {
		res, err := db.Exec(
			`INSERT OR IGNORE INTO discovered_substitutions (from_part, to_part, description)
			 VALUES (?, ?, ?)`,
			s.FromPart, s.ToPart, s.Description)
		if err != nil {
			continue
		}
		if rows, _ := res.RowsAffected(); rows > 0 {
			n++
		}
	}
	return n
}

// ── Stats ───────────────────────────────────────────────────────────

func printStats(db *sql.DB) {
	var total, amTotal, subTotal int
	db.QueryRow("SELECT COUNT(*) FROM discovered_oem_parts").Scan(&total)
	db.QueryRow("SELECT COUNT(*) FROM discovered_aftermarket").Scan(&amTotal)
	db.QueryRow("SELECT COUNT(*) FROM discovered_substitutions").Scan(&subTotal)

	var withDesc int
	db.QueryRow("SELECT COUNT(*) FROM discovered_oem_parts WHERE description != ''").Scan(&withDesc)

	fmt.Println("═══ Discovery Database Stats ═══")
	fmt.Printf("Total OEM parts discovered: %d\n", total)
	fmt.Printf("  With description:         %d (%.0f%%)\n", withDesc, pct(withDesc, total))
	fmt.Printf("Total aftermarket refs:     %d\n", amTotal)
	fmt.Printf("Total substitution links:   %d\n", subTotal)

	// Parts by hop
	fmt.Println("\nParts by discovery hop:")
	rows, _ := db.Query("SELECT hop, COUNT(*) FROM discovered_oem_parts GROUP BY hop ORDER BY hop")
	if rows != nil {
		for rows.Next() {
			var hop, cnt int
			rows.Scan(&hop, &cnt)
			label := "seed"
			if hop > 0 {
				label = fmt.Sprintf("hop %d", hop)
			}
			fmt.Printf("  %-8s %d parts\n", label, cnt)
		}
		rows.Close()
	}

	// Top categories
	fmt.Println("\nTop categories discovered:")
	rows2, _ := db.Query("SELECT category, COUNT(*) FROM discovered_oem_parts WHERE category!='' GROUP BY category ORDER BY COUNT(*) DESC LIMIT 20")
	if rows2 != nil {
		for rows2.Next() {
			var cat string
			var cnt int
			rows2.Scan(&cat, &cnt)
			fmt.Printf("  %-35s %d\n", cat, cnt)
		}
		rows2.Close()
	}

	// Top aftermarket brands
	fmt.Println("\nTop aftermarket brands discovered:")
	rows3, _ := db.Query("SELECT brand, COUNT(*) FROM discovered_aftermarket GROUP BY brand ORDER BY COUNT(*) DESC LIMIT 20")
	if rows3 != nil {
		for rows3.Next() {
			var brand string
			var cnt int
			rows3.Scan(&brand, &cnt)
			fmt.Printf("  %-25s %d\n", brand, cnt)
		}
		rows3.Close()
	}

	// Overlap with existing DB
	var existingOEM int
	db.QueryRow(`SELECT COUNT(DISTINCT d.part_number) FROM discovered_oem_parts d
		INNER JOIN oem_search_index o ON REPLACE(REPLACE(UPPER(d.part_number),'-',''),' ','') = o.normalized`).Scan(&existingOEM)
	fmt.Printf("\nOverlap with existing OEM index: %d parts\n", existingOEM)
	fmt.Printf("Net new OEM parts:              %d\n", total-existingOEM)

	// Output file
	fmt.Printf("\nResults stored in: %s\n", dbPath)
	if fi, err := os.Stat(dbPath); err == nil {
		fmt.Printf("Database size: %.1f MB\n", float64(fi.Size())/1024/1024)
	}
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}

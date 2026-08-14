package main

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Scrape RockAuto.com to discover aftermarket cross-references for our OEM parts.
// Pattern: popup-widget-title.*?<span[^>]*>([^<]+)</span> → yields "BRAND PARTNUMBER"
//
// This is a ONE-TIME data collection script. We store results in aftermarket_crossref table.

type OEMPart struct {
	oem      string
	category string
}

var allParts = []OEMPart{
	// Oil Filters
	{"26300-35505", "Oil Filter"},
	{"26300-35504", "Oil Filter"},
	{"26300-35503", "Oil Filter"},
	{"26300-35530", "Oil Filter"},
	{"26300-02503", "Oil Filter"},
	{"26310-27200", "Oil Filter"},
	{"26300-21A00", "Oil Filter"},
	{"26300-3CAA0", "Oil Filter"},
	{"26310-27400", "Oil Filter"},
	// Air Filters
	{"28113-D3100", "Air Filter"},
	{"28113-F2100", "Air Filter"},
	{"28113-A9100", "Air Filter"},
	{"28113-2S000", "Air Filter"},
	{"28113-C1100", "Air Filter"},
	// Cabin Filters
	{"97133-D3000", "Cabin Filter"},
	{"97133-F2000", "Cabin Filter"},
	{"97133-2E250", "Cabin Filter"},
	{"97133-C1000", "Cabin Filter"},
	// Fuel Filters
	{"31112-1R000", "Fuel Filter"},
	{"31922-2E900", "Fuel Filter"},
	// Brake Pads Front
	{"58101-D3A70", "Brake Pads"},
	{"58101-2SA70", "Brake Pads"},
	{"58101-1RA00", "Brake Pads"},
	{"58101-A7A70", "Brake Pads"},
	// Brake Pads Rear
	{"58302-D3A70", "Brake Pads"},
	{"58302-2SA30", "Brake Pads"},
	// Brake Discs
	{"51712-D3100", "Brake Disc"},
	{"58411-D3300", "Brake Disc"},
	// Spark Plugs
	{"18843-10062", "Spark Plug"},
	{"18843-08062", "Spark Plug"},
	// Ignition Coils
	{"27301-2B100", "Ignition Coil"},
	// Shock Absorbers
	{"54651-D3000", "Shock Absorber"},
	{"55310-D3000", "Shock Absorber"},
	{"54651-2S000", "Shock Absorber"},
	{"54651-F2000", "Shock Absorber"},
	// Radiators
	{"25310-2S500", "Radiator"},
	{"25310-D3050", "Radiator"},
	// Water Pumps
	{"25100-2B000", "Water Pump"},
	{"25100-2G500", "Water Pump"},
	{"25100-2B700", "Water Pump"},
	// Timing
	{"24312-2B000", "Timing"},
	// Fuel Injectors
	{"35310-2S000", "Fuel Injector"},
	// O2 Sensors
	{"39210-2B100", "Lambda Sensor"},
	// Alternators
	{"37300-2B150", "Alternator"},
	// A/C Compressors
	{"97701-D3000", "A/C Compressor"},
	// Wiper Blades
	{"98350-D3100", "Wiper Blades"},
	// Wheel Bearings
	{"51720-D3000", "Wheel Bearing"},
	{"51720-2S000", "Wheel Bearing"},
	{"51720-1J000", "Wheel Bearing"},
	// Tie Rod Ends
	{"56820-D3000", "Tie Rod End"},
	{"56820-C1000", "Tie Rod End"},
	// Control Arms
	{"54500-D3000", "Control Arm"},
	{"54500-F2000", "Control Arm"},
	// Stabilizer Links
	{"54830-D3000", "Stabilizer Link"},
	{"54830-F2000", "Stabilizer Link"},
	// Clutch
	{"41100-24520", "Clutch"},
	// Engine Mounts
	{"21810-2S000", "Engine Mount"},
	{"21810-C1000", "Engine Mount"},
	// Thermostats
	{"25500-2B100", "Thermostat"},
	{"25500-27050", "Thermostat"},
	// Starter Motors
	{"36100-2B100", "Starter Motor"},
	// TPMS
	{"52933-1P000", "TPMS Sensor"},
	// Drive Belts
	{"25212-2B020", "Drive Belt"},
	// Belt Tensioners
	{"25281-2B010", "Belt Tensioner"},
	// Ball Joints
	{"54530-D3000", "Ball Joint"},
	// CV Joints
	{"49501-D3200", "CV Joint"},
	// Bulbs
	{"18649-55009L", "Bulb"},
}

// Brands to skip (OEM, generic, or not useful)
var skipBrands = map[string]bool{
	"HYUNDAI":     true,
	"KIA":         true,
	"MOBIS":       true,
	"VARIOUS MFR": true,
	"VARIOUS":     true,
	"OEM":         true,
	"GENUINE":     true,
}

func main() {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	reProduct := regexp.MustCompile(`(?s)popup-widget-title.*?<span[^>]*padding[^>]*>([^<]+)</span>`)

	out, _ := os.Create("rockauto_collection.txt")
	defer out.Close()
	w := func(s string, args ...interface{}) {
		msg := fmt.Sprintf(s, args...)
		fmt.Print(msg)
		out.WriteString(msg)
	}

	w("╔═══════════════════════════════════════════════════════════════════╗\n")
	w("║  ROCKAUTO AFTERMARKET CROSS-REFERENCE COLLECTION                ║\n")
	w("║  Collecting data for %d OEM parts                               ║\n", len(allParts))
	w("╚═══════════════════════════════════════════════════════════════════╝\n\n")

	type CrossRef struct {
		oem      string
		brand    string
		partNum  string
		desc     string
		category string
	}

	var allCrossRefs []CrossRef
	partsWithData := 0
	partsNoData := 0

	for i, p := range allParts {
		url := fmt.Sprintf("https://www.rockauto.com/en/partsearch/?partnum=%s", p.oem)

		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		resp, err := client.Do(req)
		if err != nil {
			w("[%d/%d] ✗ %s (%s): ERROR %v\n", i+1, len(allParts), p.oem, p.category, err)
			partsNoData++
			time.Sleep(5 * time.Second)
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		resp.Body.Close()
		html := string(body)

		if resp.StatusCode != 200 {
			w("[%d/%d] ✗ %s (%s): HTTP %d\n", i+1, len(allParts), p.oem, p.category, resp.StatusCode)
			partsNoData++
			time.Sleep(5 * time.Second)
			continue
		}

		// Extract products: "BRAND PARTNUMBER"
		matches := reProduct.FindAllStringSubmatch(html, -1)
		var found []CrossRef
		seen := map[string]bool{}

		for _, m := range matches {
			raw := strings.TrimSpace(m[1])
			if raw == "" {
				continue
			}

			// Split into brand + part number
			// The format is "BRAND PARTNUMBER" where brand can be multi-word
			// Strategy: Find the last space-separated token as part number
			parts := strings.Fields(raw)
			if len(parts) < 2 {
				continue
			}

			partNum := parts[len(parts)-1]
			brand := strings.Join(parts[:len(parts)-1], " ")

			// Handle special cases like "BECK/ARNLEY" which is one brand
			brand = strings.ToUpper(brand)

			if skipBrands[brand] {
				continue
			}

			// Skip the OEM part itself
			normalized := strings.ReplaceAll(strings.ToUpper(partNum), "-", "")
			oemNorm := strings.ReplaceAll(strings.ToUpper(p.oem), "-", "")
			if normalized == oemNorm {
				continue
			}

			key := brand + "|" + partNum
			if seen[key] {
				continue
			}
			seen[key] = true

			desc := fmt.Sprintf("%s for %s", p.category, p.oem)
			found = append(found, CrossRef{
				oem:      p.oem,
				brand:    brand,
				partNum:  partNum,
				desc:     desc,
				category: p.category,
			})
		}

		if len(found) > 0 {
			partsWithData++
			w("[%d/%d] ★ %s (%s) — %d aftermarket:\n", i+1, len(allParts), p.oem, p.category, len(found))
			for _, f := range found {
				w("         %s → %s\n", f.brand, f.partNum)
			}
			allCrossRefs = append(allCrossRefs, found...)
		} else {
			partsNoData++
			w("[%d/%d] ○ %s (%s) — no results\n", i+1, len(allParts), p.oem, p.category)
		}

		// Polite delay: 4-6 seconds between requests
		delay := 4 + (i % 3)
		time.Sleep(time.Duration(delay) * time.Second)
	}

	w("\n╔═══════════════════════════════════════════════════════════════════╗\n")
	w("║                    COLLECTION SUMMARY                            ║\n")
	w("╠═══════════════════════════════════════════════════════════════════╣\n")
	w("║  Total OEM parts queried:  %d                                    \n", len(allParts))
	w("║  Parts with data:          %d                                    \n", partsWithData)
	w("║  Parts without data:       %d                                    \n", partsNoData)
	w("║  Total cross-references:   %d                                    \n", len(allCrossRefs))
	w("╚═══════════════════════════════════════════════════════════════════╝\n\n")

	if len(allCrossRefs) == 0 {
		w("No data collected. Exiting.\n")
		return
	}

	// Count unique brands
	brandCount := map[string]int{}
	for _, cr := range allCrossRefs {
		brandCount[cr.brand]++
	}
	w("Unique brands (%d):\n", len(brandCount))
	for b, c := range brandCount {
		w("  %s: %d cross-refs\n", b, c)
	}

	// Now insert into the database
	dbPath := "../../data/hk_parts.db"
	w("\n═══ Inserting into database: %s ═══\n", dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		w("ERROR opening DB: %v\n", err)
		return
	}
	defer db.Close()

	// Ensure table exists
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS aftermarket_crossref (
		oem_number  TEXT NOT NULL,
		brand       TEXT NOT NULL,
		part_number TEXT NOT NULL,
		description TEXT,
		category    TEXT,
		verified    INTEGER DEFAULT 1,
		PRIMARY KEY (oem_number, brand, part_number)
	)`)
	if err != nil {
		w("ERROR creating table: %v\n", err)
		return
	}

	// Insert with ON CONFLICT to merge with existing data
	inserted := 0
	skipped := 0
	for _, cr := range allCrossRefs {
		_, err := db.Exec(`INSERT OR IGNORE INTO aftermarket_crossref (oem_number, brand, part_number, description, category, verified)
			VALUES (?, ?, ?, ?, ?, 1)`,
			strings.ToLower(strings.ReplaceAll(cr.oem, "-", "")),
			cr.brand, cr.partNum, cr.desc, cr.category)
		if err != nil {
			w("  INSERT ERROR for %s/%s/%s: %v\n", cr.oem, cr.brand, cr.partNum, err)
			skipped++
		} else {
			inserted++
		}
	}

	w("\n═══ DATABASE UPDATE COMPLETE ═══\n")
	w("  Inserted/Updated: %d\n", inserted)
	w("  Skipped (dupes):  %d\n", skipped)

	// Count total rows
	var total int
	db.QueryRow("SELECT COUNT(*) FROM aftermarket_crossref").Scan(&total)
	w("  Total rows in aftermarket_crossref: %d\n", total)

	var brands int
	db.QueryRow("SELECT COUNT(DISTINCT brand) FROM aftermarket_crossref").Scan(&brands)
	w("  Total unique brands: %d\n", brands)

	var oems int
	db.QueryRow("SELECT COUNT(DISTINCT oem_number) FROM aftermarket_crossref").Scan(&oems)
	w("  Total unique OEM parts covered: %d\n", oems)
}

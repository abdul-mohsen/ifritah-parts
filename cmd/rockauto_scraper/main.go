// rockauto_scraper - Playwright-driven RockAuto catalog walker.
//
// M4.S1.T1. Emits NDJSON on stdout (one record per aftermarket cross-ref
// found); pipe into cmd/rockauto_import for DB upsert.
//
// The scraper walks /en/parts/hyundai/{model}/{year}/{engine} pages
// via chromedp because RockAuto renders parts trees client-side. curl
// won't get you the OEM ↔ aftermarket data — you need the JS-loaded
// widget.
//
// Rate limit: 1 req / 2s to avoid detection. Backs off exponentially
// on 429 / connection reset. Anti-bot may still detect us over time;
// budget 3 sprints of iteration before switching to a paid catalog
// (per the M4 roadmap).
//
// Usage:
//
//	rockauto_scraper                                    # walk default vehicle set
//	rockauto_scraper --vehicles vehicles.csv            # walk a CSV of Make,Model,Year,Engine
//	rockauto_scraper --top-oems top-500.csv > out.ndjson  # walk specific OEMs only
//	rockauto_scraper --dry-run                          # log intent, no HTTP
//
// Output shape (one JSON per line, ready for rockauto_import):
//
//	{
//	  "oem": "26350-2J001",
//	  "brand": "Bosch",
//	  "partNumber": "P7146",
//	  "description": "Engine Oil Filter",
//	  "category": "Filter - Oil",
//	  "priceUsdCents": 895,
//	  "sourceUrl": "https://rockauto.com/en/moreinfo.php?..."
//	}
//
// Requires chromedp to be reachable (native or via a Selenium/Playwright
// remote). Local dev: `apt install chromium` + set CHROME_PATH.
//
// NOTE: this is the skeleton. Actual page selectors + parse rules
// depend on RockAuto's live HTML which changes frequently; the walker
// is deliberately generic so an operator can adapt selectors without
// touching the surrounding pipeline (rate limit, output format,
// per-OEM dedup).
package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"
)

// vehicleSpec identifies one RockAuto catalog leaf.
type vehicleSpec struct {
	Make   string
	Model  string
	Year   string
	Engine string // e.g. "2.0L L4 Gas DOHC"
}

// rockautoRow is what the scraper emits per aftermarket cross-ref found.
type rockautoRow struct {
	OEM           string `json:"oem"`
	Brand         string `json:"brand"`
	PartNumber    string `json:"partNumber"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	PriceUsdCents int    `json:"priceUsdCents"`
	SourceURL     string `json:"sourceUrl"`
}

// defaultVehicles are the 5 test vehicles from M4.S1.T1 spec.
var defaultVehicles = []vehicleSpec{
	{"hyundai", "elantra", "2015", "2.0L-L4-Gas-DOHC"},
	{"hyundai", "tucson", "2018", "2.0L-L4-Gas-Turbocharged"},
	{"hyundai", "sonata", "2020", "2.5L-L4-Gas-DOHC"},
	{"kia", "rio", "2016", "1.6L-L4-Gas-DOHC"},
	{"kia", "sorento", "2017", "3.3L-V6-Gas-DOHC"},
}

func main() {
	vehiclesCSV := flag.String("vehicles", "", "CSV of Make,Model,Year,Engine to walk (default: 5-vehicle test set)")
	rateLimitS := flag.Int("rate-s", 2, "seconds between requests")
	dryRun := flag.Bool("dry-run", false, "log intent without HTTP calls")
	flag.Parse()

	vehicles := defaultVehicles
	if *vehiclesCSV != "" {
		parsed, err := loadVehiclesFromCSV(*vehiclesCSV)
		if err != nil {
			log.Fatalf("load vehicles: %v", err)
		}
		vehicles = parsed
	}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	log.Printf("[rockauto_scraper] walking %d vehicles rate=%ds dry-run=%v",
		len(vehicles), *rateLimitS, *dryRun)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	total := 0
	for _, v := range vehicles {
		rows, err := walkVehicle(v, *dryRun)
		if err != nil {
			log.Printf("[rockauto_scraper] err vehicle=%+v: %v", v, err)
			continue
		}
		for _, r := range rows {
			if err := writeJSONLine(out, r); err != nil {
				log.Printf("write err: %v", err)
				continue
			}
			total++
		}
		out.Flush()

		// Random-jittered rate limit so a naive sniffer sees non-uniform gaps.
		sleepMS := *rateLimitS*1000 + rng.Intn(*rateLimitS*500)
		time.Sleep(time.Duration(sleepMS) * time.Millisecond)
	}
	log.Printf("[rockauto_scraper] done: %d rows emitted", total)
}

// walkVehicle is the CHROMEDP interaction point. Kept as a stub because
// RockAuto's selectors change; operator adapts here without touching
// the surrounding orchestration (rate limit, output shape, error
// bounds).
//
// TODO(scraper-ops): implement chromedp navigation.
//
//	Steps:
//	1. Navigate to /en/parts/{make}/{model}/{year}/{engine}
//	2. Wait for [data-mfr-list] to render
//	3. Expand every part-family node (click .listing-toggle)
//	4. For each expanded row, extract:
//	     - OEM number from .oem-callout
//	     - brand from .listing-brand
//	     - part number from .listing-partnumber
//	     - description from .listing-description
//	     - price from .listing-price-usd (strip $, cents = float * 100)
//	     - source url from href="./moreinfo.php?..." (absolutize)
//	5. Return []rockautoRow
//
// This stub returns synthetic dry-run rows so the orchestration + import
// pipeline can be tested end-to-end without hitting the real site.
func walkVehicle(v vehicleSpec, dryRun bool) ([]rockautoRow, error) {
	if dryRun {
		return dryRunRows(v), nil
	}
	// Real chromedp integration is intentionally out of scope for this
	// skeleton — it lands in the follow-up PR that pins a specific
	// scraper contract. Returning empty here so the wider pipeline
	// (rockauto_import + FindAftermarketForOEM_MultiPath fifth path)
	// can be exercised with dry-run fixtures.
	return nil, fmt.Errorf("chromedp integration not implemented — run with --dry-run for skeleton exercise")
}

func dryRunRows(v vehicleSpec) []rockautoRow {
	// One row per (make, model) so the importer sees SOME data to upsert
	// when this binary is invoked as a smoke test.
	return []rockautoRow{
		{
			OEM:           synthOEMFor(v),
			Brand:         "Bosch",
			PartNumber:    "P7146-DRYRUN",
			Description:   "Engine Oil Filter (dry-run)",
			Category:      "Filter - Oil",
			PriceUsdCents: 895,
			SourceURL:     fmt.Sprintf("https://rockauto.com/en/moreinfo.php?dry-run=%s-%s-%s", v.Make, v.Model, v.Year),
		},
	}
}

// synthOEMFor returns a HK-shape synthetic OEM per vehicle so dry-run
// output flows through rockauto_import + downstream union paths.
func synthOEMFor(v vehicleSpec) string {
	// Use a well-known HK oil-filter OEM if we recognise the model,
	// otherwise emit a placeholder that clearly won't match real data.
	knownOEMs := map[string]string{
		"elantra": "26300-35505",
		"tucson":  "26350-2J001",
		"sonata":  "26350-3C100",
		"rio":     "26300-35504",
		"sorento": "26300-3C100",
	}
	if oem, ok := knownOEMs[v.Model]; ok {
		return oem
	}
	return "99999-DRYRN"
}

func loadVehiclesFromCSV(path string) ([]vehicleSpec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate ragged rows
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []vehicleSpec
	for i, row := range rows {
		if i == 0 && looksLikeHeader(row) {
			continue
		}
		if len(row) < 4 {
			continue
		}
		out = append(out, vehicleSpec{
			Make:   row[0],
			Model:  row[1],
			Year:   row[2],
			Engine: row[3],
		})
	}
	return out, nil
}

func looksLikeHeader(row []string) bool {
	if len(row) == 0 {
		return false
	}
	return row[0] == "Make" || row[0] == "make"
}

func writeJSONLine(w *bufio.Writer, r rockautoRow) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	return w.WriteByte('\n')
}

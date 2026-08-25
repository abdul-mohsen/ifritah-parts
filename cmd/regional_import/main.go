// regional_import - generic NDJSON→aftermarket_regional upserter.
//
// M4.S2.T2 scaffold. Every regional supplier scraper
// (scripts/scrapers/regional/{supplier}/main.go — future work)
// emits NDJSON in this shape; this binary reads stdin and upserts.
//
// Input record:
//
//	{
//	  "oem": "26350-2J001",
//	  "supplier": "ali_al_ghanim",
//	  "brand": "Bosch",
//	  "partNumber": "P7146",
//	  "description": "Engine Oil Filter",
//	  "stockStatus": "in_stock",
//	  "region": "KSA",
//	  "url": "https://ali-al-ghanim.example.com/...",
//	  "priceLocal": 45.50,
//	  "priceCurrency": "SAR"
//	}
//
// Usage:
//
//	cat ali-al-ghanim-2026-08-25.ndjson | ./regional_import
//
// --dry-run to preview upserts without writing.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"parts-engine/internal/config"
	"parts-engine/internal/db"
	"parts-engine/internal/service"
)

type regionalRecord struct {
	OEM           string  `json:"oem"`
	Supplier      string  `json:"supplier"`
	Brand         string  `json:"brand"`
	PartNumber    string  `json:"partNumber"`
	Description   string  `json:"description,omitempty"`
	StockStatus   string  `json:"stockStatus,omitempty"`
	Region        string  `json:"region,omitempty"`
	URL           string  `json:"url,omitempty"`
	PriceLocal    float64 `json:"priceLocal,omitempty"`
	PriceCurrency string  `json:"priceCurrency,omitempty"`
}

// knownSuppliers protects against accidental typos in the supplier
// column so unknown values don't silently pollute the table.
var knownSuppliers = map[string]bool{
	"ali_al_ghanim":         true,
	"al_futtaim":            true,
	"petromin":              true,
	"al_ghazlain_autoparts": true,
	"aljazirah_vehicles":    true,
	"agmc":                  true,
	"kanoo_motors":          true,
}

func main() {
	dryRun := flag.Bool("dry-run", false, "log rows without writing")
	strictSuppliers := flag.Bool("strict-suppliers", true, "reject unknown supplier values (default true)")
	flag.Parse()

	cfg := config.Load()

	pg := db.NewPostgres(cfg)
	if pg == nil {
		log.Fatal("postgres: connection failed")
	}
	defer pg.Close()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 65536), 1024*1024)

	total := 0
	inserted := 0
	skipped := 0
	errored := 0
	ctx := context.Background()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		total++

		var rec regionalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			errored++
			log.Printf("[regional_import] parse err line %d: %v", total, err)
			continue
		}

		clean := service.NormalizeOEM(rec.OEM)
		supplier := strings.TrimSpace(strings.ToLower(rec.Supplier))
		if clean == "" || supplier == "" || rec.PartNumber == "" {
			skipped++
			continue
		}
		if *strictSuppliers && !knownSuppliers[supplier] {
			log.Printf("[regional_import] rejecting unknown supplier=%q on line %d", supplier, total)
			skipped++
			continue
		}

		if *dryRun {
			fmt.Printf("[dry-run] oem=%s supplier=%s partNumber=%s price=%.2f %s\n",
				clean, supplier, rec.PartNumber, rec.PriceLocal, rec.PriceCurrency)
			continue
		}

		if err := upsert(ctx, pg, clean, supplier, rec); err != nil {
			errored++
			log.Printf("[regional_import] upsert err oem=%s supplier=%s: %v", clean, supplier, err)
			continue
		}
		inserted++
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scanner: %v", err)
	}

	fmt.Printf("total=%d inserted=%d skipped=%d errored=%d dry_run=%v\n",
		total, inserted, skipped, errored, *dryRun)
}

func upsert(ctx context.Context, pg *sql.DB, cleanOEM, supplier string, rec regionalRecord) error {
	const q = `
		INSERT INTO aftermarket_regional
			(oem_normalized, supplier, brand, part_number, description, stock_status, region, url, price_local, price_currency, updated_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (oem_normalized, supplier, part_number) DO UPDATE SET
			brand           = EXCLUDED.brand,
			description     = EXCLUDED.description,
			stock_status    = EXCLUDED.stock_status,
			region          = EXCLUDED.region,
			url             = EXCLUDED.url,
			price_local     = EXCLUDED.price_local,
			price_currency  = EXCLUDED.price_currency,
			updated_at      = NOW()`

	brandCanonical := ""
	if rec.Brand != "" {
		brandCanonical = service.NormalizeBrand(rec.Brand)
	}

	_, err := pg.ExecContext(ctx, q,
		cleanOEM, supplier, brandCanonical, rec.PartNumber,
		rec.Description, rec.StockStatus, rec.Region, rec.URL,
		nullFloat(rec.PriceLocal), nullString(rec.PriceCurrency),
	)
	return err
}

func nullFloat(f float64) any {
	if f == 0 {
		return nil
	}
	return f
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// rockauto_import - upserts RockAuto scraper NDJSON into aftermarket_rockauto.
//
// M4.S1.T2 stub. The actual scraper (M4.S1.T1) is external because it
// requires Playwright + anti-bot mitigation the CI env can't do
// natively. This binary is the "receiver" side: reads newline-delimited
// JSON on stdin, upserts each record into aftermarket_rockauto with
// ON CONFLICT (oem_normalized, brand, part_number) DO UPDATE SET
// scraped_at = EXCLUDED.scraped_at, ...
//
// Input record shape:
//
//	{
//	  "oem": "26350-2J001",
//	  "brand": "Bosch",
//	  "partNumber": "P7146",
//	  "description": "Engine Oil Filter",
//	  "category": "Oil Filter",
//	  "priceUsdCents": 895,
//	  "sourceUrl": "https://rockauto.com/en/..."
//	}
//
// Usage:
//
//	cat rockauto-scrape-2026-08-25.ndjson | ./rockauto_import
//
// Also supports --dry-run to preview upserts without writing.
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

type rockautoRecord struct {
	OEM           string `json:"oem"`
	Brand         string `json:"brand"`
	PartNumber    string `json:"partNumber"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	PriceUsdCents int    `json:"priceUsdCents"`
	SourceURL     string `json:"sourceUrl"`
}

func main() {
	dryRun := flag.Bool("dry-run", false, "log rows without writing to DB")
	flag.Parse()

	cfg := config.Load()

	pg := db.NewPostgres(cfg)
	if pg == nil {
		log.Fatalf("postgres: connection failed")
	}
	defer pg.Close()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 65536), 1024*1024) // 1MB per line max

	var (
		total, inserted, skipped, errored int
	)
	ctx := context.Background()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		total++

		var rec rockautoRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			errored++
			log.Printf("[rockauto_import] parse err line %d: %v", total, err)
			continue
		}

		clean := service.NormalizeOEM(rec.OEM)
		brand := service.NormalizeBrand(rec.Brand)
		if clean == "" || brand == "" || rec.PartNumber == "" {
			skipped++
			continue
		}

		if *dryRun {
			fmt.Printf("[dry-run] oem=%s brand=%s partNumber=%s price=%d\n",
				clean, brand, rec.PartNumber, rec.PriceUsdCents)
			continue
		}

		if err := upsert(ctx, pg, clean, brand, rec); err != nil {
			errored++
			log.Printf("[rockauto_import] upsert err oem=%s brand=%s: %v", clean, brand, err)
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

func upsert(ctx context.Context, pg *sql.DB, cleanOEM, canonicalBrand string, rec rockautoRecord) error {
	const q = `
		INSERT INTO aftermarket_rockauto
			(oem_normalized, brand, part_number, description, category, price_usd_cents, source_url, scraped_at)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (oem_normalized, brand, part_number) DO UPDATE SET
			description     = EXCLUDED.description,
			category        = EXCLUDED.category,
			price_usd_cents = EXCLUDED.price_usd_cents,
			source_url      = EXCLUDED.source_url,
			scraped_at      = EXCLUDED.scraped_at`
	_, err := pg.ExecContext(ctx, q,
		cleanOEM, canonicalBrand, rec.PartNumber,
		rec.Description, rec.Category, rec.PriceUsdCents, rec.SourceURL)
	return err
}

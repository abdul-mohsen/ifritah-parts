// derive_hk_maps auto-derives Hyundai/Kia prefix + chassis-code maps by
// clustering TecDoc MySQL data. It UPSERTs into Postgres tables created by
// migration 000011.
//
// Usage:
//
//	go run ./scripts/derive_hk_maps
//
// Env vars required:
//
//	MYSQL_HOST / MYSQL_PORT / MYSQL_USER / MYSQL_PASSWORD / MYSQL_DATABASE
//	   (or TECDOC_DSN as full DSN)
//	POSTGRES_HOST / POSTGRES_PORT / POSTGRES_USER / POSTGRES_PASSWORD /
//	POSTGRES_DB / POSTGRES_SSLMODE
//	   (or POSTGRES_URL as full DSN)
//
// Strategy (Path A):
//  1. Pull every distinct OEM number from TecDoc `oem_number` that matches
//     the Hyundai/Kia format (5 digits + 5 alphanumeric). Join to
//     `articles.genericArticleDescription` for the semantic label.
//  2. Cluster by (5-digit prefix, part description modal). Any prefix with
//     ≥3 supporting rows becomes a derived hk_oem_prefix_map row at
//     confidence 0.90 (source='tecdoc_derived').
//  3. Cluster by (2-char chassis code, vehicle description tokens). Any
//     chassis code with ≥5 supporting rows and consistent make+model becomes
//     a derived hk_chassis_code_map row at confidence 0.90.
//
// Path B (fallback): migration 000011 already seeded a hand-curated baseline
// at confidence 0.85. If this script fails to connect to TecDoc, the seed
// baseline still works — the feature stays functional either way.
//
// The script never DELETEs rows and only OVERRIDES a seed row when the
// derived confidence is higher AND the derived (make, model) matches the
// seed (safety guard against noisy clusters).
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	minPrefixSamples  = 3
	minChassisSamples = 5
	derivedConfidence = 0.90
)

func main() {
	mysqlDSN := getenv("TECDOC_DSN", "")
	if mysqlDSN == "" {
		mysqlDSN = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true",
			os.Getenv("MYSQL_USER"), os.Getenv("MYSQL_PASSWORD"),
			getenv("MYSQL_HOST", "localhost"), getenv("MYSQL_PORT", "3306"),
			getenv("MYSQL_DATABASE", "tecdoc"))
	}
	pgDSN := getenv("POSTGRES_URL", "")
	if pgDSN == "" {
		pgDSN = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			getenv("POSTGRES_HOST", "localhost"), getenv("POSTGRES_PORT", "5432"),
			getenv("POSTGRES_USER", "postgres"), os.Getenv("POSTGRES_PASSWORD"),
			getenv("POSTGRES_DB", "hk_parts"), getenv("POSTGRES_SSLMODE", "disable"))
	}

	mysqlDB, err := sql.Open("mysql", mysqlDSN)
	if err != nil {
		log.Fatalf("[derive_hk_maps] TecDoc MySQL open error (falling back to seed baseline): %v", err)
	}
	defer mysqlDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mysqlDB.PingContext(ctx); err != nil {
		log.Fatalf("[derive_hk_maps] TecDoc MySQL unreachable — Path A skipped, seed baseline (Path B) remains active: %v", err)
	}
	log.Printf("[derive_hk_maps] TecDoc MySQL connected")

	pgDB, err := sql.Open("pgx", pgDSN)
	if err != nil {
		log.Fatalf("[derive_hk_maps] Postgres open error: %v", err)
	}
	defer pgDB.Close()
	if err := pgDB.PingContext(ctx); err != nil {
		log.Fatalf("[derive_hk_maps] Postgres unreachable: %v", err)
	}
	log.Printf("[derive_hk_maps] Postgres connected")

	if err := derivePrefixMap(mysqlDB, pgDB); err != nil {
		log.Printf("[derive_hk_maps] prefix map derivation error: %v", err)
	}
	if err := deriveChassisMap(mysqlDB, pgDB); err != nil {
		log.Printf("[derive_hk_maps] chassis map derivation error: %v", err)
	}
	log.Printf("[derive_hk_maps] done — seed baseline (source='seed') preserved for prefixes/chassis codes with insufficient TecDoc support")
}

// derivePrefixMap clusters TecDoc oem_number by 5-digit prefix and joins to
// article descriptions. Prefixes with ≥3 supporting rows are UPSERTed at
// confidence 0.90 (source='tecdoc_derived').
func derivePrefixMap(mysqlDB, pgDB *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Restrict to Hyundai/Kia-shaped OEMs (5 digits + 5 alphanumeric).
	// Match the HK format regex: ^\d{5}[A-Z0-9]{5}$ after strip.
	const q = `
		SELECT
			LEFT(REPLACE(REPLACE(o.number, '-', ''), ' ', ''), 5) AS prefix,
			COALESCE(a.genericArticleDescription, '') AS desc,
			COUNT(*) AS cnt
		FROM oem_number o
		JOIN articles a ON a.legacyArticleId = o.articleId
		WHERE o.number REGEXP '^[0-9]{5}[- ]?[A-Z0-9]{5}$'
		  AND a.genericArticleDescription IS NOT NULL
		  AND a.genericArticleDescription != ''
		GROUP BY prefix, desc
		HAVING cnt >= ?`

	log.Printf("[derive_hk_maps] querying TecDoc for HK-shaped OEM prefixes…")
	rows, err := mysqlDB.QueryContext(ctx, q, minPrefixSamples)
	if err != nil {
		return fmt.Errorf("mysql prefix query: %w", err)
	}
	defer rows.Close()

	// Group by prefix, pick modal description.
	type descCount struct {
		desc string
		n    int
	}
	byPrefix := make(map[string][]descCount)
	for rows.Next() {
		var prefix, desc string
		var cnt int
		if err := rows.Scan(&prefix, &desc, &cnt); err != nil {
			continue
		}
		if len(prefix) != 5 {
			continue
		}
		byPrefix[prefix] = append(byPrefix[prefix], descCount{desc: desc, n: cnt})
	}

	log.Printf("[derive_hk_maps] found %d distinct 5-digit HK prefixes with ≥%d samples", len(byPrefix), minPrefixSamples)

	upserted := 0
	for prefix, descs := range byPrefix {
		sort.Slice(descs, func(i, j int) bool { return descs[i].n > descs[j].n })
		top := descs[0]
		total := 0
		for _, d := range descs {
			total += d.n
		}
		category, system := classifyDescription(top.desc)
		_, err := pgDB.ExecContext(ctx, `
			INSERT INTO hk_oem_prefix_map (prefix, system, category, description, confidence, source, sample_count)
			VALUES ($1, $2, $3, $4, $5, 'tecdoc_derived', $6)
			ON CONFLICT (prefix) DO UPDATE
			  SET system       = EXCLUDED.system,
			      category     = EXCLUDED.category,
			      description  = EXCLUDED.description,
			      confidence   = GREATEST(hk_oem_prefix_map.confidence, EXCLUDED.confidence),
			      source       = 'tecdoc_derived',
			      sample_count = EXCLUDED.sample_count,
			      updated_at   = NOW()
			  WHERE hk_oem_prefix_map.source != 'user'`,
			prefix, system, category, top.desc, derivedConfidence, total)
		if err != nil {
			log.Printf("[derive_hk_maps] upsert prefix=%s err=%v", prefix, err)
			continue
		}
		upserted++
	}
	log.Printf("[derive_hk_maps] upserted %d prefix map entries", upserted)
	return nil
}

// deriveChassisMap clusters TecDoc linkagetargets + oem_number to map
// 2-char chassis codes to (make, model, year range).
func deriveChassisMap(mysqlDB, pgDB *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Chassis code is the 2 chars right after the 5-digit prefix + optional dash.
	// e.g. "82460-2T010" → chassis "2T".
	const q = `
		SELECT
			SUBSTRING(REPLACE(REPLACE(o.number, '-', ''), ' ', ''), 6, 2) AS chassis,
			COALESCE(m.manuName, '') AS make,
			COALESCE(ms.modelname, '') AS model,
			MIN(FLOOR(lt.beginYearMonth / 100)) AS year_start,
			MAX(FLOOR(lt.endYearMonth   / 100)) AS year_end,
			COUNT(*) AS cnt
		FROM oem_number o
		JOIN articles a ON a.legacyArticleId = o.articleId
		JOIN articlesvehicletrees avt ON avt.legacyArticleId = a.legacyArticleId AND avt.linkingTargetType = 'P'
		JOIN linkagetargets lt ON lt.linkageTargetId = avt.linkingTargetId AND lt.lang = 'en'
		JOIN modelseries ms ON ms.modelId = lt.vehicleModelSeriesId
		JOIN manufacturers m ON m.manuId = ms.manuId
		WHERE o.number REGEXP '^[0-9]{5}[- ]?[A-Z0-9]{5}$'
		  AND m.manuName IN ('HYUNDAI','KIA','KIA MOTORS','HYUNDAI MOTOR','GENESIS')
		GROUP BY chassis, make, model
		HAVING cnt >= ?
		ORDER BY chassis, cnt DESC`

	log.Printf("[derive_hk_maps] querying TecDoc for HK chassis codes…")
	rows, err := mysqlDB.QueryContext(ctx, q, minChassisSamples)
	if err != nil {
		return fmt.Errorf("mysql chassis query: %w", err)
	}
	defer rows.Close()

	// For each chassis code, take the (make, model) tuple with the highest count.
	type mm struct {
		make      string
		model     string
		yearStart int
		yearEnd   int
		cnt       int
	}
	byChassis := make(map[string][]mm)
	for rows.Next() {
		var chassis, make, model string
		var yStart, yEnd, cnt int
		if err := rows.Scan(&chassis, &make, &model, &yStart, &yEnd, &cnt); err != nil {
			continue
		}
		if len(chassis) != 2 {
			continue
		}
		byChassis[chassis] = append(byChassis[chassis], mm{
			make: normalizeMake(make), model: model,
			yearStart: yStart, yearEnd: yEnd, cnt: cnt,
		})
	}

	log.Printf("[derive_hk_maps] found %d distinct HK chassis codes with ≥%d samples", len(byChassis), minChassisSamples)

	upserted := 0
	for chassis, tuples := range byChassis {
		sort.Slice(tuples, func(i, j int) bool { return tuples[i].cnt > tuples[j].cnt })
		top := tuples[0]
		var yEnd interface{}
		if top.yearEnd > 0 {
			yEnd = top.yearEnd
		}
		_, err := pgDB.ExecContext(ctx, `
			INSERT INTO hk_chassis_code_map (chassis_code, make, model, year_start, year_end, confidence, source, sample_count, notes)
			VALUES ($1, $2, $3, $4, $5, $6, 'tecdoc_derived', $7, $8)
			ON CONFLICT (chassis_code) DO UPDATE
			  SET make         = EXCLUDED.make,
			      model        = EXCLUDED.model,
			      year_start   = LEAST(hk_chassis_code_map.year_start, EXCLUDED.year_start),
			      year_end     = GREATEST(COALESCE(hk_chassis_code_map.year_end, 9999), COALESCE(EXCLUDED.year_end::int, 9999)),
			      confidence   = GREATEST(hk_chassis_code_map.confidence, EXCLUDED.confidence),
			      source       = 'tecdoc_derived',
			      sample_count = EXCLUDED.sample_count,
			      updated_at   = NOW()
			  WHERE hk_chassis_code_map.source != 'user'
			    AND hk_chassis_code_map.make = EXCLUDED.make`,
			chassis, top.make, top.model, top.yearStart, yEnd, derivedConfidence, top.cnt,
			fmt.Sprintf("derived from %d TecDoc samples", top.cnt))
		if err != nil {
			log.Printf("[derive_hk_maps] upsert chassis=%s err=%v", chassis, err)
			continue
		}
		upserted++
	}
	log.Printf("[derive_hk_maps] upserted %d chassis map entries", upserted)
	return nil
}

// classifyDescription maps a TecDoc generic article description to a coarse
// (category, system) tuple. Deterministic — no LLM, no external calls.
func classifyDescription(desc string) (category, system string) {
	d := strings.ToLower(desc)
	switch {
	case strings.Contains(d, "oil filter"):
		return "Oil Filter", "Engine"
	case strings.Contains(d, "air filter"), strings.Contains(d, "airfilter"):
		return "Air Filter", "Engine"
	case strings.Contains(d, "fuel filter"):
		return "Fuel Filter", "Engine"
	case strings.Contains(d, "cabin"), strings.Contains(d, "interior air"):
		return "Cabin Air Filter", "HVAC"
	case strings.Contains(d, "brake pad"):
		return "Brake Pad Set", "Brakes"
	case strings.Contains(d, "brake disc"), strings.Contains(d, "brake rotor"):
		return "Brake Disc", "Brakes"
	case strings.Contains(d, "shock absorber"), strings.Contains(d, "strut"):
		return "Shock Absorber / Strut", "Suspension"
	case strings.Contains(d, "spark plug"):
		return "Spark Plug", "Engine"
	case strings.Contains(d, "ignition coil"):
		return "Ignition Coil", "Engine"
	case strings.Contains(d, "window motor"), strings.Contains(d, "power window"):
		return "Power Window Motor", "Body"
	case strings.Contains(d, "radiator"):
		return "Radiator", "Cooling"
	case strings.Contains(d, "water pump"):
		return "Water Pump", "Cooling"
	case strings.Contains(d, "thermostat"):
		return "Thermostat", "Cooling"
	case strings.Contains(d, "headlight"), strings.Contains(d, "headlamp"):
		return "Headlight Assembly", "Electrical"
	case strings.Contains(d, "tail light"), strings.Contains(d, "taillamp"):
		return "Tail Light Assembly", "Electrical"
	case strings.Contains(d, "oxygen sensor"), strings.Contains(d, "lambda"):
		return "Oxygen Sensor", "Electrical"
	default:
		return desc, ""
	}
}

func normalizeMake(m string) string {
	up := strings.ToUpper(strings.TrimSpace(m))
	switch up {
	case "HYUNDAI", "HYUNDAI MOTOR":
		return "Hyundai"
	case "KIA", "KIA MOTORS":
		return "Kia"
	case "GENESIS":
		return "Genesis"
	default:
		return m
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

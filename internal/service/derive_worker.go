// Package service — DeriveWorker automates the Path A (TecDoc-driven)
// enrichment of hk_oem_prefix_map + hk_chassis_code_map.
//
// Replaces the manual `go run ./scripts/derive_hk_maps` step.
//
// Design:
//
//   - Runs as a background goroutine started from cmd/server/main.go
//     immediately after migrations complete.
//   - Uses a Postgres advisory lock so multiple app replicas (e.g.
//     docker-compose scale) don't race.
//   - Persists last-run timestamp in background_task_runs so we don't
//     re-derive on every container restart — default cadence is 30 days.
//   - Silently no-ops when TecDoc MySQL is unreachable (Path B seed
//     baseline from migration 000011 remains active).
//   - After the initial run, sets a monthly time.Ticker to keep the
//     derived data fresh as new TecDoc dumps land.
//
// Failure modes are logged, never fatal — the app never depends on the
// derived data (seed baseline is always available).
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

const (
	deriveTaskKey        = "derive_hk_maps"
	deriveLockKey  int64 = 7412_0801_0817_5678 // advisory lock — distinct from migrator's
	deriveMinInterval    = 30 * 24 * time.Hour // 30 days default cadence
	minPrefixSamples     = 3
	minChassisSamples    = 5
	derivedConfidence    = 0.90
)

// DeriveWorker owns the periodic derive task.
type DeriveWorker struct {
	pg    *sql.DB
	mysql *sql.DB
	// interval overrides deriveMinInterval when non-zero. Tests use small
	// values so they don't wait 30 days.
	interval time.Duration
	// force flag causes RunOnce to skip the last-run cadence check.
	force bool
}

func NewDeriveWorker(pg, mysql *sql.DB) *DeriveWorker {
	return &DeriveWorker{pg: pg, mysql: mysql, interval: deriveMinInterval}
}

// SetForce toggles the cadence-check bypass. When true, RunOnce ignores
// last_run_at and always performs the derive. Used by the CLI wrapper's
// --force flag.
func (w *DeriveWorker) SetForce(force bool) {
	if w == nil {
		return
	}
	w.force = force
}

// SetInterval overrides the default 30-day cadence. Primarily for tests.
func (w *DeriveWorker) SetInterval(d time.Duration) {
	if w == nil || d <= 0 {
		return
	}
	w.interval = d
}

// Start launches the worker as a background goroutine. It runs once
// immediately (subject to the last-run cadence check), then sleeps for
// w.interval and repeats. Cancel the returned context (or shut the
// process) to stop.
//
// Returns immediately — safe to call from main.go without blocking boot.
func (w *DeriveWorker) Start(ctx context.Context) {
	if w == nil || w.pg == nil {
		return // seed baseline stays; no-op
	}
	if w.mysql == nil {
		log.Printf("[derive_worker] TecDoc MySQL not configured — derive disabled (seed baseline from migration 000011 remains active)")
		return
	}
	go func() {
		// Small initial jitter so multi-replica deploys don't dogpile at
		// boot. The advisory lock would serialize them anyway, but this
		// avoids the noisy "waiting for lock" period at every start.
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		if err := w.RunOnce(ctx); err != nil {
			log.Printf("[derive_worker] initial run error: %v", err)
		}
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.RunOnce(ctx); err != nil {
					log.Printf("[derive_worker] scheduled run error: %v", err)
				}
			}
		}
	}()
}

// RunOnce performs a single derive pass with double-run protection:
//
//  1. Take advisory lock on w.pg.
//  2. Check background_task_runs.last_run_at; skip if within interval
//     (unless w.force).
//  3. Ping w.mysql; skip with 'skipped' status if unreachable.
//  4. Derive prefix + chassis maps; UPSERT into Postgres.
//  5. Record run in background_task_runs.
//  6. Release lock.
//
// Errors from steps 4-5 mark the run as 'error' but the worker keeps
// going on the next tick — a temporary TecDoc outage never stalls the app.
func (w *DeriveWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.pg == nil {
		return errors.New("derive_worker: pg is nil")
	}

	conn, err := w.pg.Conn(ctx)
	if err != nil {
		return fmt.Errorf("derive_worker: acquire connection: %w", err)
	}
	defer conn.Close()

	// pg_try_advisory_lock — non-blocking. If another replica is running
	// derive right now, we skip. Idempotent by design.
	var got bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", deriveLockKey).Scan(&got); err != nil {
		return fmt.Errorf("derive_worker: try_advisory_lock: %w", err)
	}
	if !got {
		log.Printf("[derive_worker] another instance holds the lock — skipping this run")
		return nil
	}
	defer func() {
		if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", deriveLockKey); err != nil {
			log.Printf("[derive_worker] WARN unlock: %v", err)
		}
	}()

	// Cadence check.
	if !w.force {
		var lastRun sql.NullTime
		err := conn.QueryRowContext(ctx,
			`SELECT last_run_at FROM background_task_runs WHERE task_key = $1`, deriveTaskKey).
			Scan(&lastRun)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("derive_worker: read last run: %w", err)
		}
		if lastRun.Valid && time.Since(lastRun.Time) < w.interval {
			log.Printf("[derive_worker] last run %v ago (< %v cadence) — skipping",
				time.Since(lastRun.Time).Round(time.Minute), w.interval)
			return nil
		}
	}

	// TecDoc reachability probe.
	probeCtx, probeCancel := context.WithTimeout(ctx, 3*time.Second)
	defer probeCancel()
	if err := w.mysql.PingContext(probeCtx); err != nil {
		w.recordRun(ctx, conn, "skipped", fmt.Sprintf("TecDoc MySQL unreachable: %v", err))
		log.Printf("[derive_worker] TecDoc MySQL unreachable — skipped: %v", err)
		return nil // not an error at the app level — Path B baseline remains active
	}

	start := time.Now()
	var (
		prefixesUpserted int
		chassisUpserted  int
		derr             error
	)
	prefixesUpserted, derr = w.derivePrefixMap(ctx)
	if derr != nil {
		w.recordRun(ctx, conn, "error", fmt.Sprintf("prefix derivation: %v", derr))
		return derr
	}
	chassisUpserted, derr = w.deriveChassisMap(ctx)
	if derr != nil {
		w.recordRun(ctx, conn, "error", fmt.Sprintf("chassis derivation: %v", derr))
		return derr
	}
	msg := fmt.Sprintf("upserted prefixes=%d chassis=%d in %v", prefixesUpserted, chassisUpserted, time.Since(start).Round(time.Millisecond))
	w.recordRun(ctx, conn, "success", msg)
	log.Printf("[derive_worker] %s", msg)
	return nil
}

func (w *DeriveWorker) recordRun(ctx context.Context, conn *sql.Conn, status, message string) {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO background_task_runs (task_key, last_run_at, last_status, last_message, next_run_at)
		VALUES ($1, NOW(), $2, $3, NOW() + $4::interval)
		ON CONFLICT (task_key) DO UPDATE
		SET last_run_at   = NOW(),
		    last_status   = EXCLUDED.last_status,
		    last_message  = EXCLUDED.last_message,
		    next_run_at   = NOW() + $4::interval`,
		deriveTaskKey, status, message, fmt.Sprintf("%d seconds", int(w.interval.Seconds())))
	if err != nil {
		log.Printf("[derive_worker] WARN record run: %v", err)
	}
}

// derivePrefixMap — see the standalone script at scripts/derive_hk_maps/main.go
// for the full comments; this is the same query lifted into the worker.
func (w *DeriveWorker) derivePrefixMap(ctx context.Context) (int, error) {
	qctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	const q = `
		SELECT
			LEFT(REPLACE(REPLACE(o.number, '-', ''), ' ', ''), 5) AS prefix,
			COALESCE(a.genericArticleDescription, '') AS desc_text,
			COUNT(*) AS cnt
		FROM oem_number o
		JOIN articles a ON a.legacyArticleId = o.articleId
		WHERE o.number REGEXP '^[0-9]{5}[- ]?[A-Z0-9]{5}$'
		  AND a.genericArticleDescription IS NOT NULL
		  AND a.genericArticleDescription != ''
		GROUP BY prefix, desc_text
		HAVING cnt >= ?`

	rows, err := w.mysql.QueryContext(qctx, q, minPrefixSamples)
	if err != nil {
		return 0, fmt.Errorf("mysql prefix query: %w", err)
	}
	defer rows.Close()

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
	log.Printf("[derive_worker] prefix pass: %d distinct 5-digit HK prefixes with ≥%d samples", len(byPrefix), minPrefixSamples)

	upserted := 0
	for prefix, descs := range byPrefix {
		sort.Slice(descs, func(i, j int) bool { return descs[i].n > descs[j].n })
		top := descs[0]
		total := 0
		for _, d := range descs {
			total += d.n
		}
		category, system := classifyDeriveDescription(top.desc)
		if _, err := w.pg.ExecContext(qctx, `
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
			prefix, system, category, top.desc, derivedConfidence, total); err != nil {
			log.Printf("[derive_worker] upsert prefix=%s err=%v", prefix, err)
			continue
		}
		upserted++
	}
	return upserted, nil
}

// deriveChassisMap — parallels derivePrefixMap.
func (w *DeriveWorker) deriveChassisMap(ctx context.Context) (int, error) {
	qctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	const q = `
		SELECT
			SUBSTRING(REPLACE(REPLACE(o.number, '-', ''), ' ', ''), 6, 2) AS chassis,
			COALESCE(m.manuName, '') AS make_name,
			COALESCE(ms.modelname, '') AS model_name,
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
		GROUP BY chassis, make_name, model_name
		HAVING cnt >= ?
		ORDER BY chassis, cnt DESC`

	rows, err := w.mysql.QueryContext(qctx, q, minChassisSamples)
	if err != nil {
		return 0, fmt.Errorf("mysql chassis query: %w", err)
	}
	defer rows.Close()

	type mm struct {
		make      string
		model     string
		yearStart int
		yearEnd   int
		cnt       int
	}
	byChassis := make(map[string][]mm)
	for rows.Next() {
		var chassis, mk, model string
		var yStart, yEnd, cnt int
		if err := rows.Scan(&chassis, &mk, &model, &yStart, &yEnd, &cnt); err != nil {
			continue
		}
		if len(chassis) != 2 {
			continue
		}
		byChassis[chassis] = append(byChassis[chassis], mm{
			make: normalizeDeriveMake(mk), model: model,
			yearStart: yStart, yearEnd: yEnd, cnt: cnt,
		})
	}
	log.Printf("[derive_worker] chassis pass: %d distinct HK chassis codes with ≥%d samples", len(byChassis), minChassisSamples)

	upserted := 0
	for chassis, tuples := range byChassis {
		sort.Slice(tuples, func(i, j int) bool { return tuples[i].cnt > tuples[j].cnt })
		top := tuples[0]
		var yEnd interface{}
		if top.yearEnd > 0 {
			yEnd = top.yearEnd
		}
		if _, err := w.pg.ExecContext(qctx, `
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
			fmt.Sprintf("derived from %d TecDoc samples", top.cnt)); err != nil {
			log.Printf("[derive_worker] upsert chassis=%s err=%v", chassis, err)
			continue
		}
		upserted++
	}
	return upserted, nil
}

// classifyDeriveDescription maps a TecDoc generic-article description to
// (category, system). Deterministic — no LLM, no external calls.
//
// Ordering matters: "Cabin Air Filter" must be caught by the cabin/HVAC
// branch BEFORE the generic "air filter" → engine branch, otherwise cabin
// filters get misclassified as engine parts.
func classifyDeriveDescription(desc string) (category, system string) {
	d := strings.ToLower(desc)
	switch {
	case strings.Contains(d, "cabin"), strings.Contains(d, "interior air"):
		return "Cabin Air Filter", "HVAC"
	case strings.Contains(d, "oil filter"):
		return "Oil Filter", "Engine"
	case strings.Contains(d, "air filter"), strings.Contains(d, "airfilter"):
		return "Air Filter", "Engine"
	case strings.Contains(d, "fuel filter"):
		return "Fuel Filter", "Engine"
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

func normalizeDeriveMake(m string) string {
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

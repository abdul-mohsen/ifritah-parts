package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// M6.S2.T2 — Per-request cost meter.
//
// A CostMeter is a small, lock-free (atomic-only) accumulator attached to
// a request's context.Context. Every hot path — DB query, external HTTP
// call, cache hit — records into the meter as work happens. On response
// the meter is merged into the process-wide DefaultAggregate, which the
// GET /api/debug/cost handler surfaces as JSON.
//
// The rates below are PLACEHOLDER numbers — real cost tuning is a
// separate task once the wiring is proven. What matters for now is
// that we have a shape:
//   * per-request granularity (each request produces one CostSnapshot)
//   * decomposed by cost driver (DB vs external, count vs bytes)
//   * process-lifetime aggregate for at-a-glance p95 monitoring
//   * pluggable rate card (CostRates) so ops can retune without a rebuild
//
// See docs/sprints/M5-M6-intelligence-and-production.md §M6.S2.T2 for
// the intended Grafana wiring (out of scope for this PR).

// costMeterKey is the context.Value key for the per-request CostMeter.
// Unexported to prevent accidental collision with third-party keys.
type costMeterKey struct{}

// CostRatesConfig is the process-wide cost model. All fields are USD.
// Copy-by-value; treat any GetCostRates() result as an immutable snapshot.
type CostRatesConfig struct {
	// PerDBQuery is charged once per DB query, regardless of size.
	PerDBQuery float64
	// PerDBKB is charged per KB of DB response bytes read.
	PerDBKB float64
	// PerExternalCall is charged once per outbound HTTP call (eBay,
	// PartsOuq, dealer scrapes, G5 sources, etc.).
	PerExternalCall float64
	// PerExternalKB is charged per KB of external response bytes.
	PerExternalKB float64
	// PerCacheHit is charged per cache hit (default 0 — free).
	PerCacheHit float64
	// PerSlowQuery is an ADDITIONAL charge applied when a DB query
	// takes >1s. Callers record slow queries via RecordDBQuerySlow AND
	// still call RecordDBQuery — the slow cost is on top.
	PerSlowQuery float64
}

// defaultCostRates are the placeholder rates. Tuning is a separate task
// (see M6.S2.T2 sprint spec) — these are approximations to get a shape
// and let alerting develop against real traffic.
//
//	DB query:     $0.0001  (1/100 cent per query)
//	DB bytes:     $0.000001 per KB
//	External:     $0.001   (~10× DB query — external HTTP calls are expensive)
//	External KB:  $0.00001 per KB
//	Cache hit:    $0        (free)
//	Slow query:   $0.001    (extra when >1s — slow queries are 10× normal)
var defaultCostRates = CostRatesConfig{
	PerDBQuery:      0.0001,
	PerDBKB:         0.000001,
	PerExternalCall: 0.001,
	PerExternalKB:   0.00001,
	PerCacheHit:     0.0,
	PerSlowQuery:    0.001,
}

// costRates + costRatesMu form the process-wide read-mostly rate card.
// Reads acquire an RLock (cheap when uncontended); writes acquire a Lock
// and replace the whole struct.
var (
	costRatesMu sync.RWMutex
	costRates   = defaultCostRates
)

// GetCostRates returns a copy of the current process-wide cost model.
// Safe to call from anywhere.
func GetCostRates() CostRatesConfig {
	costRatesMu.RLock()
	defer costRatesMu.RUnlock()
	return costRates
}

// SetCostRates replaces the process-wide cost model. Used in tests and
// when a deploy-specific pricing sheet is loaded at boot.
func SetCostRates(r CostRatesConfig) {
	costRatesMu.Lock()
	costRates = r
	costRatesMu.Unlock()
}

// ResetCostRates restores the built-in defaults. Test-only convenience.
func ResetCostRates() {
	SetCostRates(defaultCostRates)
}

// CostMeter tracks the approximate USD cost of servicing a single request.
// All Record* methods are safe on a nil receiver — they no-op — so callers
// can freely dereference an unwired context without nil-checks:
//
//	service.CostMeterFromContext(ctx).RecordDBQuery(len(bytes))
//
// works whether or not the caller wired a meter into ctx.
type CostMeter struct {
	dbQueries     atomic.Int64 // count of DB queries
	dbBytes       atomic.Int64 // approx bytes read from DB
	externalCalls atomic.Int64 // count of external HTTP calls
	externalBytes atomic.Int64 // approx bytes received from external calls
	cacheHits     atomic.Int64 // count of cache hits (free)
	slowQueries   atomic.Int64 // count of DB queries > 1s (extra cost)

	start time.Time // set at construction; never mutated (race-safe read)
}

// NewCostMeter allocates a fresh per-request meter anchored at now.
func NewCostMeter() *CostMeter {
	return &CostMeter{start: time.Now()}
}

// CostMeterFromContext extracts the CostMeter attached to ctx. Returns
// nil when none present; nil is safe to Record*() against.
func CostMeterFromContext(ctx context.Context) *CostMeter {
	if ctx == nil {
		return nil
	}
	if m, ok := ctx.Value(costMeterKey{}).(*CostMeter); ok {
		return m
	}
	return nil
}

// WithCostMeter returns a derived context carrying the given meter.
// Passing a nil ctx returns nil (mirrors context.WithValue's contract).
func WithCostMeter(ctx context.Context, m *CostMeter) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, costMeterKey{}, m)
}

// RecordDBQuery records one DB query event and its approximate response
// size in bytes. Pass bytes=0 when the size is unknown (COUNT queries,
// scalar lookups, etc.) — count-based cost still applies.
func (m *CostMeter) RecordDBQuery(bytes int) {
	if m == nil {
		return
	}
	m.dbQueries.Add(1)
	if bytes > 0 {
		m.dbBytes.Add(int64(bytes))
	}
}

// RecordDBQuerySlow records one slow DB query (elapsed > 1s). This is
// ADDITIVE — callers should invoke both RecordDBQuery (for the base
// count/bytes cost) AND RecordDBQuerySlow (for the slow-query surcharge).
func (m *CostMeter) RecordDBQuerySlow() {
	if m == nil {
		return
	}
	m.slowQueries.Add(1)
}

// RecordExternal records one external HTTP call with an approximate
// response size in bytes. Pass bytes=0 when the size is unknown.
func (m *CostMeter) RecordExternal(bytes int) {
	if m == nil {
		return
	}
	m.externalCalls.Add(1)
	if bytes > 0 {
		m.externalBytes.Add(int64(bytes))
	}
}

// RecordCacheHit records a cache hit (free by default; configurable via
// CostRates.PerCacheHit).
func (m *CostMeter) RecordCacheHit() {
	if m == nil {
		return
	}
	m.cacheHits.Add(1)
}

// CostSnapshot is an immutable point-in-time view of a CostMeter's
// counters combined with the current CostRates to produce a total cost
// estimate. Emitted from CostMeter.Snapshot and consumed by
// AggregateMeter.Merge and the /api/debug/cost endpoint.
type CostSnapshot struct {
	DBQueries     int64 `json:"dbQueries"`
	DBBytes       int64 `json:"dbBytes"`
	ExternalCalls int64 `json:"externalCalls"`
	ExternalBytes int64 `json:"externalBytes"`
	CacheHits     int64 `json:"cacheHits"`
	SlowQueries   int64 `json:"slowQueries"`

	ElapsedMs    int64   `json:"elapsedMs"`
	CostUsd      float64 `json:"costUsd"`
	CostUsdCents float64 `json:"costUsdCents"`
}

// Snapshot captures the meter's counters plus the current rates into an
// immutable CostSnapshot. Safe on a nil receiver — returns the zero value.
func (m *CostMeter) Snapshot() CostSnapshot {
	if m == nil {
		return CostSnapshot{}
	}
	dbQ := m.dbQueries.Load()
	dbB := m.dbBytes.Load()
	exC := m.externalCalls.Load()
	exB := m.externalBytes.Load()
	ch := m.cacheHits.Load()
	sQ := m.slowQueries.Load()

	rates := GetCostRates()
	cost := computeCost(dbQ, dbB, exC, exB, ch, sQ, rates)

	elapsed := time.Since(m.start)
	return CostSnapshot{
		DBQueries:     dbQ,
		DBBytes:       dbB,
		ExternalCalls: exC,
		ExternalBytes: exB,
		CacheHits:     ch,
		SlowQueries:   sQ,
		ElapsedMs:     elapsed.Milliseconds(),
		CostUsd:       cost,
		CostUsdCents:  cost * 100,
	}
}

// computeCost is the shared cost formula used by both CostSnapshot and
// AggregateSnapshot. Extracted so the two paths cannot drift.
func computeCost(dbQ, dbB, exC, exB, ch, sQ int64, r CostRatesConfig) float64 {
	return float64(dbQ)*r.PerDBQuery +
		float64(dbB)/1024.0*r.PerDBKB +
		float64(exC)*r.PerExternalCall +
		float64(exB)/1024.0*r.PerExternalKB +
		float64(ch)*r.PerCacheHit +
		float64(sQ)*r.PerSlowQuery
}

// -------------------- Aggregate (process lifetime) ---------------------

// AggregateMeter accumulates per-request CostSnapshots into
// process-lifetime totals. Read via /api/debug/cost.
//
// processStart is set at construction and only re-anchored by Reset()
// (test/admin path). Reset is documented as not-safe-to-run-concurrently
// with a live Snapshot; production code never calls Reset.
type AggregateMeter struct {
	requestsServed atomic.Int64
	dbQueries      atomic.Int64
	dbBytes        atomic.Int64
	externalCalls  atomic.Int64
	externalBytes  atomic.Int64
	cacheHits      atomic.Int64
	slowQueries    atomic.Int64

	mu           sync.Mutex // guards processStart against Reset() races
	processStart time.Time
}

// NewAggregateMeter allocates a fresh accumulator anchored at now.
func NewAggregateMeter() *AggregateMeter {
	return &AggregateMeter{processStart: time.Now()}
}

// Merge adds a request-scoped CostSnapshot into the aggregate. Safe on
// nil (no-op). Called from SearchWithProgress' deferred cleanup.
func (a *AggregateMeter) Merge(s CostSnapshot) {
	if a == nil {
		return
	}
	a.requestsServed.Add(1)
	a.dbQueries.Add(s.DBQueries)
	a.dbBytes.Add(s.DBBytes)
	a.externalCalls.Add(s.ExternalCalls)
	a.externalBytes.Add(s.ExternalBytes)
	a.cacheHits.Add(s.CacheHits)
	a.slowQueries.Add(s.SlowQueries)
}

// AggregateSnapshot is the JSON payload returned by /api/debug/cost.
type AggregateSnapshot struct {
	RequestsServed            int64           `json:"requestsServed"`
	TotalDBQueries            int64           `json:"totalDbQueries"`
	TotalDBBytes              int64           `json:"totalDbBytes"`
	TotalExternalCalls        int64           `json:"totalExternalCalls"`
	TotalExternalBytes        int64           `json:"totalExternalBytes"`
	TotalCacheHits            int64           `json:"totalCacheHits"`
	TotalSlowQueries          int64           `json:"totalSlowQueries"`
	TotalCostUsd              float64         `json:"totalCostUsd"`
	TotalCostUsdCents         float64         `json:"totalCostUsdCents"`
	AvgCostUsdCentsPerRequest float64         `json:"avgCostUsdCentsPerRequest"`
	SinceProcessStart         string          `json:"sinceProcessStart"`
	SinceProcessStartMs       int64           `json:"sinceProcessStartMs"`
	Rates                     CostRatesConfig `json:"rates"`
}

// Snapshot returns the current aggregate view with cost computed against
// the current CostRates. Safe to call concurrently with Merge.
func (a *AggregateMeter) Snapshot() AggregateSnapshot {
	if a == nil {
		return AggregateSnapshot{}
	}
	req := a.requestsServed.Load()
	dbQ := a.dbQueries.Load()
	dbB := a.dbBytes.Load()
	exC := a.externalCalls.Load()
	exB := a.externalBytes.Load()
	ch := a.cacheHits.Load()
	sQ := a.slowQueries.Load()

	rates := GetCostRates()
	total := computeCost(dbQ, dbB, exC, exB, ch, sQ, rates)

	a.mu.Lock()
	elapsed := time.Since(a.processStart)
	a.mu.Unlock()

	avgCents := 0.0
	if req > 0 {
		avgCents = (total * 100) / float64(req)
	}
	return AggregateSnapshot{
		RequestsServed:            req,
		TotalDBQueries:            dbQ,
		TotalDBBytes:              dbB,
		TotalExternalCalls:        exC,
		TotalExternalBytes:        exB,
		TotalCacheHits:            ch,
		TotalSlowQueries:          sQ,
		TotalCostUsd:              total,
		TotalCostUsdCents:         total * 100,
		AvgCostUsdCentsPerRequest: avgCents,
		SinceProcessStart:         elapsed.Truncate(time.Second).String(),
		SinceProcessStartMs:       elapsed.Milliseconds(),
		Rates:                     rates,
	}
}

// Reset zeroes all counters and re-anchors processStart to now.
// Convenience for tests + admin operations. NOT safe to run concurrently
// with in-flight Snapshot/Merge callers — do not wire into production.
func (a *AggregateMeter) Reset() {
	if a == nil {
		return
	}
	a.requestsServed.Store(0)
	a.dbQueries.Store(0)
	a.dbBytes.Store(0)
	a.externalCalls.Store(0)
	a.externalBytes.Store(0)
	a.cacheHits.Store(0)
	a.slowQueries.Store(0)
	a.mu.Lock()
	a.processStart = time.Now()
	a.mu.Unlock()
}

// DefaultAggregate is the process-wide aggregate meter merged into by
// SearchWithProgress on request completion. Consumed by the /api/debug/cost
// endpoint. Exported so tests + admin code can inspect / reset it.
var DefaultAggregate = NewAggregateMeter()

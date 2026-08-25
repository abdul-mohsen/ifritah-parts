package service

import (
	"context"
	"sync"
	"sync/atomic"
)

// costMeterKey is the context.Value key for the per-request CostMeter.
// Unexported to prevent accidental collision with third-party keys.
type costMeterKey struct{}

// CostMeter tracks the approximate USD cost of servicing a single request.
// Increment via AddDBQuery / AddExternalAPI / AddCacheHit as work happens;
// the middleware layer reads Total at response time and emits it as an
// X-Request-Cost-Usd header + a log line.
//
// M6.S2.T2. Numbers are approximate — real-world costs come from the
// managed DB pricing sheet + external API contracts. Purpose is
// per-request visibility, not accounting precision.
type CostMeter struct {
	DBQueries   atomic.Int64 // count
	ExternalAPI atomic.Int64
	CacheHits   atomic.Int64
	SlowQueries atomic.Int64 // queries > 1s

	// Optional overrides (fall back to defaults when zero)
	perDBQuery  float64
	perExternal float64
	perCache    float64
	perSlow     float64

	// Guard for perDBQuery + friends when set concurrently
	// (init is typically single-threaded but keep it safe).
	mu sync.Mutex
}

// Cost defaults - approximate for a managed Postgres + Anthropic /
// TecDoc-external-API price point. Adjust per deploy env if precision
// matters.
const (
	defaultDBQueryCost   = 0.0001 // 1/100 cent per query
	defaultExternalCost  = 0.002  // ~200x DB query
	defaultCacheHitCost  = 0.0    // free
	defaultSlowQueryCost = 0.001  // slow queries cost 10x normal
)

func NewCostMeter() *CostMeter {
	return &CostMeter{}
}

// FromContext extracts the CostMeter attached to ctx. Returns a no-op
// meter (nil) when none present so callers don't need to nil-check.
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
func WithCostMeter(ctx context.Context, m *CostMeter) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, costMeterKey{}, m)
}

// AddDBQuery records one DB query event. Slow=true if the query took > 1s.
func (m *CostMeter) AddDBQuery(slow bool) {
	if m == nil {
		return
	}
	m.DBQueries.Add(1)
	if slow {
		m.SlowQueries.Add(1)
	}
}

// AddExternalAPI records one external HTTP call.
func (m *CostMeter) AddExternalAPI() {
	if m == nil {
		return
	}
	m.ExternalAPI.Add(1)
}

// AddCacheHit records a cache hit (free by default).
func (m *CostMeter) AddCacheHit() {
	if m == nil {
		return
	}
	m.CacheHits.Add(1)
}

// TotalUSD returns the accumulated cost in USD. Uses per-event defaults
// unless SetRates was called.
func (m *CostMeter) TotalUSD() float64 {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	pdb := m.perDBQuery
	pex := m.perExternal
	pca := m.perCache
	psl := m.perSlow
	m.mu.Unlock()
	if pdb == 0 {
		pdb = defaultDBQueryCost
	}
	if pex == 0 {
		pex = defaultExternalCost
	}
	if pca == 0 {
		pca = defaultCacheHitCost
	}
	if psl == 0 {
		psl = defaultSlowQueryCost
	}
	return float64(m.DBQueries.Load())*pdb +
		float64(m.ExternalAPI.Load())*pex +
		float64(m.CacheHits.Load())*pca +
		float64(m.SlowQueries.Load())*psl
}

// SetRates overrides the per-event cost defaults. Used by tests + when
// deploy-specific pricing sheets kick in.
func (m *CostMeter) SetRates(perDBQuery, perExternal, perCache, perSlow float64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.perDBQuery = perDBQuery
	m.perExternal = perExternal
	m.perCache = perCache
	m.perSlow = perSlow
	m.mu.Unlock()
}

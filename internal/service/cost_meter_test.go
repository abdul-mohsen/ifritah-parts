package service

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"
)

// TestCostMeter_EmptyMeter_ZeroSnapshot verifies a freshly-allocated meter
// has all-zero counters and zero cost.
func TestCostMeter_EmptyMeter_ZeroSnapshot(t *testing.T) {
	t.Cleanup(ResetCostRates)
	ResetCostRates()

	m := NewCostMeter()
	s := m.Snapshot()

	if s.DBQueries != 0 || s.DBBytes != 0 {
		t.Errorf("empty meter dbQueries=%d dbBytes=%d, want 0/0", s.DBQueries, s.DBBytes)
	}
	if s.ExternalCalls != 0 || s.ExternalBytes != 0 {
		t.Errorf("empty meter external calls=%d bytes=%d, want 0/0", s.ExternalCalls, s.ExternalBytes)
	}
	if s.CacheHits != 0 || s.SlowQueries != 0 {
		t.Errorf("empty meter cacheHits=%d slowQueries=%d, want 0/0", s.CacheHits, s.SlowQueries)
	}
	if s.CostUsd != 0 || s.CostUsdCents != 0 {
		t.Errorf("empty meter costUsd=%v cents=%v, want 0/0", s.CostUsd, s.CostUsdCents)
	}
}

// TestCostMeter_RecordCounts_TableDriven exercises the three Record*
// entry points and verifies the resulting counter + cost via a table.
func TestCostMeter_RecordCounts_TableDriven(t *testing.T) {
	t.Cleanup(ResetCostRates)
	ResetCostRates()

	tests := []struct {
		name          string
		dbQueries     int
		dbBytesEach   int
		externalCalls int
		externalBytes int
		cacheHits     int
		slowQueries   int
		wantCostUsd   float64
	}{
		{
			name:        "10 DB queries no bytes → count-only cost",
			dbQueries:   10,
			wantCostUsd: 10 * 0.0001, // 0.001
		},
		{
			name:          "5 external calls no bytes → count-only cost",
			externalCalls: 5,
			wantCostUsd:   5 * 0.001, // 0.005
		},
		{
			name:        "DB query with 4KB payload → count + bytes cost",
			dbQueries:   1,
			dbBytesEach: 4096,
			wantCostUsd: 0.0001 + (4096.0/1024.0)*0.000001, // 0.0001 + 0.000004
		},
		{
			name:          "external call with 2KB payload → count + bytes cost",
			externalCalls: 1,
			externalBytes: 2048,
			wantCostUsd:   0.001 + (2048.0/1024.0)*0.00001, // 0.001 + 0.00002
		},
		{
			name:        "cache hits are free at default rates",
			cacheHits:   1000,
			wantCostUsd: 0,
		},
		{
			name:        "slow query surcharge stacks on regular",
			dbQueries:   1,
			slowQueries: 1,
			wantCostUsd: 0.0001 + 0.001,
		},
		{
			name:          "mixed workload — real-shape request",
			dbQueries:     20,
			dbBytesEach:   1024,
			externalCalls: 2,
			externalBytes: 8192,
			cacheHits:     3,
			slowQueries:   1,
			wantCostUsd: 20*0.0001 + (20*1024.0/1024.0)*0.000001 +
				2*0.001 + (8192.0/1024.0)*0.00001 +
				0 + 1*0.001,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewCostMeter()
			for i := 0; i < tc.dbQueries; i++ {
				m.RecordDBQuery(tc.dbBytesEach)
			}
			for i := 0; i < tc.externalCalls; i++ {
				// Distribute externalBytes evenly across calls.
				perCall := 0
				if tc.externalCalls > 0 {
					perCall = tc.externalBytes / tc.externalCalls
				}
				m.RecordExternal(perCall)
			}
			for i := 0; i < tc.cacheHits; i++ {
				m.RecordCacheHit()
			}
			for i := 0; i < tc.slowQueries; i++ {
				m.RecordDBQuerySlow()
			}
			s := m.Snapshot()
			if int(s.DBQueries) != tc.dbQueries {
				t.Errorf("dbQueries=%d want %d", s.DBQueries, tc.dbQueries)
			}
			if int(s.ExternalCalls) != tc.externalCalls {
				t.Errorf("externalCalls=%d want %d", s.ExternalCalls, tc.externalCalls)
			}
			if int(s.CacheHits) != tc.cacheHits {
				t.Errorf("cacheHits=%d want %d", s.CacheHits, tc.cacheHits)
			}
			if int(s.SlowQueries) != tc.slowQueries {
				t.Errorf("slowQueries=%d want %d", s.SlowQueries, tc.slowQueries)
			}
			if !floatNear(s.CostUsd, tc.wantCostUsd, 1e-9) {
				t.Errorf("costUsd=%v want %v", s.CostUsd, tc.wantCostUsd)
			}
			// Cents mirror of costUsd for the same tolerance × 100.
			if !floatNear(s.CostUsdCents, tc.wantCostUsd*100, 1e-7) {
				t.Errorf("costUsdCents=%v want %v", s.CostUsdCents, tc.wantCostUsd*100)
			}
		})
	}
}

// TestCostMeter_Concurrent_NoDataRace fans out N goroutines each
// recording M events and asserts the final counts are exact. Combined
// with `go test -race`, this validates our atomic.Int64 record paths.
func TestCostMeter_Concurrent_NoDataRace(t *testing.T) {
	t.Cleanup(ResetCostRates)
	ResetCostRates()

	const goroutines = 8
	const perGoroutine = 1000

	m := NewCostMeter()
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				m.RecordDBQuery(128)
				m.RecordExternal(64)
				m.RecordCacheHit()
				if i%10 == 0 {
					m.RecordDBQuerySlow()
				}
			}
		}()
	}
	wg.Wait()

	s := m.Snapshot()
	wantDBQueries := int64(goroutines * perGoroutine)
	wantDBBytes := int64(goroutines * perGoroutine * 128)
	wantExternalCalls := int64(goroutines * perGoroutine)
	wantExternalBytes := int64(goroutines * perGoroutine * 64)
	wantCacheHits := int64(goroutines * perGoroutine)
	wantSlowQueries := int64(goroutines * perGoroutine / 10)

	if s.DBQueries != wantDBQueries {
		t.Errorf("concurrent dbQueries=%d want %d", s.DBQueries, wantDBQueries)
	}
	if s.DBBytes != wantDBBytes {
		t.Errorf("concurrent dbBytes=%d want %d", s.DBBytes, wantDBBytes)
	}
	if s.ExternalCalls != wantExternalCalls {
		t.Errorf("concurrent externalCalls=%d want %d", s.ExternalCalls, wantExternalCalls)
	}
	if s.ExternalBytes != wantExternalBytes {
		t.Errorf("concurrent externalBytes=%d want %d", s.ExternalBytes, wantExternalBytes)
	}
	if s.CacheHits != wantCacheHits {
		t.Errorf("concurrent cacheHits=%d want %d", s.CacheHits, wantCacheHits)
	}
	if s.SlowQueries != wantSlowQueries {
		t.Errorf("concurrent slowQueries=%d want %d", s.SlowQueries, wantSlowQueries)
	}
}

// TestCostMeter_NilSafe_NoPanic — nil meter operations must not panic.
// This is what lets callers do CostMeterFromContext(ctx).Record*() without
// nil-checks.
func TestCostMeter_NilSafe_NoPanic(t *testing.T) {
	var m *CostMeter
	// None of these should panic.
	m.RecordDBQuery(1024)
	m.RecordDBQuerySlow()
	m.RecordExternal(2048)
	m.RecordCacheHit()

	s := m.Snapshot()
	if s.DBQueries != 0 || s.CostUsd != 0 {
		t.Errorf("nil meter snapshot non-zero: %+v", s)
	}
}

// TestCostMeter_Context_RoundTrip — Meter attaches to ctx via
// WithCostMeter and comes back out via CostMeterFromContext.
func TestCostMeter_Context_RoundTrip(t *testing.T) {
	m := NewCostMeter()
	m.RecordExternal(0)

	ctx := WithCostMeter(context.Background(), m)
	got := CostMeterFromContext(ctx)
	if got != m {
		t.Errorf("round-trip returned different meter (%p vs %p)", got, m)
	}

	// Recording via the retrieved handle mutates the original.
	got.RecordDBQuery(0)
	if s := m.Snapshot(); s.DBQueries != 1 {
		t.Errorf("retrieved handle didn't share state, got %d, want 1", s.DBQueries)
	}
}

// TestCostMeter_Context_EmptyOrNil — missing meter returns nil (which
// is safe to Record* against).
func TestCostMeter_Context_EmptyOrNil(t *testing.T) {
	if CostMeterFromContext(context.Background()) != nil {
		t.Errorf("empty ctx returned non-nil meter")
	}
	if CostMeterFromContext(nil) != nil {
		t.Errorf("nil ctx returned non-nil meter")
	}
	// A ctx with a WithCostMeter(nil, ...) is programmer error — the
	// derived ctx is nil and behaves like nil.
	if WithCostMeter(nil, NewCostMeter()) != nil {
		t.Errorf("WithCostMeter(nil, m) should return nil (mirrors context.WithValue contract)")
	}
}

// TestCostMeter_RateCard_ReflectsInCost — SetCostRates changes the
// per-Snapshot cost computation for all future Snapshot() calls.
func TestCostMeter_RateCard_ReflectsInCost(t *testing.T) {
	t.Cleanup(ResetCostRates)

	m := NewCostMeter()
	m.RecordDBQuery(1024)  // 1 query, 1024 bytes
	m.RecordExternal(2048) // 1 call, 2048 bytes

	// Default rates:
	// 1*0.0001 + (1024/1024)*0.000001 + 1*0.001 + (2048/1024)*0.00001
	// = 0.0001 + 0.000001 + 0.001 + 0.00002 = 0.001121
	ResetCostRates()
	baseCost := m.Snapshot().CostUsd
	wantBase := 0.0001 + 0.000001 + 0.001 + 0.00002
	if !floatNear(baseCost, wantBase, 1e-9) {
		t.Errorf("default-rate cost=%v want %v", baseCost, wantBase)
	}

	// Override rates: 100× everything.
	SetCostRates(CostRatesConfig{
		PerDBQuery:      0.01,
		PerDBKB:         0.0001,
		PerExternalCall: 0.1,
		PerExternalKB:   0.001,
		PerCacheHit:     0,
		PerSlowQuery:    0,
	})
	// 1*0.01 + 1*0.0001 + 1*0.1 + 2*0.001 = 0.1121
	newCost := m.Snapshot().CostUsd
	wantNew := 0.01 + 0.0001 + 0.1 + 0.002
	if !floatNear(newCost, wantNew, 1e-9) {
		t.Errorf("overridden-rate cost=%v want %v", newCost, wantNew)
	}
	if newCost <= baseCost {
		t.Errorf("expected 100x rate hike to increase cost: base=%v new=%v", baseCost, newCost)
	}
}

// TestCostMeter_Snapshot_ElapsedIsPositive — the elapsed field measures
// wall-clock time since NewCostMeter.
func TestCostMeter_Snapshot_ElapsedIsPositive(t *testing.T) {
	m := NewCostMeter()
	time.Sleep(2 * time.Millisecond)
	s := m.Snapshot()
	if s.ElapsedMs < 1 {
		t.Errorf("elapsedMs=%d, expected >= 1", s.ElapsedMs)
	}
}

// TestAggregateMeter_Merge_Sums — Merge accumulates snapshots and cost.
func TestAggregateMeter_Merge_Sums(t *testing.T) {
	t.Cleanup(ResetCostRates)
	ResetCostRates()

	agg := NewAggregateMeter()

	// Request 1: 5 DB queries, no external.
	m1 := NewCostMeter()
	for i := 0; i < 5; i++ {
		m1.RecordDBQuery(512)
	}
	agg.Merge(m1.Snapshot())

	// Request 2: 3 DB queries + 2 external calls + 1 slow.
	m2 := NewCostMeter()
	for i := 0; i < 3; i++ {
		m2.RecordDBQuery(1024)
	}
	m2.RecordExternal(4096)
	m2.RecordExternal(4096)
	m2.RecordDBQuerySlow()
	agg.Merge(m2.Snapshot())

	s := agg.Snapshot()

	if s.RequestsServed != 2 {
		t.Errorf("requestsServed=%d want 2", s.RequestsServed)
	}
	if s.TotalDBQueries != 8 {
		t.Errorf("totalDbQueries=%d want 8", s.TotalDBQueries)
	}
	if s.TotalExternalCalls != 2 {
		t.Errorf("totalExternalCalls=%d want 2", s.TotalExternalCalls)
	}
	if s.TotalSlowQueries != 1 {
		t.Errorf("totalSlowQueries=%d want 1", s.TotalSlowQueries)
	}
	if s.TotalCostUsd <= 0 {
		t.Errorf("totalCostUsd=%v want > 0", s.TotalCostUsd)
	}
	// avgCostUsdCentsPerRequest is (total cents) / requestsServed.
	wantAvg := s.TotalCostUsdCents / float64(s.RequestsServed)
	if !floatNear(s.AvgCostUsdCentsPerRequest, wantAvg, 1e-9) {
		t.Errorf("avgCostUsdCentsPerRequest=%v want %v", s.AvgCostUsdCentsPerRequest, wantAvg)
	}
}

// TestAggregateMeter_Snapshot_ZeroRequestsSafe — avoids div-by-zero when
// no requests have been served yet (freshly-booted process).
func TestAggregateMeter_Snapshot_ZeroRequestsSafe(t *testing.T) {
	t.Cleanup(ResetCostRates)
	ResetCostRates()

	agg := NewAggregateMeter()
	s := agg.Snapshot()
	if s.RequestsServed != 0 {
		t.Errorf("requestsServed=%d want 0", s.RequestsServed)
	}
	if s.AvgCostUsdCentsPerRequest != 0 {
		t.Errorf("avgCostUsdCentsPerRequest=%v want 0 (no requests yet)", s.AvgCostUsdCentsPerRequest)
	}
	if math.IsNaN(s.AvgCostUsdCentsPerRequest) || math.IsInf(s.AvgCostUsdCentsPerRequest, 0) {
		t.Errorf("avg should not be NaN/Inf, got %v", s.AvgCostUsdCentsPerRequest)
	}
	if s.SinceProcessStartMs < 0 {
		t.Errorf("sinceProcessStartMs=%d want >= 0", s.SinceProcessStartMs)
	}
}

// TestAggregateMeter_Concurrent_MergeSafe — many goroutines merging
// snapshots concurrently must produce exact totals.
func TestAggregateMeter_Concurrent_MergeSafe(t *testing.T) {
	t.Cleanup(ResetCostRates)
	ResetCostRates()

	agg := NewAggregateMeter()
	const goroutines = 8
	const perGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				m := NewCostMeter()
				m.RecordDBQuery(0)
				m.RecordExternal(0)
				agg.Merge(m.Snapshot())
			}
		}()
	}
	wg.Wait()

	s := agg.Snapshot()
	if s.RequestsServed != int64(goroutines*perGoroutine) {
		t.Errorf("requestsServed=%d want %d", s.RequestsServed, goroutines*perGoroutine)
	}
	if s.TotalDBQueries != int64(goroutines*perGoroutine) {
		t.Errorf("totalDbQueries=%d want %d", s.TotalDBQueries, goroutines*perGoroutine)
	}
	if s.TotalExternalCalls != int64(goroutines*perGoroutine) {
		t.Errorf("totalExternalCalls=%d want %d", s.TotalExternalCalls, goroutines*perGoroutine)
	}
}

// TestAggregateMeter_Reset_ZeroesCounters — Reset() zeros the counters
// and re-anchors processStart.
func TestAggregateMeter_Reset_ZeroesCounters(t *testing.T) {
	t.Cleanup(ResetCostRates)
	ResetCostRates()

	agg := NewAggregateMeter()
	m := NewCostMeter()
	m.RecordDBQuery(1024)
	agg.Merge(m.Snapshot())

	if agg.Snapshot().RequestsServed != 1 {
		t.Fatal("pre-reset: expected 1 request served")
	}

	agg.Reset()

	s := agg.Snapshot()
	if s.RequestsServed != 0 {
		t.Errorf("post-reset requestsServed=%d want 0", s.RequestsServed)
	}
	if s.TotalDBQueries != 0 {
		t.Errorf("post-reset totalDbQueries=%d want 0", s.TotalDBQueries)
	}
	if s.TotalCostUsd != 0 {
		t.Errorf("post-reset totalCostUsd=%v want 0", s.TotalCostUsd)
	}
}

// TestGetSetCostRates_RoundTrip — SetCostRates + GetCostRates roundtrip.
func TestGetSetCostRates_RoundTrip(t *testing.T) {
	t.Cleanup(ResetCostRates)

	custom := CostRatesConfig{
		PerDBQuery:      0.5,
		PerDBKB:         0.6,
		PerExternalCall: 0.7,
		PerExternalKB:   0.8,
		PerCacheHit:     0.9,
		PerSlowQuery:    1.0,
	}
	SetCostRates(custom)
	got := GetCostRates()
	if got != custom {
		t.Errorf("get after set: got=%+v want=%+v", got, custom)
	}

	ResetCostRates()
	got = GetCostRates()
	if got.PerDBQuery != 0.0001 { // default
		t.Errorf("ResetCostRates didn't restore defaults: %+v", got)
	}
}

// floatNear is a small helper matching two floats within a tolerance.
func floatNear(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}

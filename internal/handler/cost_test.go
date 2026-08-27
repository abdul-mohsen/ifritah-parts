package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// TestCostHandler_Aggregate_ReturnsSnapshot verifies the /api/debug/cost
// endpoint returns a JSON snapshot with the expected top-level fields.
// Resets service.DefaultAggregate so this test is independent of test-run
// ordering (other tests may or may not have merged into the aggregate).
func TestCostHandler_Aggregate_ReturnsSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Reset to zero so we know exactly what to expect. Reset() is
	// documented as unsafe-under-concurrent-load but this test is
	// single-threaded.
	service.DefaultAggregate.Reset()

	// Merge a synthetic per-request snapshot so the aggregate has
	// non-zero counters.
	m := service.NewCostMeter()
	m.RecordDBQuery(2048)
	m.RecordDBQuery(1024)
	m.RecordExternal(4096)
	service.DefaultAggregate.Merge(m.Snapshot())

	r := gin.New()
	h := NewCostHandler()
	r.GET("/api/debug/cost", h.Aggregate)

	req := httptest.NewRequest(http.MethodGet, "/api/debug/cost", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v — %s", err, rec.Body.String())
	}

	// Contract fields — every one must be present + non-nil.
	wantKeys := []string{
		"requestsServed",
		"totalDbQueries",
		"totalDbBytes",
		"totalExternalCalls",
		"totalExternalBytes",
		"totalCacheHits",
		"totalSlowQueries",
		"totalCostUsd",
		"totalCostUsdCents",
		"avgCostUsdCentsPerRequest",
		"sinceProcessStart",
		"sinceProcessStartMs",
		"rates",
	}
	for _, k := range wantKeys {
		if _, ok := body[k]; !ok {
			t.Errorf("missing field %q in body: %s", k, rec.Body.String())
		}
	}

	// Sanity: the one request we merged is reflected.
	if got := body["requestsServed"]; got != float64(1) {
		t.Errorf("requestsServed=%v want 1", got)
	}
	if got := body["totalDbQueries"]; got != float64(2) {
		t.Errorf("totalDbQueries=%v want 2", got)
	}
	if got := body["totalExternalCalls"]; got != float64(1) {
		t.Errorf("totalExternalCalls=%v want 1", got)
	}

	// rates must be a nested object with the placeholder-cost fields.
	rates, ok := body["rates"].(map[string]any)
	if !ok {
		t.Fatalf("rates not a map: %T", body["rates"])
	}
	for _, k := range []string{"PerDBQuery", "PerDBKB", "PerExternalCall", "PerExternalKB", "PerCacheHit", "PerSlowQuery"} {
		if _, ok := rates[k]; !ok {
			t.Errorf("rates missing field %q: %v", k, rates)
		}
	}
}

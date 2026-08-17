package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// makeTestSearchHandler wires a SearchHandler with an unwired SmartSearch.
// Since the input-validation tests don't hit the DB path (they short-circuit
// on the validation guard), this is enough coverage for the handler-level checks.
func makeTestSearchHandler() *SearchHandler {
	ss := service.NewSmartSearch(nil, nil, nil, nil, nil, nil, false)
	return NewSearchHandler(ss)
}

func newTestRouter(h *SearchHandler) *gin.Engine {
	r := gin.New()
	r.GET("/api/search", h.Search)
	r.GET("/api/search/modes", h.Modes)
	return r
}

// TestSearch_BadRequest_NoQueryNoVehicleNoCategory verifies the 400 gate when
// the caller supplies nothing to search on.
func TestSearch_BadRequest_NoQueryNoVehicleNoCategory(t *testing.T) {
	h := makeTestSearchHandler()
	r := newTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty search, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "provide") {
		t.Errorf("expected error message to mention required params, got %s", w.Body.String())
	}
}

// TestSearch_BadRequest_UnknownMode verifies the plan requirement: an unknown
// `?mode=` returns 400 rather than silently falling through to the legacy cascade.
func TestSearch_BadRequest_UnknownMode(t *testing.T) {
	h := makeTestSearchHandler()
	r := newTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=test&mode=not_a_real_mode", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown mode, got %d body=%s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if body["error"] != "unknown mode" {
		t.Errorf("expected error='unknown mode', got %v", body["error"])
	}
	if body["mode"] != "not_a_real_mode" {
		t.Errorf("response should echo back the invalid mode, got %v", body["mode"])
	}
	if _, ok := body["validModes"].([]interface{}); !ok {
		t.Errorf("response should list validModes, got %v", body["validModes"])
	}
}

// TestSearch_BadRequest_UnknownEnrichmentLevel verifies the same guard for
// enrichmentLevel.
func TestSearch_BadRequest_UnknownEnrichmentLevel(t *testing.T) {
	h := makeTestSearchHandler()
	r := newTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=test&enrichmentLevel=extreme", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown enrichmentLevel, got %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if body["error"] != "unknown enrichmentLevel" {
		t.Errorf("expected error='unknown enrichmentLevel', got %v", body["error"])
	}
}

// TestSearch_AcceptsKnownEnrichmentLevels verifies all valid enrichment levels
// pass the validation gate (they may still fail downstream when there's no DB
// wired, but that's a 500 not a 400).
func TestSearch_AcceptsKnownEnrichmentLevels(t *testing.T) {
	h := makeTestSearchHandler()
	r := newTestRouter(h)
	for _, lvl := range []string{"", "none", "basic", "full"} {
		u := "/api/search?q=test"
		if lvl != "" {
			u += "&enrichmentLevel=" + lvl
		}
		req := httptest.NewRequest(http.MethodGet, u, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		// We expect NOT a 400 — anything else is fine (500 is acceptable
		// because the DB isn't wired in the test rig).
		if w.Code == http.StatusBadRequest {
			t.Errorf("enrichmentLevel=%q rejected as 400 (should be accepted); body=%s", lvl, w.Body.String())
		}
	}
}

// TestSearch_EmptyModeIsAllowed verifies that empty `?mode=` (or unspecified)
// falls through to the legacy cascade rather than 400ing.
func TestSearch_EmptyModeIsAllowed(t *testing.T) {
	h := makeTestSearchHandler()
	r := newTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusBadRequest {
		t.Errorf("empty mode should be allowed (legacy cascade); got 400 body=%s", w.Body.String())
	}
}

// TestModes_ReturnsBaseSet verifies /api/search/modes returns the base set
// of modes even when TecDoc is not wired.
func TestModes_ReturnsBaseSet(t *testing.T) {
	h := makeTestSearchHandler()
	r := newTestRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/search/modes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Modes []struct {
			Key         string `json:"key"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"modes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	seen := map[string]bool{}
	for _, m := range body.Modes {
		if m.Key == "" || m.Name == "" || m.Description == "" {
			t.Errorf("mode has empty field: %+v", m)
		}
		seen[m.Key] = true
	}
	for _, want := range []string{"exact_oem", "cross_reference", "vehicle_fitment", "supersession", "cross_brand", "owned_catalog", "keyword_gated", "combined"} {
		if !seen[want] {
			t.Errorf("/api/search/modes missing base mode %q", want)
		}
	}
	// spec-based modes must NOT be present without tecDocSpecs
	for _, forbidden := range []string{"spec_match", "assembly_context", "vin_assembly"} {
		if seen[forbidden] {
			t.Errorf("/api/search/modes exposes %q without tecDocSpecs — guard violated", forbidden)
		}
	}
}

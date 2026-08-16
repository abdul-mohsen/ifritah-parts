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

// newNilDBSearch returns a SearchHandler whose SmartSearch has a nil DB.
// The SmartSearch itself is non-nil (avoids nil receiver panic), but every
// search / categories / crossref operation returns "database not connected"
// → HTTP 500.
//
// CrossRef is also initialised with nil DB (non-nil struct, nil queries),
// so GetOEMNumbers and GetVehiclesForArticle return errors without panic.
func newNilDBSearch() *SearchHandler {
	cr := service.NewCrossRef(nil, false)
	ss := service.NewSmartSearch(nil, nil, cr, nil, nil, nil, false)
	return NewSearchHandler(ss)
}

// buildSearchRouter registers all three SearchHandler routes on a fresh router.
func buildSearchRouter(h *SearchHandler) *gin.Engine {
	r := gin.New()
	r.GET("/api/search", h.Search)
	r.GET("/api/vehicle/:id/categories", h.Categories)
	r.GET("/api/part/:id/crossref", h.CrossRef)
	return r
}

// assertErrorJSON verifies that the response body is valid JSON containing
// an "error" key. Called as a helper so failures report the caller's line.
func assertErrorJSON(t *testing.T, body string) {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("response is not valid JSON: %s", body)
	}
	if _, ok := m["error"]; !ok {
		t.Errorf("JSON response missing 'error' key: %s", body)
	}
}

// TestSearchHandler_MissingParams_Returns400 verifies that the handler
// rejects requests where none of the three search triggers (q, linkageTargetId,
// category) is non-zero, and that supplying any one of them routes the request
// onward to the search engine (→ 500 with nil DB).
func TestSearchHandler_MissingParams_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		url      string
		wantCode int
	}{
		// ── All three triggers absent → 400 ─────────────────────────────────
		{
			"no params at all",
			"/api/search",
			http.StatusBadRequest,
		},
		{
			"explicit q empty string",
			"/api/search?q=",
			http.StatusBadRequest,
		},
		{
			"q empty and linkageTargetId=0",
			"/api/search?q=&linkageTargetId=0",
			http.StatusBadRequest,
		},
		{
			"q empty and category empty",
			"/api/search?q=&category=",
			http.StatusBadRequest,
		},
		{
			"all three triggers explicitly empty",
			"/api/search?q=&linkageTargetId=0&category=",
			http.StatusBadRequest,
		},
		{
			"linkageTargetId=0 only",
			"/api/search?linkageTargetId=0",
			http.StatusBadRequest,
		},
		{
			"category empty string only",
			"/api/search?category=",
			http.StatusBadRequest,
		},
		{
			"linkageTargetId=0 and category empty",
			"/api/search?linkageTargetId=0&category=",
			http.StatusBadRequest,
		},
		{
			"q empty linkageTargetId empty category empty",
			"/api/search?q=&linkageTargetId=&category=",
			http.StatusBadRequest,
		},
		{
			"only fuelType provided (not a trigger)",
			"/api/search?fuelType=petrol",
			http.StatusBadRequest,
		},
		{
			"only vehicleCC provided (not a trigger)",
			"/api/search?vehicleCC=2000",
			http.StatusBadRequest,
		},
		{
			"only page provided (not a trigger)",
			"/api/search?page=2",
			http.StatusBadRequest,
		},
		{
			"only limit provided (not a trigger)",
			"/api/search?limit=10",
			http.StatusBadRequest,
		},
		// ── Any one trigger present → search executes → nil DB → 500 ────────
		{
			"q=oil+filter triggers search",
			"/api/search?q=oil+filter",
			http.StatusInternalServerError,
		},
		{
			"q=OEM number triggers search",
			"/api/search?q=26300-35505",
			http.StatusInternalServerError,
		},
		{
			"linkageTargetId=10001 triggers search",
			"/api/search?linkageTargetId=10001",
			http.StatusInternalServerError,
		},
		{
			"category=filter triggers search",
			"/api/search?category=filter",
			http.StatusInternalServerError,
		},
		{
			"q and category both present triggers search",
			"/api/search?q=oil&category=engine",
			http.StatusInternalServerError,
		},
	}

	router := buildSearchRouter(newNilDBSearch())

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d (body: %s)",
					rec.Code, tc.wantCode, rec.Body.String())
			}
			assertErrorJSON(t, rec.Body.String())
		})
	}
}

// TestSearchHandler_InvalidVehicleID_Returns400 checks that non-positive or
// non-numeric values for the :id path parameter in
// GET /api/vehicle/:id/categories produce HTTP 400, while a valid positive
// integer (with nil DB) produces HTTP 500.
func TestSearchHandler_InvalidVehicleID_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		path     string
		wantCode int
	}{
		// ── Invalid IDs → 400 ────────────────────────────────────────────────
		{"non-numeric id abc", "/api/vehicle/abc/categories", http.StatusBadRequest},
		{"zero id", "/api/vehicle/0/categories", http.StatusBadRequest},
		{"negative id -1", "/api/vehicle/-1/categories", http.StatusBadRequest},
		{"negative id -100", "/api/vehicle/-100/categories", http.StatusBadRequest},
		{"float-like 1.5", "/api/vehicle/1.5/categories", http.StatusBadRequest},
		{"hex-like 0x1A", "/api/vehicle/0x1A/categories", http.StatusBadRequest},
		{"letters only xyz", "/api/vehicle/xyz/categories", http.StatusBadRequest},
		{"mixed alphanum ab12", "/api/vehicle/ab12/categories", http.StatusBadRequest},
		{"special char 1!", "/api/vehicle/1!/categories", http.StatusBadRequest},
		{"max-negative -2147483648", "/api/vehicle/-2147483648/categories", http.StatusBadRequest},
		// ── Valid positive ID → nil DB → 500 ─────────────────────────────────
		{"valid id 10001 nil DB", "/api/vehicle/10001/categories", http.StatusInternalServerError},
		{"valid id 1 nil DB", "/api/vehicle/1/categories", http.StatusInternalServerError},
	}

	router := buildSearchRouter(newNilDBSearch())

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d (body: %s)",
					rec.Code, tc.wantCode, rec.Body.String())
			}
			// Both 400 and 500 from this handler use {"error": "..."}
			assertErrorJSON(t, rec.Body.String())
		})
	}
}

// TestSearchHandler_InvalidPartID_Returns400 checks that non-positive or
// non-numeric values for the :id path parameter in
// GET /api/part/:id/crossref produce HTTP 400, while a valid positive
// integer (with nil DB) produces HTTP 500.
//
// An empty path segment (/api/part//crossref or /api/part/crossref) is not
// routed to the handler at all — gin returns 404 or a redirect.
func TestSearchHandler_InvalidPartID_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		path     string
		wantCode int
	}{
		// ── Invalid IDs → 400 ────────────────────────────────────────────────
		{"non-numeric id abc", "/api/part/abc/crossref", http.StatusBadRequest},
		{"zero id", "/api/part/0/crossref", http.StatusBadRequest},
		{"negative id -1", "/api/part/-1/crossref", http.StatusBadRequest},
		{"negative id -999", "/api/part/-999/crossref", http.StatusBadRequest},
		{"float-like 3.14", "/api/part/3.14/crossref", http.StatusBadRequest},
		{"letters only xyz", "/api/part/xyz/crossref", http.StatusBadRequest},
		{"mixed alphanum ab42", "/api/part/ab42/crossref", http.StatusBadRequest},
		{"large negative -2147483648", "/api/part/-2147483648/crossref", http.StatusBadRequest},
		// ── Valid positive ID → nil DB → both OEM + vehicle queries error → 500
		{"valid part id 100001 nil DB", "/api/part/100001/crossref", http.StatusInternalServerError},
		{"valid part id 1 nil DB", "/api/part/1/crossref", http.StatusInternalServerError},
		{"valid part id 999999 nil DB", "/api/part/999999/crossref", http.StatusInternalServerError},
	}

	router := buildSearchRouter(newNilDBSearch())

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d (body: %s)",
					rec.Code, tc.wantCode, rec.Body.String())
			}
			assertErrorJSON(t, rec.Body.String())
		})
	}
}

// TestSearchHandler_NilSearch_Returns500 verifies that any valid search request
// to a SearchHandler backed by a nil-DB SmartSearch returns HTTP 500 with a
// JSON body containing both an "error" key and the message "database not connected".
//
// Uses newNilDBSearch() which builds SmartSearch(db=nil) — the SmartSearch
// struct is non-nil so method dispatch succeeds; Search() short-circuits at
// `if s.db == nil` and returns the "database not connected" error.
func TestSearchHandler_NilSearch_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		url  string
	}{
		{"free-text query oil filter", "/api/search?q=oil+filter"},
		{"OEM number query 26300-35505", "/api/search?q=26300-35505"},
		{"OEM number query 97133-D3000", "/api/search?q=97133-D3000"},
		{"article number query PH6811", "/api/search?q=PH6811"},
		{"linkageTargetId=10001 no q", "/api/search?linkageTargetId=10001"},
		{"category=engine+oil+filter", "/api/search?category=engine+oil+filter"},
		{"q and linkage together", "/api/search?q=brake+pad&linkageTargetId=10001"},
	}

	router := buildSearchRouter(newNilDBSearch())

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, "database not connected") {
				t.Errorf("body should contain 'database not connected'; got: %s", body)
			}
			assertErrorJSON(t, body)
		})
	}
}

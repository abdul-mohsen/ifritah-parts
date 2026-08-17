package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// buildOEMRouter registers the OEM lookup route on a fresh router.
func buildOEMRouter(h *OEMHandler) *gin.Engine {
	r := gin.New()
	r.GET("/api/oem/:number", h.Lookup)
	return r
}

// newNilDBOEM returns an OEMHandler whose OEMLookup has a nil DB.
// The OEMLookup struct is non-nil; Search() checks s.queries == nil and
// returns "database not connected" without panic.
func newNilDBOEM() *OEMHandler {
	oem := service.NewOEMLookup(nil)
	return NewOEMHandler(oem)
}

// TestOEMHandler_EmptyNumber_Returns400 verifies that the empty-number guard
// in OEMHandler.Lookup returns 400.
//
// Because gin's router requires a non-empty path segment for :number, the only
// reliable way to trigger `number == ""` is to inject the param directly via
// gin.CreateTestContext. The test also confirms that an empty-segment URL is
// not routed to the handler, and that a non-empty number bypasses the 400 guard.
func TestOEMHandler_EmptyNumber_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newNilDBOEM()

	// Sub-test 1: inject empty param directly — should return 400.
	t.Run("injected empty param returns 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		// Attach a minimal request so c.DefaultQuery calls do not panic.
		c.Request = httptest.NewRequest(http.MethodGet, "/api/oem/", nil)
		c.Params = gin.Params{gin.Param{Key: "number", Value: ""}}

		h.Lookup(c)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("empty param: status = %d, want 400 (body: %s)",
				rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "OEM number required") {
			t.Errorf("body should contain 'OEM number required'; got: %s", rec.Body.String())
		}
	})

	// Sub-test 2: trailing-slash URL (/api/oem/) is not routed to the handler
	// — gin returns 404 or 301. The 400 "OEM number required" message must NOT
	// appear, proving the handler was not invoked with an empty number.
	t.Run("trailing slash not routed to handler", func(t *testing.T) {
		r := buildOEMRouter(h)
		req := httptest.NewRequest(http.MethodGet, "/api/oem/", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if strings.Contains(rec.Body.String(), "OEM number required") {
			t.Errorf("handler was called with empty param; body: %s", rec.Body.String())
		}
		// Acceptable responses: 404 (not found) or 301/308 (redirect) — but never 400.
		if rec.Code == http.StatusBadRequest {
			t.Errorf("router returned 400 for empty-segment URL, want 404 or redirect")
		}
	})

	// Sub-test 3: a non-empty number bypasses the empty guard and reaches the
	// DB call — with nil DB the handler returns 500, not 400.
	t.Run("non-empty number bypasses 400 guard nil DB returns 500", func(t *testing.T) {
		r := buildOEMRouter(h)
		req := httptest.NewRequest(http.MethodGet, "/api/oem/X", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code == http.StatusBadRequest {
			t.Errorf("non-empty number should NOT return 400 (empty guard); got 400 body: %s",
				rec.Body.String())
		}
	})
}

// TestOEMHandler_NilDB_Returns500 verifies that OEM number lookups against
// an OEMLookup with a nil DB return HTTP 500 with a JSON error body.
//
// service.NewOEMLookup(nil) creates a non-nil OEMLookup whose queries field
// is nil. OEMLookup.Search() checks s.queries == nil → returns
// "database not connected" error → handler returns 500.
func TestOEMHandler_NilDB_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newNilDBOEM()
	r := buildOEMRouter(h)

	// Real OEM numbers from the seed database and live API.
	cases := []struct {
		name   string
		number string
	}{
		{"canonical oil filter OEM", "26300-35505"},
		{"cabin air filter OEM", "97133-D3000"},
		{"brake pad OEM", "58101-D3A70"},
		{"shock absorber OEM", "54651-D3000"},
		{"alternator OEM", "37300-2B100"},
		{"a/c compressor OEM", "97701-D3000"},
		{"aftermarket-style but non-empty W-811-80", "W-811-80"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				"/api/oem/"+tc.number, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Errorf("OEM %q: status = %d, want 500 (body: %s)",
					tc.number, rec.Code, rec.Body.String())
			}
			assertErrorJSON(t, rec.Body.String())
			if !strings.Contains(rec.Body.String(), "database not connected") {
				t.Errorf("OEM %q: body should contain 'database not connected'; got: %s",
					tc.number, rec.Body.String())
			}
		})
	}
}

// TestOEMHandler_SettersNoopOnNilDeps verifies that:
//  1. NewOEMHandler(nil)   does not panic (construction is safe with nil oem).
//  2. SetCrossRef(nil)     does not panic.
//  3. SetPartsLookup(nil)  does not panic.
//
// None of these should call any DB methods — they only assign struct fields.
func TestOEMHandler_SettersNoopOnNilDeps(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. Construct with nil oem — must not panic.
	h := NewOEMHandler(nil)
	if h == nil {
		t.Fatal("NewOEMHandler(nil) returned nil handler")
	}

	// 2. SetCrossRef(nil) — must not panic.
	h.SetCrossRef(nil)

	// 3. SetPartsLookup(nil) — must not panic.
	h.SetPartsLookup(nil)

	// Also verify setters work on a properly constructed handler.
	oem := service.NewOEMLookup(nil)
	h2 := NewOEMHandler(oem)
	h2.SetCrossRef(nil)
	h2.SetPartsLookup(nil)
}

// TestOEMHandler_ResponseContainsOEMFields verifies that an OEM lookup response
// body is valid JSON containing an "error" key when the DB is not connected.
// The test confirms the handler does not panic and the response is machine-readable.
func TestOEMHandler_ResponseContainsOEMFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newNilDBOEM()
	r := buildOEMRouter(h)

	t.Run("error response is valid JSON with error key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/oem/26300-35505", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
		// Must be valid JSON with "error" key — no panic, no empty body.
		assertErrorJSON(t, rec.Body.String())
	})
}

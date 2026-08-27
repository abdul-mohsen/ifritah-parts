package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// fakeFeedbackStore is the in-memory FeedbackStore used by every handler
// test in this file. It records every Insert call so the tests can
// assert (a) the PII columns are SHA256-shaped, (b) the values match
// what the handler saw. Concurrency-safe because the rate-limit test
// fires 60+ requests via the gin router which is single-threaded per
// call but the store may be accessed from multiple goroutines during a
// t.Parallel() future rearrangement.
type fakeFeedbackStore struct {
	mu       sync.Mutex
	inserts  []service.FeedbackEvent
	weekly   []service.WeeklyBucket
	disputed []service.DisputedOEM
	// insertErr, if set, is returned by every Insert call. Used to
	// exercise the DB-error branch of the handler.
	insertErr error
	// weeklyErr / disputedErr play the same role for the aggregate
	// endpoints.
	weeklyErr   error
	disputedErr error
	// nextID is the auto-increment id returned by Insert.
	nextID int64
}

func newFakeStore() *fakeFeedbackStore {
	return &fakeFeedbackStore{}
}

func (f *fakeFeedbackStore) Insert(_ context.Context, ev service.FeedbackEvent) (int64, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return 0, time.Time{}, f.insertErr
	}
	f.nextID++
	f.inserts = append(f.inserts, ev)
	return f.nextID, time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC), nil
}

func (f *fakeFeedbackStore) AggregateWeekly(_ context.Context) ([]service.WeeklyBucket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.weeklyErr != nil {
		return nil, f.weeklyErr
	}
	// Return a copy so the test doesn't race with future Insert calls.
	out := make([]service.WeeklyBucket, len(f.weekly))
	copy(out, f.weekly)
	return out, nil
}

func (f *fakeFeedbackStore) TopDisputedOEMs(_ context.Context) ([]service.DisputedOEM, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.disputedErr != nil {
		return nil, f.disputedErr
	}
	out := make([]service.DisputedOEM, len(f.disputed))
	copy(out, f.disputed)
	return out, nil
}

func (f *fakeFeedbackStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.inserts)
}

func (f *fakeFeedbackStore) last() service.FeedbackEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.inserts) == 0 {
		return service.FeedbackEvent{}
	}
	return f.inserts[len(f.inserts)-1]
}

// buildFeedbackRouter registers every feedback route on a fresh gin
// engine. The router uses a shared handler so tests can inspect the
// handler's rate-limiter state across requests.
func buildFeedbackRouter(h *FeedbackHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/feedback", h.Submit)
	r.GET("/api/feedback/weekly", h.Weekly)
	r.GET("/api/feedback/disputed", h.Disputed)
	return r
}

// postFeedback fires a POST with the supplied body and returns the
// httptest recorder. Sets a fixed RemoteAddr so the rate-limiter keys
// each request on the same IP (unless overridden).
func postFeedback(r *gin.Engine, body []byte, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// ── T1 ────────────────────────────────────────────────────────────────
// POST with a valid payload → 200 + id returned + hashed identifiers
// stored (never raw). Also asserts the response shape matches the
// documented { id, createdAt } contract.
func TestFeedbackHandler_ValidPayload_Returns200WithID(t *testing.T) {
	store := newFakeStore()
	h := NewFeedbackHandler(store)
	defer h.Close()
	r := buildFeedbackRouter(h)

	body := []byte(`{
		"searchId":         "550e8400-e29b-41d4-a716-446655440000",
		"queryOem":         "26300-35505",
		"resultArticleId":  123456,
		"resultBrand":      "Bosch",
		"resultPartNum":    "F 026 400 100",
		"verdict":          "thumbs_up",
		"reason":           "exact match"
	}`)
	rec := postFeedback(r, body, "192.0.2.10:5555")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp FeedbackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v — body: %s", err, rec.Body.String())
	}
	if resp.ID == 0 {
		t.Errorf("response.id should be non-zero (got 0)")
	}
	if resp.CreatedAt == "" {
		t.Errorf("response.createdAt should be non-empty")
	}
	if store.count() != 1 {
		t.Fatalf("expected 1 stored feedback, got %d", store.count())
	}
	stored := store.last()
	if stored.QueryOEM != "26300-35505" {
		t.Errorf("stored.QueryOEM = %q, want %q", stored.QueryOEM, "26300-35505")
	}
	if stored.Verdict != service.FeedbackThumbsUp {
		t.Errorf("stored.Verdict = %q, want thumbs_up", stored.Verdict)
	}
	if stored.ResultArticleID != 123456 {
		t.Errorf("stored.ResultArticleID = %d, want 123456", stored.ResultArticleID)
	}
}

// ── T2 ────────────────────────────────────────────────────────────────
// POST with invalid verdict → 400. Tests each verdict the caller might
// stray toward — free text, capitalised, legacy up/down (which is
// intentionally rejected at the handler layer; the DB CHECK keeps
// accepting them for backfill compatibility).
func TestFeedbackHandler_InvalidVerdict_Returns400(t *testing.T) {
	badVerdicts := []string{
		"",                    // empty
		"up",                  // legacy, rejected by binding
		"down",                // legacy, rejected by binding
		"UP",                  // wrong case
		"thumbs-up",           // wrong separator
		"like",                // free text
		"👍",                   // emoji
		"thumbs_updown_bogus", // garbage superset
	}

	for _, verdict := range badVerdicts {
		t.Run("verdict="+verdict, func(t *testing.T) {
			store := newFakeStore()
			h := NewFeedbackHandler(store)
			defer h.Close()
			r := buildFeedbackRouter(h)

			body := []byte(fmt.Sprintf(`{
				"searchId":  "abc",
				"queryOem":  "26300-35505",
				"verdict":   %q
			}`, verdict))
			rec := postFeedback(r, body, "192.0.2.20:5555")

			if rec.Code != http.StatusBadRequest {
				t.Errorf("verdict %q: status = %d, want 400 (body: %s)",
					verdict, rec.Code, rec.Body.String())
			}
			assertErrorJSON(t, rec.Body.String())
			if store.count() != 0 {
				t.Errorf("verdict %q: nothing should have been stored, got %d rows",
					verdict, store.count())
			}
		})
	}
}

// ── T3 ────────────────────────────────────────────────────────────────
// POST with missing required field → 400. Covers each required field
// (searchId, queryOem, verdict) individually so a regression pin-points
// which field's binding tag regressed.
func TestFeedbackHandler_MissingRequiredField_Returns400(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "no searchId",
			body: `{"queryOem":"26300-35505","verdict":"thumbs_up"}`,
		},
		{
			name: "no queryOem",
			body: `{"searchId":"abc","verdict":"thumbs_up"}`,
		},
		{
			name: "no verdict",
			body: `{"searchId":"abc","queryOem":"26300-35505"}`,
		},
		{
			name: "empty JSON",
			body: `{}`,
		},
		{
			name: "malformed JSON",
			body: `{"searchId":`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			h := NewFeedbackHandler(store)
			defer h.Close()
			r := buildFeedbackRouter(h)

			rec := postFeedback(r, []byte(tc.body), "192.0.2.30:5555")

			if rec.Code != http.StatusBadRequest {
				t.Errorf("case %s: status = %d, want 400 (body: %s)",
					tc.name, rec.Code, rec.Body.String())
			}
			if store.count() != 0 {
				t.Errorf("case %s: nothing should have been stored", tc.name)
			}
		})
	}
}

// ── T4 ────────────────────────────────────────────────────────────────
// Rate-limit trigger → 429 after 60 requests. The middleware.RateLimiter
// is configured for 60 sustained rpm + burst 10 → the first 10 succeed
// immediately (draining the burst), further requests should hit the
// bucket-empty condition. We loop 80 times to have headroom over the
// 60-rpm sustained rate (which trickles in tokens at 1/sec).
//
// Because the test runs in real wall-clock time, we can't send 80
// requests through in less than ~1 second on a fast machine — after
// the initial burst is drained, at least one token will refill during
// the loop. To keep the test deterministic, we assert only that at
// least ONE request returned 429, and every request either succeeded
// (200) or was rate-limited (429).
func TestFeedbackHandler_RateLimit_Returns429(t *testing.T) {
	store := newFakeStore()
	h := NewFeedbackHandler(store)
	defer h.Close()
	r := buildFeedbackRouter(h)

	body := []byte(`{
		"searchId": "abc",
		"queryOem": "26300-35505",
		"verdict":  "thumbs_up"
	}`)

	got200 := 0
	got429 := 0
	other := 0

	// 80 requests, all from the same "client" (same RemoteAddr), fired
	// as fast as possible so the token bucket empties.
	for i := 0; i < 80; i++ {
		rec := postFeedback(r, body, "192.0.2.99:5555")
		switch rec.Code {
		case http.StatusOK:
			got200++
		case http.StatusTooManyRequests:
			got429++
		default:
			other++
			t.Logf("unexpected status %d on iter %d: %s", rec.Code, i, rec.Body.String())
		}
	}

	if got200 == 0 {
		t.Errorf("no request succeeded — rate limiter is too tight (200s=%d, 429s=%d, other=%d)",
			got200, got429, other)
	}
	if got429 == 0 {
		t.Errorf("no request was rate-limited — 80 rapid requests should trigger at least one 429 (200s=%d, 429s=%d, other=%d)",
			got200, got429, other)
	}
	if other > 0 {
		t.Errorf("saw %d unexpected non-200 non-429 responses", other)
	}
	// Sanity: total should be 80.
	if got200+got429+other != 80 {
		t.Errorf("accounting mismatch: 200s=%d + 429s=%d + other=%d != 80",
			got200, got429, other)
	}
}

// ── T5 ────────────────────────────────────────────────────────────────
// GET /api/feedback/weekly with empty DB → returns `[]`. The critical
// bit is the response body being the empty-array literal (not `null`)
// so the frontend can .map() over it without a null-check.
func TestFeedbackHandler_WeeklyEmpty_ReturnsEmptyArray(t *testing.T) {
	store := newFakeStore() // no weekly rows pre-loaded
	h := NewFeedbackHandler(store)
	defer h.Close()
	r := buildFeedbackRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback/weekly", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("body = %q, want %q", body, "[]")
	}
}

// TestFeedbackHandler_DisputedEmpty_ReturnsEmptyArray mirrors T5 for
// the /disputed endpoint — same empty-array invariant.
func TestFeedbackHandler_DisputedEmpty_ReturnsEmptyArray(t *testing.T) {
	store := newFakeStore()
	h := NewFeedbackHandler(store)
	defer h.Close()
	r := buildFeedbackRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback/disputed", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("body = %q, want %q", body, "[]")
	}
}

// ── T6 ────────────────────────────────────────────────────────────────
// The handler MUST hash the session cookie and the client IP with
// SHA256 before handing them to the store. Nothing in the stored
// row should be a raw IP or a raw cookie value. We assert both:
//
//  1. The stored user_hash / client_ip_hash match the SHA256(raw) hex
//     of the values we sent in — proving the handler ran hashing.
//  2. Neither field contains the raw values as substrings — proving
//     the raw values did not sneak past the hash.
//  3. Both fields, when non-empty, are shape-valid SHA256 hex
//     (64 lowercase hex chars).
func TestFeedbackHandler_HashesPIIBeforeStorage(t *testing.T) {
	store := newFakeStore()
	h := NewFeedbackHandler(store)
	defer h.Close()
	r := buildFeedbackRouter(h)

	const (
		rawSessionCookie = "sess_seller_alice_12345"
		rawClientIP      = "203.0.113.42"
	)
	body := []byte(`{
		"searchId": "search-abc",
		"queryOem": "26300-35505",
		"verdict":  "thumbs_up"
	}`)

	req := httptest.NewRequest(http.MethodPost, "/api/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = rawClientIP + ":5555"
	req.AddCookie(&http.Cookie{Name: "feedback_uid", Value: rawSessionCookie})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	stored := store.last()

	// (1) Deterministic hash-match — proves the hashing actually ran.
	expectedUserHash := sha256Hex(rawSessionCookie)
	expectedIPHash := sha256Hex(rawClientIP)
	if stored.UserHash != expectedUserHash {
		t.Errorf("UserHash = %q, want %q (SHA256 of session cookie)",
			stored.UserHash, expectedUserHash)
	}
	if stored.ClientIPHash != expectedIPHash {
		t.Errorf("ClientIPHash = %q, want %q (SHA256 of client IP)",
			stored.ClientIPHash, expectedIPHash)
	}

	// (2) Raw values must never appear in the stored row.
	// json-serialise the whole row and grep for the plaintexts.
	rawSerialised, _ := json.Marshal(stored)
	if strings.Contains(string(rawSerialised), rawSessionCookie) {
		t.Errorf("stored row contains RAW session cookie: %s", rawSerialised)
	}
	if strings.Contains(string(rawSerialised), rawClientIP) {
		t.Errorf("stored row contains RAW client IP: %s", rawSerialised)
	}

	// (3) Hash-shape assertion — 64 lowercase hex chars.
	hashRe := regexp.MustCompile(`^[a-f0-9]{64}$`)
	if !hashRe.MatchString(stored.UserHash) {
		t.Errorf("UserHash %q is not SHA256-hex-shaped", stored.UserHash)
	}
	if !hashRe.MatchString(stored.ClientIPHash) {
		t.Errorf("ClientIPHash %q is not SHA256-hex-shaped", stored.ClientIPHash)
	}
}

// TestFeedbackHandler_AnonymousVote_NoSessionCookie verifies that a
// request with NO session cookie set stores an empty UserHash (rather
// than a hash of the empty string, which would be a fixed sentinel
// value across all anonymous rows and would violate the "no PII"
// invariant in a subtle way — it'd let an attacker enumerate anonymous
// vs authenticated flows by their user_hash column).
func TestFeedbackHandler_AnonymousVote_EmptyUserHash(t *testing.T) {
	store := newFakeStore()
	h := NewFeedbackHandler(store)
	defer h.Close()
	r := buildFeedbackRouter(h)

	body := []byte(`{
		"searchId": "search-anon",
		"queryOem": "26300-35505",
		"verdict":  "thumbs_down"
	}`)
	rec := postFeedback(r, body, "192.0.2.77:5555")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	stored := store.last()
	if stored.UserHash != "" {
		t.Errorf("anonymous vote should carry empty UserHash, got %q", stored.UserHash)
	}
	// Client IP hash IS still stored (rate-limiting requires it).
	if stored.ClientIPHash == "" {
		t.Errorf("ClientIPHash should be set even for anonymous votes (needed for rate-limit ops-review)")
	}
}

// TestFeedbackHandler_ServiceNotConfigured_Returns503 verifies the nil-
// store branch returns 503 instead of panicking.
func TestFeedbackHandler_ServiceNotConfigured_Returns503(t *testing.T) {
	h := NewFeedbackHandler(nil) // no store
	defer h.Close()
	r := buildFeedbackRouter(h)

	body := []byte(`{"searchId":"a","queryOem":"b","verdict":"thumbs_up"}`)
	rec := postFeedback(r, body, "192.0.2.88:5555")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (body: %s)", rec.Code, rec.Body.String())
	}
	assertErrorJSON(t, rec.Body.String())
}

// TestFeedbackHandler_DBError_Returns500 verifies unexpected store
// errors surface as 500 with a JSON error body (not a panic, not a
// naked 200).
func TestFeedbackHandler_DBError_Returns500(t *testing.T) {
	store := newFakeStore()
	store.insertErr = errors.New("simulated db failure")
	h := NewFeedbackHandler(store)
	defer h.Close()
	r := buildFeedbackRouter(h)

	body := []byte(`{"searchId":"a","queryOem":"b","verdict":"thumbs_up"}`)
	rec := postFeedback(r, body, "192.0.2.66:5555")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (body: %s)", rec.Code, rec.Body.String())
	}
	assertErrorJSON(t, rec.Body.String())
}

// TestFeedbackHandler_AdminGate_UnauthorizedRejected verifies that
// when the admin token IS configured, unauthorised requests get 401
// on the aggregate endpoints (and authorised requests get through).
func TestFeedbackHandler_AdminGate_UnauthorizedRejected(t *testing.T) {
	store := newFakeStore()
	h := NewFeedbackHandler(store)
	defer h.Close()
	h.SetAdminAuthToken("s3cr3t-admin-token")
	r := buildFeedbackRouter(h)

	// Missing header → 401.
	req := httptest.NewRequest(http.MethodGet, "/api/feedback/weekly", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("missing token: status = %d, want 401 (body: %s)", rec.Code, rec.Body.String())
	}

	// Wrong token → 401.
	req2 := httptest.NewRequest(http.MethodGet, "/api/feedback/weekly", nil)
	req2.Header.Set("Authorization", "Bearer wrong-token")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", rec2.Code)
	}

	// Right token → 200 + `[]`.
	req3 := httptest.NewRequest(http.MethodGet, "/api/feedback/weekly", nil)
	req3.Header.Set("Authorization", "Bearer s3cr3t-admin-token")
	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Errorf("valid token: status = %d, want 200 (body: %s)", rec3.Code, rec3.Body.String())
	}
	if strings.TrimSpace(rec3.Body.String()) != "[]" {
		t.Errorf("empty-DB body = %q, want []", rec3.Body.String())
	}
}

// TestFeedbackHandler_WeeklyReturnsPreloadedBuckets exercises the happy
// path where the store DOES have data — verifies the handler passes it
// through unchanged and serialises WeeklyBucket cleanly.
func TestFeedbackHandler_WeeklyReturnsPreloadedBuckets(t *testing.T) {
	store := newFakeStore()
	store.weekly = []service.WeeklyBucket{
		{WeekStart: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), Verdict: service.FeedbackThumbsUp, Votes: 42},
		{WeekStart: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), Verdict: service.FeedbackThumbsDown, Votes: 7},
		{WeekStart: time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC), Verdict: service.FeedbackSkip, Votes: 3},
	}
	h := NewFeedbackHandler(store)
	defer h.Close()
	r := buildFeedbackRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback/weekly", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got []service.WeeklyBucket
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v — body: %s", err, rec.Body.String())
	}
	if len(got) != 3 {
		t.Errorf("expected 3 buckets, got %d", len(got))
	}
	if got[0].Verdict != service.FeedbackThumbsUp || got[0].Votes != 42 {
		t.Errorf("first bucket wrong: %+v", got[0])
	}
}

// TestFeedbackHandler_ResponseCreatedAt_IsRFC3339 verifies the format of
// the createdAt field so a JS Date(...) constructor on the frontend
// parses it correctly.
func TestFeedbackHandler_ResponseCreatedAt_IsRFC3339(t *testing.T) {
	store := newFakeStore()
	h := NewFeedbackHandler(store)
	defer h.Close()
	r := buildFeedbackRouter(h)

	body := []byte(`{"searchId":"a","queryOem":"b","verdict":"thumbs_up"}`)
	rec := postFeedback(r, body, "192.0.2.55:5555")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp FeedbackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, resp.CreatedAt); err != nil {
		t.Errorf("createdAt %q is not RFC3339: %v", resp.CreatedAt, err)
	}
}

// sha256HexTest mirrors the internal sha256Hex helper so the test file
// can compute expected hashes without importing internals. Kept as a
// separate function so a divergence between the two SHA256 impls would
// show up as a test failure rather than a silent match.
func sha256HexTest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// TestFeedbackHandler_SHA256HelperMatchesTestOracle is a sanity check
// that the handler's sha256Hex helper agrees with a naive re-implementation
// in this test file, so future refactors don't accidentally switch to a
// different hash without a compile error.
func TestFeedbackHandler_SHA256HelperMatchesTestOracle(t *testing.T) {
	samples := []string{
		"",
		"abc",
		"192.0.2.1",
		"sess_seller_alice_12345",
		strings.Repeat("x", 1000),
	}
	for _, s := range samples {
		got := sha256Hex(s)
		// Special case for the empty string — handler returns "" so
		// the DB stores NULL rather than a fixed-sentinel hash.
		if s == "" {
			if got != "" {
				t.Errorf("sha256Hex(\"\") = %q, want empty string", got)
			}
			continue
		}
		want := sha256HexTest(s)
		if got != want {
			t.Errorf("sha256Hex(%q) = %q, oracle = %q", s, got, want)
		}
	}
}

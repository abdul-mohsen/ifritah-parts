package handler

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/middleware"
	"parts-engine/internal/service"
)

// FeedbackHandler serves the search-result feedback API (M6.S2.T1):
//
//	POST /api/feedback              — record one thumbs-up/down/skip vote
//	GET  /api/feedback/weekly       — weekly aggregate (last 90 days)
//	GET  /api/feedback/disputed     — top-50 disputed OEMs (last 30 days)
//
// PII policy: the ONLY identifiers the handler ever passes to the storage
// layer are SHA256 fingerprints — never raw session cookies, never raw
// client IPs. The hashing happens inline in `Submit`, and a table-driven
// test in feedback_test.go asserts that stored rows carry hash-shaped
// values (64-char hex) rather than plaintext. See the "no raw PII in DB"
// acceptance criterion of M6.S2.T1.
//
// Rate limit: 60 votes/min per client IP, burst 10. Reuses the existing
// middleware.RateLimiter — no new dependency, and the per-bucket cleanup
// goroutine keeps memory bounded. Bucket lookups are indexed by
// c.ClientIP() (which is NOT persisted); the raw IP never crosses the
// process boundary.
type FeedbackHandler struct {
	store service.FeedbackStore

	limiter *middleware.RateLimiter

	// adminAuthToken gates the two aggregate endpoints. Set from the
	// ADMIN_AUTH_TOKEN env var in main.go — matches the pattern already
	// used by CommunityContribHandler. When empty the endpoints stay
	// open (dev mode); operators should set the env var in production.
	adminAuthToken string
}

// NewFeedbackHandler constructs a handler around the given store. The
// store MAY be nil — every dependent endpoint returns 503 in that case,
// consistent with how the community-contrib handler behaves when its
// backing service isn't wired.
func NewFeedbackHandler(store service.FeedbackStore) *FeedbackHandler {
	return &FeedbackHandler{
		store:   store,
		limiter: middleware.NewRateLimiter(60, 10),
	}
}

// SetAdminAuthToken configures the bearer token the aggregate endpoints
// check against. When empty, admin routes accept any request (dev mode
// only). Matches the pattern in CommunityContribHandler.SetAdminAuthToken.
func (h *FeedbackHandler) SetAdminAuthToken(tok string) {
	h.adminAuthToken = tok
}

// Close releases the rate-limiter's background cleanup goroutine. Called
// from tests to avoid leaking goroutines; in production the handler
// lives for the whole process lifetime so cleanup happens on exit.
func (h *FeedbackHandler) Close() {
	if h.limiter != nil {
		h.limiter.Stop()
	}
}

// FeedbackRequest is the JSON body accepted by POST /api/feedback.
// The field names are the OpenAPI camelCase form used elsewhere in the
// codebase. `binding:"required"` fires Gin's validator, which returns
// 400 automatically before the handler body runs.
type FeedbackRequest struct {
	SearchID        string `json:"searchId"        binding:"required"`
	QueryOEM        string `json:"queryOem"        binding:"required"`
	ResultArticleID int32  `json:"resultArticleId"`
	ResultBrand     string `json:"resultBrand"`
	ResultPartNum   string `json:"resultPartNum"`
	Verdict         string `json:"verdict"         binding:"required,oneof=thumbs_up thumbs_down skip"`
	Reason          string `json:"reason,omitempty"`
}

// FeedbackResponse is the JSON body returned on a successful POST.
type FeedbackResponse struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"createdAt"`
}

// Submit is POST /api/feedback.
//
// Flow:
//  1. Rate-limit check (returns 429 on exceed).
//  2. Bind + validate the JSON body (returns 400 on schema error).
//  3. Hash the session cookie + client IP with SHA256.
//  4. Insert via the FeedbackStore.
//  5. Return { id, createdAt }.
//
// The 429 branch runs BEFORE ShouldBindJSON so a spamming client can't
// burn CPU on payload parsing.
func (h *FeedbackHandler) Submit(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "feedback service not configured",
		})
		return
	}

	// Rate-limit by client IP. .Allow() returns false when the token
	// bucket is empty — 60 votes/min sustained, burst 10.
	if !h.limiter.Allow(c.ClientIP()) {
		c.Header("Retry-After", "60")
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "rate limit exceeded",
			"hint":  "feedback is limited to 60 votes/minute per client",
		})
		return
	}

	var req FeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Derive the PII hashes. The session-cookie name matches the two
	// established cookies in the codebase: prefer `feedback_uid` (set
	// by the frontend widget), fall back to `session_id` if present.
	// Both are optional — anonymous votes are legal.
	rawSession := readSessionCookie(c)
	rawIP := c.ClientIP()

	ev := service.FeedbackEvent{
		SearchID:        req.SearchID,
		QueryOEM:        req.QueryOEM,
		ResultArticleID: req.ResultArticleID,
		ResultBrand:     req.ResultBrand,
		ResultPartNum:   req.ResultPartNum,
		Verdict:         service.FeedbackVerdict(req.Verdict),
		Reason:          req.Reason,
		UserHash:        sha256Hex(rawSession),
		ClientIPHash:    sha256Hex(rawIP),
	}

	id, createdAt, err := h.store.Insert(c.Request.Context(), ev)
	if err != nil {
		if errors.Is(err, service.ErrFeedbackDBNotConnected) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		// Verdict / required-field errors surface here too — the store
		// re-validates for defence-in-depth.
		if strings.Contains(err.Error(), "invalid verdict") ||
			strings.Contains(err.Error(), "required") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[feedback] insert error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record feedback"})
		return
	}

	c.JSON(http.StatusOK, FeedbackResponse{
		ID:        id,
		CreatedAt: createdAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

// Weekly is GET /api/feedback/weekly — admin-gated report endpoint.
// Empty result set → returns `[]` (never null), which the frontend
// treats as "no data yet" without a null-check.
func (h *FeedbackHandler) Weekly(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "feedback service not configured"})
		return
	}
	buckets, err := h.store.AggregateWeekly(c.Request.Context())
	if err != nil {
		if errors.Is(err, service.ErrFeedbackDBNotConnected) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[feedback] weekly error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch weekly aggregate"})
		return
	}
	if buckets == nil {
		buckets = []service.WeeklyBucket{}
	}
	c.JSON(http.StatusOK, buckets)
}

// Disputed is GET /api/feedback/disputed — admin-gated report endpoint.
func (h *FeedbackHandler) Disputed(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "feedback service not configured"})
		return
	}
	rows, err := h.store.TopDisputedOEMs(c.Request.Context())
	if err != nil {
		if errors.Is(err, service.ErrFeedbackDBNotConnected) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[feedback] disputed error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch disputed OEMs"})
		return
	}
	if rows == nil {
		rows = []service.DisputedOEM{}
	}
	c.JSON(http.StatusOK, rows)
}

// requireAdmin implements the same bearer-token gate used by
// CommunityContribHandler. Empty token = dev mode (accept anything);
// set token = require exact `Authorization: Bearer <token>` header
// with a constant-time compare so per-byte timing does not leak the
// secret. Returns true when the request may proceed.
func (h *FeedbackHandler) requireAdmin(c *gin.Context) bool {
	if h.adminAuthToken == "" {
		return true // dev mode — TODO: gate behind admin auth once ADMIN_AUTH_TOKEN is standardised in prod
	}
	provided := []byte(c.GetHeader("Authorization"))
	expected := []byte("Bearer " + h.adminAuthToken)
	if subtle.ConstantTimeCompare(provided, expected) != 1 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "admin token required"})
		return false
	}
	return true
}

// readSessionCookie returns the raw session identifier from the client.
// Order of precedence: dedicated `feedback_uid` cookie (set by the
// widget), then the generic `session_id` cookie, then an empty string
// (anonymous vote — legal). The returned value is HASHED before being
// stored, never kept in memory beyond this stack frame.
func readSessionCookie(c *gin.Context) string {
	for _, name := range []string{"feedback_uid", "session_id"} {
		if v, err := c.Cookie(name); err == nil && v != "" {
			return v
		}
	}
	return ""
}

// sha256Hex returns the lower-case hex SHA256 of s, or the empty string
// when s is empty. Empty-in / empty-out lets us store a NULL for
// anonymous votes rather than a hash of the empty string (which would
// be a fixed sentinel value across every anonymous row).
func sha256Hex(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

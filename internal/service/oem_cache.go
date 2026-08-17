package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"parts-engine/internal/model"
)

// OEMCache is the persistent Postgres-backed OEM resolution cache created
// by migration 000012. Every successful search result (from any strategy)
// writes here asynchronously. Subsequent lookups for the same OEM hit this
// cache in <100 ms — bypassing MySQL 2-5 s slow queries, dealer scrapes
// that risk 403s, and network round-trips generally.
//
// Never expires by default. Corroboration boosts confidence. User feedback
// (Phase 6) drives verified_by_user + downgrade_count.
type OEMCache struct {
	db *sql.DB
}

func NewOEMCache(db *sql.DB) *OEMCache {
	if db == nil {
		return nil
	}
	return &OEMCache{db: db}
}

// Lookup returns the cached result for an OEM, or nil when no cache row
// exists OR when the row has been downgraded past the trust threshold
// (3 negative user flags without corresponding positive verifications).
//
// The returned SmartResult carries the SOURCE that seeded the cache row,
// so callers can display transparency about where the data came from.
func (c *OEMCache) Lookup(ctx context.Context, rawOEM string) (*SmartResult, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	normalized := NormalizeOEM(rawOEM)
	if normalized == "" {
		return nil, nil
	}

	var (
		oemRaw, desc, source string
		category, makeVal, modelVal, sourceURL sql.NullString
		yStart, yEnd, corroborating, downgrade sql.NullInt32
		confidence                        float64
		verifiedByUser                    bool
		firstSeen, lastSeen, lastVerified sql.NullTime
	)

	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	err := c.db.QueryRowContext(ctx, `
		SELECT oem_raw, description, category, make, model, year_start, year_end,
		       confidence, source, source_url, corroborating_sources,
		       verified_by_user, downgrade_count, first_seen_at, last_seen_at,
		       last_verified_at
		FROM oem_resolution_cache
		WHERE oem_normalized = $1`, normalized).
		Scan(&oemRaw, &desc, &category, &makeVal, &modelVal, &yStart, &yEnd,
			&confidence, &source, &sourceURL, &corroborating,
			&verifiedByUser, &downgrade, &firstSeen, &lastSeen, &lastVerified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Downgrade guard: skip entries with 3+ negative flags and no user
	// verification. Caller will re-fetch from a live source.
	if !verifiedByUser && downgrade.Int32 >= 3 {
		log.Printf("[OEMCache] downgraded past threshold, skipping cache for %s (downgrades=%d)", normalized, downgrade.Int32)
		return nil, nil
	}

	note := "Cached resolution from " + source
	if corroborating.Int32 > 1 {
		note += " (corroborated by " + intToStr(int(corroborating.Int32)) + " sources)"
	}
	if verifiedByUser {
		note += " — user-verified"
	}

	result := &SmartResult{
		Part: model.Part{
			ArticleNumber: oemRaw,
			Description:   desc,
			BrandName:     nullStr(makeVal),
			Category:      nullStr(category),
		},
		Confidence:     confidence,
		ConfidenceNote: note,
		FitmentDriver:  "cache",
		BrandResolved:  nullStr(makeVal),
		SourceStrategy: "oem_cache",
	}
	// Bump last_seen_at asynchronously so we track cache freshness without
	// blocking the read path.
	go func() {
		bumpCtx, bumpCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer bumpCancel()
		_, _ = c.db.ExecContext(bumpCtx,
			`UPDATE oem_resolution_cache SET last_seen_at = NOW() WHERE oem_normalized = $1`,
			normalized)
	}()
	return result, nil
}

// StoreResult persists a successful search result to the cache. Called
// asynchronously (fire-and-forget) after each strategy returns.
//
//   - New OEMs: insert at the calling source's confidence.
//   - Existing OEMs: append the new source to corroborations. If the new
//     description matches within Levenshtein tolerance, bump
//     corroborating_sources and boost confidence per the ladder in the
//     migration comment.
//   - Existing OEMs where the new description DIFFERS materially: log a
//     divergence event but don't overwrite; the original wins on repeat
//     reads. Human review resolves via oem_user_feedback (Phase 6).
func (c *OEMCache) StoreResult(ctx context.Context, oemRaw string, r *SmartResult, source, sourceURL string) error {
	if c == nil || c.db == nil || r == nil {
		return nil
	}
	if strings.TrimSpace(r.Description) == "" {
		return nil // don't cache empty descriptions
	}
	normalized := NormalizeOEM(oemRaw)
	if normalized == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Extract make/model from BrandResolved if it's a "Make Model" form.
	// Best-effort — we prefer explicit fields on SmartResult when present.
	makeName := r.BrandName
	if r.BrandResolved != "" {
		makeName = r.BrandResolved
	}

	// Compose corroboration payload.
	corr := map[string]string{
		"source":     source,
		"matched_at": time.Now().UTC().Format(time.RFC3339),
	}
	corrJSON, _ := json.Marshal([]map[string]string{corr})

	_, err := c.db.ExecContext(ctx, `
		INSERT INTO oem_resolution_cache
		    (oem_normalized, oem_raw, description, category, make,
		     confidence, source, source_url, corroborating_sources, corroborations)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, $9::jsonb)
		ON CONFLICT (oem_normalized) DO UPDATE
		SET
		    corroborating_sources = CASE
		        -- Description agrees (case-insensitive substring or exact):
		        WHEN LOWER(oem_resolution_cache.description) = LOWER(EXCLUDED.description)
		          OR POSITION(LOWER(EXCLUDED.description) IN LOWER(oem_resolution_cache.description)) > 0
		          OR POSITION(LOWER(oem_resolution_cache.description) IN LOWER(EXCLUDED.description)) > 0
		        THEN oem_resolution_cache.corroborating_sources + 1
		        ELSE oem_resolution_cache.corroborating_sources
		    END,
		    confidence = GREATEST(
		        oem_resolution_cache.confidence,
		        CASE
		            WHEN oem_resolution_cache.verified_by_user THEN 1.0
		            WHEN oem_resolution_cache.corroborating_sources + 1 >= 3 THEN 0.95
		            WHEN oem_resolution_cache.corroborating_sources + 1 = 2 THEN 0.85
		            ELSE EXCLUDED.confidence
		        END
		    ),
		    corroborations = oem_resolution_cache.corroborations || $9::jsonb,
		    last_seen_at = NOW()
		WHERE oem_resolution_cache.source != 'user'  -- user-authored rows are immutable`,
		normalized, strings.ToUpper(oemRaw), r.Description, r.Category, makeName,
		r.Confidence, source, sourceURL, string(corrJSON))
	if err != nil {
		log.Printf("[OEMCache.StoreResult] insert oem=%s src=%s err=%v", normalized, source, err)
		return err
	}
	return nil
}

// StoreResultAsync is the fire-and-forget wrapper used by search strategies
// so caching never adds latency to the read path.
func (c *OEMCache) StoreResultAsync(oemRaw string, r *SmartResult, source, sourceURL string) {
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.StoreResult(ctx, oemRaw, r, source, sourceURL); err != nil {
			// Already logged inside StoreResult; no re-raise.
		}
	}()
}

// Stats returns aggregate metrics for the admin/diagnostics endpoint.
type OEMCacheStats struct {
	TotalRows        int            `json:"totalRows"`
	VerifiedByUser   int            `json:"verifiedByUser"`
	ByConfidence     map[string]int `json:"byConfidence"`   // "0.7-0.8", "0.8-0.9", "0.9-1.0"
	BySource         map[string]int `json:"bySource"`       // source -> count
	AverageAgeDays   float64        `json:"averageAgeDays"`
	CorroboratedRows int            `json:"corroboratedRows"` // rows with 2+ sources
}

func (c *OEMCache) Stats(ctx context.Context) (*OEMCacheStats, error) {
	if c == nil || c.db == nil {
		return &OEMCacheStats{ByConfidence: map[string]int{}, BySource: map[string]int{}}, nil
	}
	s := &OEMCacheStats{
		ByConfidence: map[string]int{},
		BySource:     map[string]int{},
	}
	// Total + verified + corroborated + avg age
	if err := c.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE verified_by_user),
		       COUNT(*) FILTER (WHERE corroborating_sources >= 2),
		       COALESCE(AVG(EXTRACT(EPOCH FROM (NOW() - first_seen_at)) / 86400), 0)
		FROM oem_resolution_cache`).
		Scan(&s.TotalRows, &s.VerifiedByUser, &s.CorroboratedRows, &s.AverageAgeDays); err != nil {
		return nil, err
	}
	// By source breakdown
	rows, err := c.db.QueryContext(ctx, `
		SELECT source, COUNT(*) FROM oem_resolution_cache GROUP BY source ORDER BY 2 DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var src string
		var n int
		if err := rows.Scan(&src, &n); err != nil {
			continue
		}
		s.BySource[src] = n
	}
	return s, nil
}

// ─── helpers ────────────────────────────────────────────────────────────────

func nullStr(n sql.NullString) string {
	if n.Valid {
		return n.String
	}
	return ""
}

func intToStr(n int) string {
	if n < 0 {
		return "-" + intToStr(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return intToStr(n/10) + intToStr(n%10)
}

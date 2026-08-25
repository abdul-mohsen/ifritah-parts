package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// RelatedPart is one entry in the "if you're buying X, also change Y" table.
// Populated by db/migrations/000015_related_parts.sql from hard-coded
// Hyundai/Kia service-interval schedules; extended by user-cart
// co-occurrence once M6.S2 lands.
type RelatedPart struct {
	Category    string  `json:"category"`
	Correlation float64 `json:"correlation"` // 0-1, higher = stronger co-occurrence
	Evidence    string  `json:"evidence"`    // 'service_60k' / 'service_90k' / etc.
	Priority    int     `json:"priority"`    // 0-100, sort order within results
}

// RelatedParts answers "given this OEM, what should the seller offer next?"
// via the related_parts co-occurrence table. See M5.S3 in
// docs/sprints/M5-M6-intelligence-and-production.md.
type RelatedParts struct {
	db *sql.DB
}

func NewRelatedParts(db *sql.DB) *RelatedParts {
	if db == nil {
		return &RelatedParts{}
	}
	return &RelatedParts{db: db}
}

// FindRelatedByOEM decodes the OEM's category via prefixMap, then looks up
// related categories in the co-occurrence table. Returns top-N by priority.
//
// When the OEM doesn't decode, returns nil, nil — the caller shouldn't
// assume this is an error; some OEMs (dealer accessory prefixes, unseeded
// families) legitimately have no category.
//
// Limit clamped to [1, 20]. Default 5.
func (r *RelatedParts) FindRelatedByOEM(ctx context.Context, oem string, limit int) ([]RelatedPart, error) {
	if r.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}

	cat := DecodeOEMPrefix(oem)
	if cat == nil || cat.Category == "" {
		return nil, nil
	}
	return r.findByCategory(ctx, cat.Category, limit)
}

// FindRelatedByCategory - direct-category variant used by tests + the
// /api/parts/related endpoint when the caller has a resolved category
// rather than a raw OEM.
func (r *RelatedParts) FindRelatedByCategory(ctx context.Context, category string, limit int) ([]RelatedPart, error) {
	if r.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	if strings.TrimSpace(category) == "" {
		return nil, nil
	}
	return r.findByCategory(ctx, category, limit)
}

func (r *RelatedParts) findByCategory(ctx context.Context, category string, limit int) ([]RelatedPart, error) {
	const q = `
		SELECT related_category, correlation, evidence_source, priority
		FROM related_parts
		WHERE source_category = $1
		ORDER BY priority DESC, correlation DESC
		LIMIT $2`

	rows, err := r.db.QueryContext(ctx, q, category, limit)
	if err != nil {
		return nil, fmt.Errorf("related_parts query: %w", err)
	}
	defer rows.Close()

	var out []RelatedPart
	for rows.Next() {
		var rp RelatedPart
		if err := rows.Scan(&rp.Category, &rp.Correlation, &rp.Evidence, &rp.Priority); err != nil {
			continue
		}
		out = append(out, rp)
	}
	return out, nil
}

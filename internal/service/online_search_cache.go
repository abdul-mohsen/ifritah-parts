package service

// M8 online-search cache repository.
//
// Backing table: aftermarket_online_cache (migration 000021).
// Backend: Postgres via pgx.
//
// Design notes:
//   * Reads happen on the hot path — cache hits must return in < 50 ms
//     to preserve the p95 latency budget in FindAftermarketForOEM. The
//     idx_aftermarket_online_cache_oem index handles this.
//   * Writes happen off the hot path via UpsertAsync — the caller does
//     NOT wait for them. Failures are logged but do not fail the search.
//   * We store one row per (source, oem_normalized, brand, part_number)
//     tuple. UNIQUE constraint + ON CONFLICT DO UPDATE handle re-fetches.

import (
	"context"
	"database/sql"
	"log"
	"time"

	"parts-engine/internal/model"
)

// AftermarketOnlineCacheRepo is the persistence layer for online-source
// results.
type AftermarketOnlineCacheRepo struct {
	db *sql.DB
}

// NewAftermarketOnlineCacheRepo returns a repo bound to the given
// Postgres connection.
func NewAftermarketOnlineCacheRepo(db *sql.DB) *AftermarketOnlineCacheRepo {
	return &AftermarketOnlineCacheRepo{db: db}
}

// FreshFor returns the fresh (fetched_at + ttl_seconds > NOW()) rows
// cached for the given OEM across all sources. Result order:
// most-recently-fetched first.
//
// Callers should filter/dedupe further in the dispatcher; this repo is
// intentionally dumb about business logic.
func (r *AftermarketOnlineCacheRepo) FreshFor(ctx context.Context, oemNormalized string) ([]model.AftermarketPart, error) {
	if r == nil || r.db == nil || oemNormalized == "" {
		return nil, nil
	}
	const q = `
		SELECT source, brand, part_number, description, price_cents,
		       currency, condition, image_url, source_url
		FROM aftermarket_online_cache
		WHERE oem_normalized = $1
		  AND fetched_at + (ttl_seconds * INTERVAL '1 second') > NOW()
		ORDER BY fetched_at DESC
		LIMIT 100`
	rows, err := r.db.QueryContext(ctx, q, oemNormalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.AftermarketPart
	for rows.Next() {
		var (
			source, brand, partNumber string
			description               sql.NullString
			priceCents                sql.NullInt64
			currency, condition       sql.NullString
			imageURL, sourceURL       sql.NullString
		)
		if err := rows.Scan(&source, &brand, &partNumber, &description,
			&priceCents, &currency, &condition, &imageURL, &sourceURL); err != nil {
			return nil, err
		}
		p := model.AftermarketPart{
			PartNumber:  partNumber,
			Description: description.String,
			Brand:       brand,
			Source:      source,
			SourceURL:   sourceURL.String,
			PriceCents:  priceCents.Int64,
			Currency:    currency.String,
			Condition:   condition.String,
			ImageURL:    imageURL.String,
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertMany writes a slice of results to the cache, one row per
// (source, oem, brand, part_number) tuple. Existing rows have their
// fetched_at + values refreshed. Called by the dispatcher after a
// cache-miss fan-out.
func (r *AftermarketOnlineCacheRepo) UpsertMany(ctx context.Context, oemNormalized string, ttl time.Duration, parts []model.AftermarketPart) error {
	if r == nil || r.db == nil || len(parts) == 0 {
		return nil
	}
	ttlSec := int(ttl.Seconds())
	if ttlSec <= 0 {
		ttlSec = 30 * 24 * 60 * 60 // 30 days default
	}

	const q = `
		INSERT INTO aftermarket_online_cache
		    (source, oem_normalized, brand, part_number, description,
		     price_cents, currency, condition, image_url, source_url,
		     fetched_at, ttl_seconds)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11)
		ON CONFLICT (source, oem_normalized, brand, part_number) DO UPDATE SET
		    description = EXCLUDED.description,
		    price_cents = EXCLUDED.price_cents,
		    currency    = EXCLUDED.currency,
		    condition   = EXCLUDED.condition,
		    image_url   = EXCLUDED.image_url,
		    source_url  = EXCLUDED.source_url,
		    fetched_at  = NOW(),
		    ttl_seconds = EXCLUDED.ttl_seconds`

	for _, p := range parts {
		if p.PartNumber == "" || p.Brand == "" {
			continue
		}
		source := p.Source
		if source == "" {
			source = "unknown"
		}
		_, err := r.db.ExecContext(ctx, q,
			source, oemNormalized, p.Brand, p.PartNumber, p.Description,
			p.PriceCents, p.Currency, p.Condition, p.ImageURL, p.SourceURL,
			ttlSec)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpsertAsync fires UpsertMany off the hot path. Errors are logged only.
func (r *AftermarketOnlineCacheRepo) UpsertAsync(oemNormalized string, ttl time.Duration, parts []model.AftermarketPart) {
	if r == nil || len(parts) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.UpsertMany(ctx, oemNormalized, ttl, parts); err != nil {
			log.Printf("[AftermarketOnlineCache] UpsertAsync oem=%s err=%v", oemNormalized, err)
		}
	}()
}

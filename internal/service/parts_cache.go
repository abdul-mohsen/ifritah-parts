package service

import (
	"database/sql"
	"encoding/json"
	"log"
	"strings"
	"time"

	"parts-engine/internal/model"
)

// PartsCache provides SQLite-backed caching for online part lookups.
type PartsCache struct {
	db *sql.DB
}

// NewPartsCache creates a cache backed by the given SQLite connection.
// It auto-creates the cache table if it doesn't exist.
func NewPartsCache(db *sql.DB) *PartsCache {
	if db == nil {
		return nil
	}
	c := &PartsCache{db: db}
	c.ensureTable()
	return c
}

func (c *PartsCache) ensureTable() {
	_, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS online_parts_cache (
			oem_number    TEXT PRIMARY KEY,
			description   TEXT,
			make          TEXT,
			category      TEXT,
			substitutions TEXT,
			aftermarket   TEXT,
			compatibility TEXT,
			source        TEXT DEFAULT 'partsouq',
			fetched_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Printf("parts_cache: failed to create table: %v", err)
	}
}

// GetCached returns a cached result if it exists and is not expired.
// Positive results have 30-day TTL, negative results ("not_found") have 7-day TTL.
// Returns a special "not_found" marker result (Description == "") for negative cache hits.
func (c *PartsCache) GetCached(oemNumber string) *model.OnlinePartResult {
	if c == nil || c.db == nil {
		return nil
	}

	var (
		desc, make_, cat, source        string
		subsJSON, afterJSON, compatJSON sql.NullString
		fetchedAt                       time.Time
	)
	err := c.db.QueryRow(`
		SELECT description, make, category, substitutions, aftermarket, compatibility, source, fetched_at
		FROM online_parts_cache WHERE oem_number = ?
	`, oemNumber).Scan(&desc, &make_, &cat, &subsJSON, &afterJSON, &compatJSON, &source, &fetchedAt)
	if err != nil {
		return nil
	}

	isNegative := source == "not_found"
	ttl := 30 * 24 * time.Hour // 30 days for positive
	if isNegative {
		ttl = 7 * 24 * time.Hour // 7 days for negative
	}
	if time.Since(fetchedAt) > ttl {
		return nil
	}

	// Return negative cache marker (Description="" and Source="not_found")
	if isNegative {
		return &model.OnlinePartResult{
			PartNumber: oemNumber,
			Source:     "not_found",
		}
	}

	result := &model.OnlinePartResult{
		PartNumber:  oemNumber,
		Description: desc,
		Make:        make_,
		Category:    cat,
		Source:      source,
	}

	if subsJSON.Valid {
		json.Unmarshal([]byte(subsJSON.String), &result.Substitutions)
	}
	if afterJSON.Valid {
		json.Unmarshal([]byte(afterJSON.String), &result.Aftermarket)
	}
	if compatJSON.Valid {
		json.Unmarshal([]byte(compatJSON.String), &result.Compatibility)
	}

	return result
}

// StoreNegative caches a "not found" result to avoid re-fetching unknown parts.
func (c *PartsCache) StoreNegative(oemNumber string) error {
	if c == nil || c.db == nil {
		return nil
	}
	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO online_parts_cache
			(oem_number, description, make, category, substitutions, aftermarket, compatibility, source, fetched_at)
		VALUES (?, '', '', '', '[]', '[]', '[]', 'not_found', CURRENT_TIMESTAMP)
	`, oemNumber)
	return err
}

// StoreCache saves an online result into the SQLite cache.
func (c *PartsCache) StoreCache(oemNumber string, result *model.OnlinePartResult) error {
	if c == nil || c.db == nil || result == nil {
		return nil
	}

	subsJSON, _ := json.Marshal(result.Substitutions)
	afterJSON, _ := json.Marshal(result.Aftermarket)
	compatJSON, _ := json.Marshal(result.Compatibility)

	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO online_parts_cache
			(oem_number, description, make, category, substitutions, aftermarket, compatibility, source, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, oemNumber, result.Description, result.Make, result.Category,
		string(subsJSON), string(afterJSON), string(compatJSON), result.Source)
	return err
}

// ClearNegativeCache removes all negative cache entries so they can be retried.
func (c *PartsCache) ClearNegativeCache() (int64, error) {
	if c == nil || c.db == nil {
		return 0, nil
	}
	res, err := c.db.Exec(`DELETE FROM online_parts_cache WHERE source = 'not_found'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ensureDealerTable creates the dealer_parts_index if missing.
func (c *PartsCache) ensureDealerTable() {
	if c == nil || c.db == nil {
		return
	}
	_, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS dealer_parts_index (
			oem_number  TEXT PRIMARY KEY,
			description TEXT,
			make        TEXT,
			category    TEXT,
			source      TEXT,
			fetched_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		log.Printf("dealer_parts_index: create table err: %v", err)
	}
}

// StoreDealerPart saves a part from a dealer site into the local dictionary.
func (c *PartsCache) StoreDealerPart(oemNumber, description, make_, category, source string) error {
	if c == nil || c.db == nil {
		return nil
	}
	c.ensureDealerTable()
	_, err := c.db.Exec(`
		INSERT OR IGNORE INTO dealer_parts_index (oem_number, description, make, category, source, fetched_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, oemNumber, description, make_, category, source)
	return err
}

// GetDealerPart checks the dealer dictionary for a part.
func (c *PartsCache) GetDealerPart(oemNumber string) *model.OnlinePartResult {
	if c == nil || c.db == nil {
		return nil
	}
	c.ensureDealerTable()
	normalized := normalizePartNumberForCache(oemNumber)
	var desc, make_, cat, src string
	err := c.db.QueryRow(`
		SELECT description, make, category, source FROM dealer_parts_index
		WHERE oem_number = ? OR oem_number = ?
	`, oemNumber, normalized).Scan(&desc, &make_, &cat, &src)
	if err != nil {
		return nil
	}
	return &model.OnlinePartResult{
		PartNumber:  oemNumber,
		Description: desc,
		Make:        make_,
		Category:    cat,
		Source:      "dealer_" + src,
	}
}

// GetDealerPartByPrefix checks the dealer dictionary by OEM prefix (first 8 chars).
func (c *PartsCache) GetDealerPartByPrefix(normalizedPrefix string) *model.OnlinePartResult {
	if c == nil || c.db == nil || len(normalizedPrefix) < 8 {
		return nil
	}
	c.ensureDealerTable()
	var num, desc, make_, cat, src string
	err := c.db.QueryRow(`
		SELECT oem_number, description, make, category, source FROM dealer_parts_index
		WHERE oem_number LIKE ?
		LIMIT 1
	`, normalizedPrefix+"%").Scan(&num, &desc, &make_, &cat, &src)
	if err != nil {
		return nil
	}
	return &model.OnlinePartResult{
		PartNumber:  num,
		Description: desc,
		Make:        make_,
		Category:    cat,
		Source:      "dealer_prefix_" + src,
	}
}

func normalizePartNumberForCache(pn string) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(pn) {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// FindBySubstitution checks if any cached part lists the given normalized OEM number
// in its substitutions JSON array. Returns the parent part if found.
func (c *PartsCache) FindBySubstitution(normalizedOEM string) *model.OnlinePartResult {
	if c == nil || c.db == nil || normalizedOEM == "" {
		return nil
	}
	// Search substitutions JSON for the normalized OEM number
	var oemNum, desc, make_, cat, subsJSON string
	err := c.db.QueryRow(`
		SELECT oem_number, description, make, category, substitutions
		FROM online_parts_cache
		WHERE source != 'not_found'
		  AND (substitutions LIKE ? OR substitutions LIKE ?)
		LIMIT 1
	`, "%"+normalizedOEM+"%", "%"+strings.ToUpper(normalizedOEM)+"%").Scan(&oemNum, &desc, &make_, &cat, &subsJSON)
	if err != nil {
		return nil
	}
	// Verify it's actually in the substitutions list (not a substring match of the oem_number itself)
	var subs []model.SubstitutionPart
	json.Unmarshal([]byte(subsJSON), &subs)
	for _, sub := range subs {
		subNorm := normalizePartNumberForCache(sub.PartNumber)
		if subNorm == strings.ToUpper(normalizedOEM) {
			return &model.OnlinePartResult{
				PartNumber:  oemNum,
				Description: desc,
				Make:        make_,
				Category:    cat,
				Source:      "supersession",
			}
		}
	}
	return nil
}

-- ============================================================================
-- 000017 - aftermarket_rockauto (M4.S1.T2)
-- ============================================================================
-- Landing table for the RockAuto scraper output (M4.S1.T1 - not shipping
-- the scraper in this PR; scraper is external and requires Playwright +
-- anti-bot mitigation).
--
-- Once populated, FindAftermarketForOEM_MultiPath will union rows from
-- this table alongside articlecrosses + oem_number + oem_search_index -
-- so RockAuto-only aftermarket brands surface when TecDoc misses them.
--
-- Schema mirrors what a RockAuto walk produces per part:
--   oem_normalized  - the input OEM that led to this cross-ref
--   brand           - aftermarket brand (Bosch, MANN, MAHLE, etc.)
--   part_number     - the brand's own SKU
--   category        - RockAuto category label (may not exactly match TecDoc)
--   price_usd_cents - listed price at scrape time
--   source_url      - deep-link back to the RockAuto listing
--   scraped_at      - freshness stamp for the periodic-refresh cron
-- ============================================================================

CREATE TABLE IF NOT EXISTS aftermarket_rockauto (
	id              BIGSERIAL PRIMARY KEY,
	oem_normalized  TEXT NOT NULL,
	brand           TEXT NOT NULL,
	part_number     TEXT NOT NULL,
	description     TEXT,
	category        TEXT,
	price_usd_cents INTEGER,
	source_url      TEXT,
	scraped_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE (oem_normalized, brand, part_number)
);

CREATE INDEX IF NOT EXISTS idx_aftermarket_rockauto_oem
	ON aftermarket_rockauto (oem_normalized);
CREATE INDEX IF NOT EXISTS idx_aftermarket_rockauto_scraped
	ON aftermarket_rockauto (scraped_at DESC);

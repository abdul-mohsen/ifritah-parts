-- ============================================================================
-- 000020 - aftermarket_regional (M4.S2.T2 scaffold)
-- ============================================================================
-- Landing table for regional supplier catalogs (Ali Al-Ghanim, Al-Futtaim,
-- Petromin, etc. — see docs/data-sources/regional-catalog-survey.md for
-- feasibility matrix). Every supplier gets their own scraper/importer
-- but they all write here in the same shape so FindAftermarketForOEM
-- can union them uniformly.
-- ============================================================================

CREATE TABLE IF NOT EXISTS aftermarket_regional (
	id              BIGSERIAL PRIMARY KEY,
	oem_normalized  TEXT NOT NULL,
	supplier        TEXT NOT NULL,           -- 'ali_al_ghanim' / 'al_futtaim' / 'petromin' / etc.
	brand           TEXT,
	part_number     TEXT NOT NULL,
	description     TEXT,
	stock_status    TEXT,                     -- 'in_stock' / 'out_of_stock' / 'unknown'
	region          TEXT,                     -- 'KSA' / 'UAE' / 'GCC' / etc.
	url             TEXT,                     -- deep-link back to supplier catalog
	price_local     NUMERIC(10, 2),          -- price in supplier's local currency
	price_currency  TEXT,                     -- 'SAR' / 'AED' / 'KWD' / 'BHD'
	created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE (oem_normalized, supplier, part_number)
);

CREATE INDEX IF NOT EXISTS idx_aftermarket_regional_oem
	ON aftermarket_regional (oem_normalized);
CREATE INDEX IF NOT EXISTS idx_aftermarket_regional_supplier
	ON aftermarket_regional (supplier);
CREATE INDEX IF NOT EXISTS idx_aftermarket_regional_updated
	ON aftermarket_regional (updated_at DESC);

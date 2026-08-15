-- ============================================================================
-- aftermarket_crossref: Curated aftermarket cross-reference table
-- Built from RockAuto scrapes + build_crossref script
-- Run against SQLite hk_parts.db (local data, NOT MySQL/TecDoc)
-- ============================================================================

CREATE TABLE IF NOT EXISTS aftermarket_crossref (
    oem_number  TEXT NOT NULL,
    brand       TEXT NOT NULL,
    part_number TEXT NOT NULL,
    description TEXT,
    category    TEXT,
    verified    INTEGER DEFAULT 1,
    PRIMARY KEY (oem_number, brand, part_number)
);

CREATE INDEX IF NOT EXISTS idx_am_oem   ON aftermarket_crossref(oem_number);
CREATE INDEX IF NOT EXISTS idx_am_brand ON aftermarket_crossref(brand);

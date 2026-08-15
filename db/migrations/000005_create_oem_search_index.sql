CREATE TABLE IF NOT EXISTS oem_search_index (
    id                BIGSERIAL PRIMARY KEY,
    raw_number        TEXT NOT NULL,
    normalized        TEXT NOT NULL,
    legacy_article_id INTEGER NOT NULL,
    source_table      TEXT NOT NULL CHECK (source_table IN ('oemnumbers', 'oem_number', 'articlecrosses')),
    mfr_name          TEXT,
    brand_name        TEXT,
    article_number    TEXT,
    description       TEXT
);

CREATE INDEX IF NOT EXISTS idx_oem_search_index_normalized
    ON oem_search_index (normalized);

CREATE INDEX IF NOT EXISTS idx_oem_search_index_raw
    ON oem_search_index (raw_number);

CREATE INDEX IF NOT EXISTS idx_oem_search_index_article
    ON oem_search_index (legacy_article_id);

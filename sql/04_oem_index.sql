-- ============================================================================
-- Phase 1, Step 1.4: Normalized OEM number search index
-- Combines oemnumbers + oem_number + articlecrosses into one searchable table
-- with normalized part numbers (stripped of dashes, spaces, lowercased)
-- Run against dev_ifritah
-- ============================================================================

DROP TABLE IF EXISTS oem_search_index;

CREATE TABLE oem_search_index (
    id               INT AUTO_INCREMENT PRIMARY KEY,
    raw_number       VARCHAR(100) NOT NULL,       -- original format
    normalized       VARCHAR(100) NOT NULL,       -- stripped: no dashes, spaces, lowercase
    legacyArticleId  INT NOT NULL,
    source_table     ENUM('oemnumbers','oem_number','articlecrosses') NOT NULL,
    mfr_name         VARCHAR(200) DEFAULT NULL,   -- OEM manufacturer name
    brand_name       VARCHAR(200) DEFAULT NULL,   -- aftermarket brand name
    article_number   VARCHAR(100) DEFAULT NULL,   -- aftermarket article number
    description      VARCHAR(500) DEFAULT NULL,

    INDEX idx_normalized (normalized),
    INDEX idx_raw (raw_number),
    INDEX idx_article (legacyArticleId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Source 1: oemnumbers (1.9M rows) — OEM references per aftermarket article
INSERT INTO oem_search_index (raw_number, normalized, legacyArticleId, source_table, mfr_name, article_number, description)
SELECT
    oem.articleNumber,
    LOWER(REPLACE(REPLACE(REPLACE(REPLACE(oem.articleNumber, '-', ''), ' ', ''), '.', ''), '/', '')),
    oem.legacyArticleId,
    'oemnumbers',
    m.manuName,
    a.articleNumber,
    a.genericArticleDescription
FROM oemnumbers oem
JOIN articles a ON a.legacyArticleId = oem.legacyArticleId
LEFT JOIN manufacturers m ON m.manuId = oem.mfrId AND m.linkingTargetType = 'P'
WHERE oem.lang = 'en';

-- Source 2: oem_number (9.6M rows) — another OEM reference table
INSERT INTO oem_search_index (raw_number, normalized, legacyArticleId, source_table, article_number, description)
SELECT
    o.number,
    LOWER(REPLACE(REPLACE(REPLACE(REPLACE(o.number, '-', ''), ' ', ''), '.', ''), '/', '')),
    o.articleId,
    'oem_number',
    a.articleNumber,
    a.genericArticleDescription
FROM oem_number o
JOIN articles a ON a.legacyArticleId = o.articleId;

-- Source 3: articlecrosses (30M rows) — cross-reference table
INSERT INTO oem_search_index (raw_number, normalized, legacyArticleId, source_table, mfr_name, brand_name, article_number, description)
SELECT
    ac.oemNumber,
    LOWER(REPLACE(REPLACE(REPLACE(REPLACE(ac.oemNumber, '-', ''), ' ', ''), '.', ''), '/', '')),
    ac.legacyArticleId,
    'articlecrosses',
    m.manuName,
    ac.brandName,
    ac.number,
    a.genericArticleDescription
FROM articlecrosses ac
JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
LEFT JOIN manufacturers m ON m.manuId = ac.mfrId AND m.linkingTargetType = 'P';

-- Verify
SELECT source_table, COUNT(*) AS cnt FROM oem_search_index GROUP BY source_table;

-- Test: search for a Hyundai oil filter OEM number with different formats
-- All of these should return the same results:
-- SELECT * FROM oem_search_index WHERE normalized = LOWER(REPLACE(REPLACE('26300-35503', '-', ''), ' ', '')) LIMIT 5;

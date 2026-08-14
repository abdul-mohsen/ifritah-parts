-- ============================================================================
-- Phase 1, Step 1.3: NHTSA → TecDoc bridge table
-- Maps NHTSA vPIC naming to TecDoc linkageTargetId
-- Run AFTER 01_create_cache.sql (uses hk_parts_cache)
-- ============================================================================

DROP TABLE IF EXISTS nhtsa_tecdoc_bridge;

CREATE TABLE nhtsa_tecdoc_bridge (
    id                INT AUTO_INCREMENT PRIMARY KEY,
    nhtsa_make        VARCHAR(100) NOT NULL,       -- e.g. 'HYUNDAI'
    nhtsa_model       VARCHAR(100) NOT NULL,       -- e.g. 'TUCSON'
    year_from         INT NOT NULL,
    year_to           INT NOT NULL,
    tecdoc_manu_id    INT NOT NULL,                -- 183 or 184
    tecdoc_model_id   INT NOT NULL,                -- modelseries.modelId
    tecdoc_model_name VARCHAR(200) DEFAULT NULL,

    INDEX idx_lookup (nhtsa_make, nhtsa_model, year_from, year_to),
    INDEX idx_tecdoc (tecdoc_manu_id, tecdoc_model_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Auto-populate from TecDoc modelseries data
-- Maps each TecDoc model to NHTSA naming (NHTSA uses uppercase, same names mostly)
INSERT INTO nhtsa_tecdoc_bridge (nhtsa_make, nhtsa_model, year_from, year_to, tecdoc_manu_id, tecdoc_model_id, tecdoc_model_name)
SELECT
    CASE ms.manuId
        WHEN 183 THEN 'HYUNDAI'
        WHEN 184 THEN 'KIA'
    END AS nhtsa_make,
    UPPER(ms.modelname) AS nhtsa_model,
    COALESCE(FLOOR(ms.start_year / 100), 1990) AS year_from,
    COALESCE(FLOOR(ms.end_year / 100), 2030) AS year_to,
    ms.manuId,
    ms.modelId,
    ms.modelname
FROM modelseries ms
WHERE ms.manuId IN (183, 184)
  AND ms.linkingTargetType = 'P';

-- Verify
SELECT nhtsa_make, COUNT(*) AS models FROM nhtsa_tecdoc_bridge GROUP BY nhtsa_make;
SELECT * FROM nhtsa_tecdoc_bridge WHERE nhtsa_model LIKE '%TUCSON%';

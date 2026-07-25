-- ============================================================================
-- Phase 1, Step 1.1: Pre-computed Hyundai/Kia parts cache
-- Run against dev_ifritah on the MySQL server
-- This query will take a while (scanning 274M rows) but only runs ONCE
-- ============================================================================

-- Drop if re-running
DROP TABLE IF EXISTS hk_parts_cache;

CREATE TABLE hk_parts_cache (
    linkingTargetId      INT NOT NULL,
    legacyArticleId      INT NOT NULL,
    assemblyGroupNodeId  INT NOT NULL DEFAULT 0,
    articleNumber        VARCHAR(100) DEFAULT NULL,
    genericArticleDesc   VARCHAR(500) DEFAULT NULL,
    dataSupplierId       INT DEFAULT NULL,
    brandName            VARCHAR(200) DEFAULT NULL,
    categoryName         VARCHAR(300) DEFAULT NULL,
    vehicleDesc          VARCHAR(500) DEFAULT NULL,
    manuId               INT DEFAULT NULL,
    modelId              INT DEFAULT NULL,
    modelName            VARCHAR(200) DEFAULT NULL,
    beginYearMonth       VARCHAR(20) DEFAULT NULL,
    endYearMonth         VARCHAR(20) DEFAULT NULL,
    fuelType             VARCHAR(50) DEFAULT NULL,
    capacityCC           INT DEFAULT NULL,
    horsePowerFrom       INT DEFAULT NULL,

    PRIMARY KEY (linkingTargetId, legacyArticleId, assemblyGroupNodeId),
    INDEX idx_article (legacyArticleId),
    INDEX idx_model (manuId, modelId),
    INDEX idx_brand (dataSupplierId),
    INDEX idx_category (assemblyGroupNodeId),
    INDEX idx_article_number (articleNumber)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Populate: ETL from articlesvehicletrees → articles → brand → category → vehicle
-- Filter to Hyundai (183) and Kia (184) passenger cars only
INSERT INTO hk_parts_cache
SELECT
    avt.linkingTargetId,
    avt.legacyArticleId,
    COALESCE(avt.assemblyGroupNodeId, 0),
    a.articleNumber,
    a.genericArticleDescription,
    a.dataSupplierId,
    COALESCE(ab.en_brand, ab.any_brand),
    COALESCE(gag.en_name, gag.any_name),
    lt.description,
    ms.manuId,
    ms.modelId,
    ms.modelname,
    lt.beginYearMonth,
    lt.endYearMonth,
    lt.fuelType,
    lt.capacityCC,
    lt.horsePowerFrom
FROM articlesvehicletrees avt
JOIN linkagetargets lt ON lt.linkageTargetId = avt.linkingTargetId AND lt.lang = 'en'
JOIN modelseries ms ON ms.modelId = lt.vehicleModelSeriesId AND ms.linkingTargetType = 'P'
JOIN articles a ON a.legacyArticleId = avt.legacyArticleId
LEFT JOIN (
    SELECT brandId,
           MIN(CASE WHEN lang = 'en' THEN brandName END) AS en_brand,
           MIN(brandName) AS any_brand
    FROM ambrand
    GROUP BY brandId
) ab ON ab.brandId = a.dataSupplierId
LEFT JOIN legacy2generic l2g ON l2g.legacyArticleId = avt.legacyArticleId
LEFT JOIN (
    SELECT genericArticleId,
           MIN(CASE WHEN lang = 'en' THEN masterDesignation END) AS en_name,
           MIN(masterDesignation) AS any_name
    FROM genericarticlesgroups
    GROUP BY genericArticleId
) gag ON gag.genericArticleId = l2g.genericArticleId
WHERE avt.linkingTargetType = 'P'
  AND ms.manuId IN (183, 184);

-- Verify
SELECT COUNT(*) AS total_cached_rows FROM hk_parts_cache;
SELECT manuId, COUNT(DISTINCT legacyArticleId) AS unique_parts, COUNT(DISTINCT linkingTargetId) AS vehicles
FROM hk_parts_cache
GROUP BY manuId;

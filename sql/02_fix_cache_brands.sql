-- ============================================================================
-- Fix hk_parts_cache: populate brandName and categoryName
-- brandName was NULL because it used mfrId (which is 0 for 99% of articles)
-- categoryName was NULL because assemblygroupnodenames is empty
-- ============================================================================

-- Step 1: Rename mfrId column to dataSupplierId
ALTER TABLE hk_parts_cache CHANGE mfrId dataSupplierId INT DEFAULT NULL;

-- Step 2: Populate dataSupplierId from articles table
UPDATE hk_parts_cache hk
JOIN articles a ON a.legacyArticleId = hk.legacyArticleId
SET hk.dataSupplierId = a.dataSupplierId;

-- Step 3: Populate brandName using dataSupplierId → ambrand
UPDATE hk_parts_cache hk
JOIN articles a ON a.legacyArticleId = hk.legacyArticleId
JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
SET hk.brandName = ab.brandName;

-- Step 4: Populate categoryName from genericarticlesgroups via legacy2generic
UPDATE hk_parts_cache hk
SET hk.categoryName = (
    SELECT gag.masterDesignation
    FROM legacy2generic l2g
    JOIN genericarticlesgroups gag ON gag.genericArticleId = l2g.genericArticleId AND gag.lang = 'en'
    WHERE l2g.legacyArticleId = hk.legacyArticleId
    LIMIT 1
)
WHERE hk.categoryName IS NULL OR hk.categoryName = '';

-- Verify
SELECT 'Brand fix results' AS section;
SELECT
    COUNT(*) AS total,
    SUM(brandName IS NOT NULL AND brandName != '') AS has_brand,
    ROUND(100.0 * SUM(brandName IS NOT NULL AND brandName != '') / COUNT(*), 1) AS pct_brand,
    SUM(categoryName IS NOT NULL AND categoryName != '') AS has_category,
    ROUND(100.0 * SUM(categoryName IS NOT NULL AND categoryName != '') / COUNT(*), 1) AS pct_category
FROM hk_parts_cache;

SELECT brandName, COUNT(*) AS cnt
FROM hk_parts_cache
WHERE brandName IS NOT NULL AND brandName != ''
GROUP BY brandName
ORDER BY cnt DESC
LIMIT 20;

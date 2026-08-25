-- ============================================================================
-- M0.T4 - vehicle_fitment diagnosis
-- ============================================================================
-- Two symptoms to explain:
--   S1: /api/catalog/vehicles?make=Hyundai&model=Elantra returns total=0
--   S2: vehicle_fitment mode returns 0 hits when passed a random linkageTargetId
--
-- Hypothesis: the join in catalog handler + strategy uses a filter (lang='en',
-- linkingTargetType='P') that eliminates most rows, OR the modelseries table
-- doesn't cover HK.
--
-- Run against qa MySQL (TecDoc read replica).
-- ============================================================================

-- ── 1. Are Hyundai / Kia manufacturers present? ────────────────────────────

SELECT manuId, manuName, linkingTargetType
FROM manufacturers
WHERE manuName IN ('Hyundai', 'Kia', 'HYUNDAI', 'KIA', 'HYUNDAI/KIA')
   OR manuName LIKE 'Hyundai%'
   OR manuName LIKE 'Kia%';

-- ── 2. Model series for Hyundai (the LEFT side of the /api/catalog join) ──

SELECT
  ms.modelId,
  ms.manuId,
  ms.modelname,
  ms.linkingTargetType,
  COUNT(lt.linkageTargetId) AS linkage_count
FROM modelseries ms
LEFT JOIN linkagetargets lt ON lt.vehicleModelSeriesId = ms.modelId
WHERE ms.manuId IN (SELECT manuId FROM manufacturers WHERE manuName LIKE '%Hyundai%')
  AND ms.linkingTargetType = 'P'
GROUP BY ms.modelId, ms.manuId, ms.modelname, ms.linkingTargetType
ORDER BY linkage_count DESC
LIMIT 20;

-- ── 3. Direct sample of linkagetargets ("linkageTargetIds we could pass" ) ─

SELECT
  lt.linkageTargetId,
  lt.description,
  lt.fuelType,
  lt.capacityCC,
  lt.beginYearMonth,
  lt.endYearMonth,
  m.manuName
FROM linkagetargets lt
JOIN modelseries ms ON lt.vehicleModelSeriesId = ms.modelId
JOIN manufacturers m ON m.manuId = ms.manuId
WHERE m.manuName LIKE '%Hyundai%'
  AND ms.modelname LIKE '%Elantra%'
  AND lt.lang = 'en'
LIMIT 10;

-- ── 4. Given the linkageTargetIds from section 3, how many parts fit them? ─
--    Fill in <ID_LIST> from section 3.

/* -- UNCOMMENT AND FILL AFTER SECTION 3 --
SELECT
  avt.linkingTargetId,
  COUNT(DISTINCT a.legacyArticleId) AS parts_count,
  MIN(agn.assemblyGroupName)         AS sample_category
FROM articlesvehicletrees avt
JOIN articles a ON a.legacyArticleId = avt.legacyArticleId
LEFT JOIN assemblygroupnodenames agn ON agn.assemblyGroupNodeId = avt.assemblyGroupNodeId AND agn.lang = 'en'
WHERE avt.linkingTargetId IN (<ID_LIST FROM SECTION 3>)
  AND avt.linkingTargetType = 'P'
GROUP BY avt.linkingTargetId
ORDER BY parts_count DESC;
*/

-- ── 5. Check languages available for linkagetargets (H: only non-en?) ──────

SELECT lang, COUNT(*)
FROM linkagetargets
GROUP BY lang
ORDER BY COUNT(*) DESC;

-- ── 6. What linkingTargetType values exist? ────────────────────────────────
--    The strategy filters on 'P' (Passenger). If HK data uses a different
--    code, that filter drops everything.

SELECT linkingTargetType, COUNT(*)
FROM articlesvehicletrees
GROUP BY linkingTargetType
ORDER BY COUNT(*) DESC;

-- ============================================================================
-- Interpretation
-- ============================================================================
--
-- Case A: section 1 returns 0 rows.
--   Cause: TecDoc dump on qa doesn't include Hyundai/Kia manufacturers.
--   Fix: verify the dump import; may need to re-import a fuller TecDoc
--        catalog for HK coverage.
--
-- Case B: section 3 returns rows, section 4 returns rows for those IDs.
--   Cause: /api/catalog/vehicles endpoint has a bug — its query filter
--          is eliminating rows the underlying data has. Trace
--          internal/handler/catalog.go:Vehicles.
--   Fix: relax filter until section 3 shape flows through the endpoint.
--
-- Case C: section 5 shows lang column values that ARE NOT 'en' (e.g.
--         'de', 'fr' only).
--   Cause: TecDoc dump uses non-English language codes; our query
--          hardcodes lang='en'.
--   Fix: fall back to any language when 'en' returns 0.
--
-- Case D: section 6 shows linkingTargetType values other than 'P'
--         (e.g. 'V' for vehicle, 'C' for commercial).
--   Cause: HK dumps use a different type code.
--   Fix: accept 'P' OR 'V' in the strategy query.

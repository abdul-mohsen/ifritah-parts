-- ============================================================================
-- M0.T2 - supersession diagnosis
-- ============================================================================
-- Purpose: figure out why `supersession` mode returns 0 hits for every
-- tested HK OEM. Three hypotheses:
--
--   H1: replacedbyarticles + replacesarticles tables are EMPTY on qa
--       (TecDoc dump doesn't ship HK data)
--   H2: Tables populated but the JOIN filter (UPPER(articleNumber) match)
--       fails for HK numbers with spaces / dashes
--   H3: Article-id promotion at strategy entry (line 605
--       st.search.oem.Search(req.OEM, 5)) returns 0 refs for these OEMs
--
-- Run against qa MySQL (TecDoc read replica). Paste output into
-- docs/data-sources/supersession-diagnosis.md.
-- ============================================================================

-- ── 1. Table sizes (falsifies H1) ──────────────────────────────────────────

SELECT
  (SELECT COUNT(*) FROM replacedbyarticles)      AS replacedby_rows,
  (SELECT COUNT(*) FROM replacesarticles)         AS replaces_rows,
  (SELECT COUNT(DISTINCT legacyArticleId) FROM replacedbyarticles) AS distinct_source_articles;

-- ── 2. Article-id resolution for corpus OEMs (via oem_number path) ─────────
--    Every OEM must resolve to a legacyArticleId for the strategy to work.

SELECT
  on2.clean_number,
  a.legacyArticleId,
  a.articleNumber,
  a.genericArticleDescription
FROM oem_number on2
LEFT JOIN articles a ON a.legacyArticleId = on2.articleId
WHERE on2.clean_number IN (
  '263502j001',
  '581013xa00',
  '97133d3000',
  '824602t010',
  '273012e400'
)
ORDER BY on2.clean_number;

-- ── 3. Do those resolved articleIds have supersession rows? ────────────────
--    Replace <ID_LIST> with the legacyArticleIds from section 2.
--    Example: WHERE legacyArticleId IN (6103, 44521, 88012, ...)

/* -- UNCOMMENT AND FILL IN AFTER SECTION 2 --
SELECT
  legacyArticleId,
  COUNT(*) AS replaced_by_count,
  GROUP_CONCAT(articleNumber SEPARATOR ',') AS successor_numbers
FROM replacedbyarticles
WHERE legacyArticleId IN (<ID_LIST FROM SECTION 2>)
GROUP BY legacyArticleId;
*/

-- ── 4. Sample a KNOWN active supersession chain (falsifies H2) ─────────────
--    Grab any 5 legacyArticleIds that DO have supersession rows.
--    If the strategy can resolve THESE, the code works and the issue is
--    just data coverage on HK OEMs (H1 partially).

SELECT
  rba.legacyArticleId,
  a.articleNumber                                     AS current_article,
  a.genericArticleDescription                         AS current_desc,
  rba.articleNumber                                   AS successor_article,
  COALESCE(ab.brandName, '')                          AS successor_brand
FROM replacedbyarticles rba
LEFT JOIN articles a ON a.legacyArticleId = rba.legacyArticleId
LEFT JOIN articles a2 ON UPPER(a2.articleNumber) = UPPER(rba.articleNumber) AND a2.mfrId = rba.mfrId
LEFT JOIN ambrand ab ON ab.brandId = a2.dataSupplierId AND ab.lang = 'en'
LIMIT 5;

-- ── 5. Are HK-brand supersession rows populated? ───────────────────────────
--    dataSupplierId + ambrand.brandName should include Hyundai / Kia / Mobis
--    on both sides of the chain.

SELECT
  COALESCE(ab.brandName, '<null>') AS brand,
  COUNT(*) AS rows
FROM replacedbyarticles rba
LEFT JOIN articles a ON a.legacyArticleId = rba.legacyArticleId
LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
GROUP BY brand
ORDER BY rows DESC
LIMIT 20;

-- ============================================================================
-- Interpretation
-- ============================================================================
--
-- Case A: section 1 shows both tables empty (< 1000 rows).
--   Cause: TecDoc dump doesn't ship supersession for HK.
--   Fix: need a different data source. Follow up in M4 (dealer catalog).
--
-- Case B: sections 1 + 4 healthy but section 3 empty for our OEMs.
--   Cause: HK OEMs don't have supersession entries — data coverage gap.
--   Fix: expand test corpus to only OEMs that DO have chain rows;
--        supersession strategy is fine, just track per-category coverage.
--
-- Case C: sections 1 + 4 healthy, section 3 populated but strategy still 0.
--   Cause: strategy code bug — likely in the JOIN filter or article-id
--          promotion at strategy.go:605.
--   Fix: trace SupersessionStrategy.Search() against a section-4 case
--        and identify where hits are dropped.

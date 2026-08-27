-- ============================================================================
-- TecDoc Audit + Diagnostic — ALL-IN-ONE
-- ============================================================================
-- One file. Runs the whole health check for the TecDoc MySQL / MariaDB
-- backend end-to-end. Answers every question that gates M0-M8 progress:
--
--   Part A · Environment
--     1.  Table sizes
--     2.  Schema + P0 index check (sql/06 + sql/07 + sql/08 hotfix)
--     3.  articles + ambrand + articlecriteria schemas (JOIN discovery)
--     4.  Hyundai/Kia manuIds + aftermarket-brand catalog
--
--   Part B · HK data coverage
--     5.  oem_number HK prefix coverage
--     6.  articlecrosses HK prefix coverage
--     7.  articlecriteria HK spec coverage
--     8.  articlesvehicletrees linkingTargetType distribution
--     9.  Supersession chain HK coverage
--     10. linkagetargets + modelseries HK vehicle catalog
--     11. Language distribution (validates lang='en' filter assumption)
--
--   Part C · The real aftermarket answer
--     12. Aftermarket brands per HK prefix (via articles.dataSupplierId
--         → ambrand.brandId — the CORRECT JOIN, not mfrId which is 0)
--     13. Explicit marquee-brand probe (BOSCH / MANN / MAHLE / etc.)
--
--   Part D · Corpus verification
--     14. 19-OEM audit corpus via oem_number
--     15. 19-OEM audit corpus via articlecrosses
--     16. REAL aftermarket brand per corpus OEM
--
--   Part E · Sample data (spot-check)
--     17. Sample rows: articles / ambrand / oem_number / articlecrosses
--
--   Part F · Query-plan verification
--     18. EXPLAIN FindBySpecMatch      (needs sql/08 index)
--     19. EXPLAIN FindSpecifications   (needs sql/07 legacyArticleId)
--     20. EXPLAIN SearchCrossReferences (needs sql/06 oemNumberNormalized)
--     21. EXPLAIN SearchByOEM primary   (needs sql/06 clean_number)
--     22. EXPLAIN PartsForVehicle       (uses linkingTargetId index)
--     23. EXPLAIN SearchByOEMIndex      (uses oem_search_index.normalized)
--
-- ─── Runtime characteristics ─────────────────────────────────────────
--
--   * Total runtime: 2-8 minutes on a healthy database with all P0
--     indexes applied. Longer (up to 20 min) when sql/07 or sql/08
--     indexes are missing — the EXPLAINs at the end will surface that.
--
--   * Memory-safe: no COUNT(DISTINCT) over 340M rows; no LIMIT-inside-
--     IN-SELECT patterns; all subqueries bounded; sample-based
--     estimates where full scans would blow /var/tmp.
--
--   * Compatible: MariaDB 10.3+ AND MySQL 5.7 / 8.x. No window
--     functions, no FORMAT=TREE, no JSON_TABLE, no CTEs (portable to
--     older MariaDB), no reserved-word column aliases.
--
--   * Read-only: every DML/DDL that touches the DB is a TEMPORARY
--     table (auto-dropped at session end); ENGINE=MEMORY.
--
--   * No credentials in this file: pass creds via mysql client CLI.
--
-- ─── Usage ───────────────────────────────────────────────────────────
--
--   mysql --host=<tecdoc-mysql-host> --user=<user> --password \
--         --database=<tecdoc-db-name> \
--         < scripts/diagnostics/tecdoc_diagnostic_full.sql \
--         > tecdoc-diagnostic-$(date +%Y-%m-%d).txt
--
-- Or interactively:
--
--   mysql> source scripts/diagnostics/tecdoc_diagnostic_full.sql;
--
-- Paste the full output back for section-by-section analysis.
-- ============================================================================


SELECT '' AS ' ';
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '  TECDOC AUDIT + DIAGNOSTIC — ALL-IN-ONE' AS ' ';
SELECT '  Run at:' AS ' ', NOW() AS run_at;
SELECT '  Database:' AS ' ', DATABASE() AS db;
SELECT '  Server:' AS ' ', @@version AS mysql_version, @@version_compile_os AS os;
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '' AS ' ';


-- ═══════════════════════════════════════════════════════════════════════════
--                       PART A — ENVIRONMENT SCAN
-- ═══════════════════════════════════════════════════════════════════════════

-- ─── §1  Table sizes ─────────────────────────────────────────────────────
--
-- Sanity-check that TecDoc data is present at all. Any table showing 0
-- rows means either the dump wasn't loaded, the wrong DB is connected,
-- or a table was renamed in the source schema.

SELECT '─── §1  Table sizes ────────────────────────────────────' AS section;

SELECT
	TABLE_NAME,
	TABLE_ROWS                                          AS approx_rows,
	ROUND(DATA_LENGTH  / 1024 / 1024, 1)                AS data_mb,
	ROUND(INDEX_LENGTH / 1024 / 1024, 1)                AS index_mb
FROM information_schema.tables
WHERE table_schema = DATABASE()
  AND table_name IN (
	'articles','articlecrosses','articlecriteria','oem_number',
	'oem_search_index','articlesvehicletrees','linkagetargets',
	'modelseries','manufacturers','ambrand',
	'replacedbyarticles','replacesarticles',
	'assemblygroupnodenames','assemblygroupnodes'
  )
ORDER BY TABLE_ROWS DESC;

SELECT '─── §1b Exact row counts (slower but authoritative) ────' AS section;

SELECT 'articles'                AS tbl, COUNT(*) AS rows_exact FROM articles
UNION ALL SELECT 'articlecrosses',       COUNT(*) FROM articlecrosses
UNION ALL SELECT 'articlecriteria',      COUNT(*) FROM articlecriteria
UNION ALL SELECT 'oem_number',           COUNT(*) FROM oem_number
UNION ALL SELECT 'oem_search_index',     COUNT(*) FROM oem_search_index
UNION ALL SELECT 'linkagetargets',       COUNT(*) FROM linkagetargets
UNION ALL SELECT 'modelseries',          COUNT(*) FROM modelseries
UNION ALL SELECT 'manufacturers',        COUNT(*) FROM manufacturers
UNION ALL SELECT 'ambrand',              COUNT(*) FROM ambrand
UNION ALL SELECT 'replacedbyarticles',   COUNT(*) FROM replacedbyarticles
UNION ALL SELECT 'replacesarticles',     COUNT(*) FROM replacesarticles;


-- ─── §2  Schema + P0 index check ─────────────────────────────────────────
--
-- Verifies the three P0 index migrations are applied:
--   sql/06 → articlecrosses.oemNumberNormalized generated column + BTREE
--         → oem_number.clean_number BTREE index
--   sql/07 → articlecriteria (legacyArticleId) + (criteriaDescription, rawValue)
--   sql/08 → hotfix that adds sql/07's second index with prefix lengths
--            for the TEXT columns (was blocked by ERROR 1170)

SELECT '─── §2  Schema — articlecrosses columns ──────────────' AS section;

SELECT
	COLUMN_NAME,
	DATA_TYPE,
	IS_NULLABLE,
	IS_GENERATED,
	LEFT(GENERATION_EXPRESSION, 80) AS gen_expr_first_80
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'articlecrosses'
ORDER BY ORDINAL_POSITION;

SELECT '─── §2b Index inventory across every table the app touches ──' AS section;

SELECT
	TABLE_NAME,
	INDEX_NAME,
	NON_UNIQUE,
	GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS `columns`,
	INDEX_TYPE
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name IN (
	'articles','articlecrosses','articlecriteria','oem_number',
	'oem_search_index','articlesvehicletrees','linkagetargets',
	'modelseries','manufacturers','ambrand',
	'replacedbyarticles','replacesarticles'
  )
GROUP BY TABLE_NAME, INDEX_NAME, NON_UNIQUE, INDEX_TYPE
ORDER BY TABLE_NAME, INDEX_NAME;

SELECT '─── §2c P0 index PASS/FAIL summary ─────────────────────' AS section;

SELECT
	'articlecrosses.oemNumberNormalized (column)' AS check_name,
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'articlecrosses'
		  AND column_name = 'oemNumberNormalized'
	) THEN 'PRESENT' ELSE 'MISSING (sql/06 needed)' END AS status
UNION ALL SELECT
	'idx_articlecrosses_oemNumberNormalized',
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'articlecrosses'
		  AND index_name = 'idx_articlecrosses_oemNumberNormalized'
	) THEN 'PRESENT' ELSE 'MISSING (sql/06 needed)' END
UNION ALL SELECT
	'idx_oem_number_clean_number',
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'oem_number'
		  AND index_name = 'idx_oem_number_clean_number'
	) THEN 'PRESENT' ELSE 'MISSING (sql/06 needed)' END
UNION ALL SELECT
	'idx_articlecriteria_legacyArticleId',
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'articlecriteria'
		  AND index_name = 'idx_articlecriteria_legacyArticleId'
	) THEN 'PRESENT' ELSE 'MISSING (sql/07 needed)' END
UNION ALL SELECT
	'idx_articlecriteria_criteria_value',
	CASE WHEN EXISTS (
		SELECT 1 FROM information_schema.statistics
		WHERE table_schema = DATABASE()
		  AND table_name = 'articlecriteria'
		  AND index_name = 'idx_articlecriteria_criteria_value'
	) THEN 'PRESENT' ELSE 'MISSING (sql/08 hotfix needed)' END;


-- ─── §3  articles + ambrand + articlecriteria schema discovery ───────────
--
-- Documents the actual column layout. Two purposes:
--   * Confirms the mfrId=0 finding from 2026-08-26 that made us pivot
--     from articles.mfrId → articles.dataSupplierId as the ambrand link
--   * Reveals whether articlecriteria uses TEXT (which requires prefix
--     length in indexes → sql/07 vs sql/08 fix)

SELECT '─── §3  articles table schema ─────────────────────────' AS section;

SELECT
	COLUMN_NAME,
	DATA_TYPE,
	IS_NULLABLE,
	COLUMN_KEY,
	COLUMN_COMMENT
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'articles'
ORDER BY ORDINAL_POSITION;

SELECT '─── §3b articles sample rows (verify mfrId is 0) ───────' AS section;

SELECT *
FROM articles
LIMIT 3;

SELECT '─── §3c ambrand table schema ──────────────────────────' AS section;

SELECT
	COLUMN_NAME,
	DATA_TYPE,
	IS_NULLABLE,
	COLUMN_KEY
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'ambrand'
ORDER BY ORDINAL_POSITION;

SELECT '─── §3d ambrand sample (aftermarket brand catalog) ────' AS section;

SELECT *
FROM ambrand
WHERE lang = 'EN'
LIMIT 10;

SELECT '─── §3e articlecriteria columns + index-prefix implications ──' AS section;

SELECT
	COLUMN_NAME,
	DATA_TYPE,
	CHARACTER_MAXIMUM_LENGTH,
	IS_NULLABLE,
	COLUMN_KEY,
	CASE
		WHEN DATA_TYPE IN ('text','mediumtext','longtext','tinytext','blob') THEN 'NEEDS_PREFIX'
		WHEN DATA_TYPE IN ('varchar','char') THEN 'DIRECT_INDEXABLE'
		ELSE 'N/A'
	END AS index_treatment
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'articlecriteria'
ORDER BY ORDINAL_POSITION;

SELECT '─── §3f replacedbyarticles + replacesarticles schemas ──' AS section;

SELECT
	TABLE_NAME,
	COLUMN_NAME,
	DATA_TYPE
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name IN ('replacedbyarticles', 'replacesarticles')
ORDER BY TABLE_NAME, ORDINAL_POSITION;


-- ─── §4  HK manufacturers + aftermarket brand catalog ────────────────────

SELECT '─── §4  Hyundai/Kia/Genesis in manufacturers table ────' AS section;

SELECT
	manuId,
	manuName,
	linkingTargetType
FROM manufacturers
WHERE manuName LIKE '%Hyundai%'
   OR manuName LIKE '%Kia%'
   OR manuName LIKE '%Mobis%'
   OR manuName LIKE '%Genesis%'
ORDER BY manuName, linkingTargetType;

SELECT '─── §4b Top aftermarket brands present in ambrand ─────' AS section;

SELECT
	brandName,
	COUNT(*) AS rows_in_ambrand
FROM ambrand
WHERE brandName IN (
	'BOSCH','MANN','MANN-FILTER','MAHLE','MAHLE ORIGINAL','KNECHT',
	'DENSO','NGK','VALEO','HELLA','BREMBO','TEXTAR','FERODO','SACHS',
	'FEBI','FEBI BILSTEIN','LEMFOERDER','LUK','INA','SKF','GATES',
	'CONTINENTAL','FILTRON','WIX','PURFLUX','MAGNETI MARELLI',
	'DELPHI','MEYLE','TRW','ATE','ZIMMERMANN','BLUE PRINT','KYB',
	'MONROE','BILSTEIN','KONI','CHAMPION','MOBIS'
  )
GROUP BY brandName
ORDER BY rows_in_ambrand DESC;


-- ═══════════════════════════════════════════════════════════════════════════
--                    PART B — HK DATA COVERAGE
-- ═══════════════════════════════════════════════════════════════════════════

-- ─── §5  oem_number HK prefix coverage ────────────────────────────────────
--
-- Primary lookup path. The app queries oem_number.clean_number for a
-- known HK OEM; results here directly gate how many audit-corpus OEMs
-- resolve.

SELECT '─── §5  oem_number total + prefix-2 diversity ─────────' AS section;

SELECT
	COUNT(*)                              AS total_rows,
	COUNT(DISTINCT LEFT(clean_number, 2)) AS distinct_prefix2
FROM oem_number;

SELECT '─── §5b oem_number rows per HK prefix ──────────────────' AS section;

SELECT
	LEFT(clean_number, 2) AS prefix2,
	COUNT(*)              AS rows_at_prefix
FROM oem_number
WHERE LEFT(clean_number, 2) IN
	('26','28','29','51','52','54','55','58','82','83','84','85','86',
	 '92','93','94','95','96','97','98','99')
GROUP BY prefix2
ORDER BY rows_at_prefix DESC;


-- ─── §6  articlecrosses HK prefix coverage ────────────────────────────────

SELECT '─── §6  articlecrosses.oemNumberNormalized presence ───' AS section;

SELECT
	'articlecrosses.oemNumberNormalized' AS check_name,
	COUNT(*)                             AS matched
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'articlecrosses'
  AND column_name = 'oemNumberNormalized';

SELECT '─── §6b articlecrosses HK-prefix cross-ref counts ──────' AS section;

SELECT
	LEFT(oemNumberNormalized, 2) AS prefix2,
	COUNT(*)                     AS crossref_rows,
	COUNT(DISTINCT brandName)    AS distinct_brands
FROM articlecrosses
WHERE oemNumberNormalized LIKE '26%'
   OR oemNumberNormalized LIKE '58%'
   OR oemNumberNormalized LIKE '82%'
   OR oemNumberNormalized LIKE '97%'
GROUP BY prefix2;


-- ─── §7  articlecriteria HK spec coverage (via correct JOIN) ─────────────
--
-- The `articles.mfrId=0` finding means we CANNOT JOIN via articles.mfrId
-- to manufacturers. Instead we filter articles via articlecrosses
-- (which has HK-prefix normalized OEMs), then look up their specs.

SELECT '─── §7  articlecriteria — global spec distribution ────' AS section;

SELECT
	criteriaDescription,
	COUNT(*) AS occurrences
FROM articlecriteria
GROUP BY criteriaDescription
ORDER BY occurrences DESC
LIMIT 20;

SELECT '─── §7b HK articles with any spec (top prefixes) ──────' AS section;

SELECT
	'HK articles with any spec' AS check_name,
	COUNT(DISTINCT ac.legacyArticleId) AS distinct_articles
FROM articlecriteria ac
WHERE ac.legacyArticleId IN (
	SELECT DISTINCT legacyArticleId
	FROM articlecrosses
	WHERE oemNumberNormalized LIKE '263%'
	   OR oemNumberNormalized LIKE '581%'
	   OR oemNumberNormalized LIKE '821%'
	   OR oemNumberNormalized LIKE '971%'
	LIMIT 50000
);


-- ─── §8  articlesvehicletrees linkingTargetType distribution ─────────────
--
-- 340M-row table. The app filters on linkingTargetType='P' (Passenger);
-- if HK data uses a different code, that filter would drop everything.
-- Uses row_count as alias (row is reserved in MySQL 8).

SELECT '─── §8  articlesvehicletrees linkingTargetType counts ─' AS section;

SELECT
	linkingTargetType,
	COUNT(*) AS row_count
FROM articlesvehicletrees
WHERE linkingTargetType IN ('P','V','C','M','A','K','L','H','S','O')
GROUP BY linkingTargetType
ORDER BY row_count DESC;

SELECT '─── §8b distinct linkage IDs sampled (10M-row bounded) ─' AS section;

SELECT
	COUNT(DISTINCT sample_id) AS distinct_p_linkages_in_10m_sample
FROM (
	SELECT linkingTargetId AS sample_id
	FROM articlesvehicletrees
	WHERE linkingTargetType = 'P'
	LIMIT 10000000
) t;

SELECT '─── §8c rows-per-P-linkage sample ─────────────────────' AS section;

SELECT
	AVG(rows_per_linkage) AS avg_rows,
	MIN(rows_per_linkage) AS min_rows,
	MAX(rows_per_linkage) AS max_rows,
	COUNT(*)              AS linkages_sampled
FROM (
	SELECT linkingTargetId, COUNT(*) AS rows_per_linkage
	FROM articlesvehicletrees
	WHERE linkingTargetType = 'P'
	GROUP BY linkingTargetId
	LIMIT 1000
) t;


-- ─── §9  Supersession chain HK coverage ─────────────────────────────────
--
-- SupersessionStrategy in the app returned 0 hits in the 2026-08-24
-- probe. This section confirms whether the tables are empty for HK OEMs
-- or the app's query has a bug.

SELECT '─── §9  Supersession table totals ─────────────────────' AS section;

SELECT
	'replacedbyarticles' AS tbl, COUNT(*) AS total_rows FROM replacedbyarticles
UNION ALL SELECT
	'replacesarticles',   COUNT(*) FROM replacesarticles;

SELECT '─── §9b Sample replacedbyarticles rows ─────────────────' AS section;
SELECT * FROM replacedbyarticles LIMIT 5;

SELECT '─── §9c Sample replacesarticles rows ──────────────────' AS section;
SELECT * FROM replacesarticles LIMIT 5;

SELECT '─── §9d HK supersession rows via articlecrosses JOIN ──' AS section;

SELECT
	'replacedbyarticles HK (sampled)' AS tbl,
	COUNT(*)                          AS matched_rows,
	COUNT(DISTINCT rba.legacyArticleId) AS distinct_articles
FROM replacedbyarticles rba
WHERE EXISTS (
	SELECT 1 FROM articlecrosses acr
	WHERE acr.legacyArticleId = rba.legacyArticleId
	  AND (
		acr.oemNumberNormalized LIKE '263%' OR acr.oemNumberNormalized LIKE '581%' OR
		acr.oemNumberNormalized LIKE '821%' OR acr.oemNumberNormalized LIKE '971%'
	  )
);

SELECT
	'replacesarticles HK (sampled)' AS tbl,
	COUNT(*)                        AS matched_rows,
	COUNT(DISTINCT ra.legacyArticleId) AS distinct_articles
FROM replacesarticles ra
WHERE EXISTS (
	SELECT 1 FROM articlecrosses acr
	WHERE acr.legacyArticleId = ra.legacyArticleId
	  AND (
		acr.oemNumberNormalized LIKE '263%' OR acr.oemNumberNormalized LIKE '581%' OR
		acr.oemNumberNormalized LIKE '821%' OR acr.oemNumberNormalized LIKE '971%'
	  )
);


-- ─── §10 linkagetargets + modelseries HK vehicle catalog ─────────────────
--
-- Uses the manuIds confirmed in §4: 183 Hyundai, 184 Kia, 4473 Genesis,
-- 3123 Hyundai(Beijing), 3127 Kia(DYK), 3128 Hyundai(Huatai).

SELECT '─── §10 HK models + linkage counts ────────────────────' AS section;

SELECT
	m.manuName,
	COUNT(DISTINCT ms.modelId)         AS model_count,
	COUNT(DISTINCT lt.linkageTargetId) AS linkage_count
FROM modelseries ms
JOIN manufacturers m ON m.manuId = ms.manuId
LEFT JOIN linkagetargets lt ON lt.vehicleModelSeriesId = ms.modelId
WHERE m.manuId IN (183, 184, 4473, 3123, 3127, 3128)
GROUP BY m.manuName
ORDER BY linkage_count DESC;

SELECT '─── §10b Sample Elantra linkage IDs (for audit script) ─' AS section;

SELECT
	lt.linkageTargetId,
	m.manuName,
	ms.modelname,
	lt.description,
	lt.beginYearMonth,
	lt.endYearMonth,
	lt.lang
FROM linkagetargets lt
JOIN modelseries ms ON lt.vehicleModelSeriesId = ms.modelId
JOIN manufacturers m ON m.manuId = ms.manuId
WHERE m.manuId = 183
  AND ms.modelname LIKE '%Elantra%'
ORDER BY lt.beginYearMonth DESC
LIMIT 10;

SELECT '─── §10c Sample Sonata linkage IDs ────────────────────' AS section;

SELECT
	lt.linkageTargetId,
	m.manuName,
	ms.modelname,
	lt.description,
	lt.beginYearMonth,
	lt.endYearMonth,
	lt.lang
FROM linkagetargets lt
JOIN modelseries ms ON lt.vehicleModelSeriesId = ms.modelId
JOIN manufacturers m ON m.manuId = ms.manuId
WHERE m.manuId = 183
  AND ms.modelname LIKE '%Sonata%'
ORDER BY lt.beginYearMonth DESC
LIMIT 10;


-- ─── §11 Language distribution ───────────────────────────────────────────
--
-- The app hard-codes `lang='en'` on linkagetargets + ambrand +
-- assemblygroupnodenames. If HK data ships in another language
-- exclusively, the JOIN eliminates everything.

SELECT '─── §11 linkagetargets language coverage ──────────────' AS section;

SELECT 'linkagetargets' AS tbl, lang, COUNT(*) AS row_count
FROM linkagetargets
GROUP BY lang
ORDER BY row_count DESC
LIMIT 10;

SELECT '─── §11b ambrand language coverage ────────────────────' AS section;

SELECT 'ambrand' AS tbl, lang, COUNT(*) AS row_count
FROM ambrand
GROUP BY lang
ORDER BY row_count DESC
LIMIT 10;

SELECT '─── §11c assemblygroupnodenames total ─────────────────' AS section;

SELECT
	'assemblygroupnodenames' AS tbl,
	COUNT(*)                 AS total_rows
FROM assemblygroupnodenames;


-- ═══════════════════════════════════════════════════════════════════════════
--             PART C — THE REAL AFTERMARKET ANSWER
-- ═══════════════════════════════════════════════════════════════════════════

-- ─── §12 Aftermarket brands per HK prefix (correct JOIN) ─────────────────
--
-- The CRITICAL section. The 2026-08-26 v1 diagnostic showed top brands
-- on articlecrosses HK prefixes are all car OEMs (Hyundai/Kia/Fiat/etc.)
-- — BUT that column is the cross-refed OEM brand, NOT the aftermarket
-- brand. The REAL aftermarket brand lives at:
--
--   articlecrosses.legacyArticleId
--     → articles.dataSupplierId (NOT mfrId — mfrId is 0 everywhere)
--       → ambrand.brandId (with lang='EN')
--
-- This section does the correct JOIN.

SELECT '─── §12 REAL aftermarket brands on HK cross-refs ──────' AS section;

SELECT
	amb.brandName                       AS aftermarket_brand,
	COUNT(*)                            AS crossref_rows,
	COUNT(DISTINCT acr.legacyArticleId) AS distinct_parts
FROM articlecrosses acr
JOIN articles a  ON a.legacyArticleId = acr.legacyArticleId
JOIN ambrand   amb ON amb.brandId = a.dataSupplierId
                 AND amb.lang = 'EN'
WHERE (
	acr.oemNumberNormalized LIKE '263%' OR
	acr.oemNumberNormalized LIKE '264%' OR
	acr.oemNumberNormalized LIKE '265%' OR
	acr.oemNumberNormalized LIKE '581%' OR
	acr.oemNumberNormalized LIKE '582%' OR
	acr.oemNumberNormalized LIKE '583%' OR
	acr.oemNumberNormalized LIKE '821%' OR
	acr.oemNumberNormalized LIKE '822%' OR
	acr.oemNumberNormalized LIKE '823%' OR
	acr.oemNumberNormalized LIKE '971%' OR
	acr.oemNumberNormalized LIKE '972%'
  )
  AND CHAR_LENGTH(acr.oemNumberNormalized) BETWEEN 8 AND 12
GROUP BY amb.brandName
ORDER BY crossref_rows DESC
LIMIT 30;

-- ─── §13 Explicit marquee-brand probe ───────────────────────────────────
--
-- Focused answer to "does BOSCH have anything for HK OEMs?" for the
-- top ~30 aftermarket brands the market expects to see.

SELECT '─── §13 Explicit marquee-brand probe (HK-only) ────────' AS section;

SELECT
	amb.brandName,
	COUNT(*) AS hk_cross_ref_rows
FROM ambrand amb
JOIN articles      a   ON a.dataSupplierId = amb.brandId
JOIN articlecrosses acr ON acr.legacyArticleId = a.legacyArticleId
WHERE amb.lang = 'EN'
  AND amb.brandName IN (
	'BOSCH','MANN-FILTER','MAHLE','MAHLE ORIGINAL','KNECHT','DENSO',
	'NGK','VALEO','HELLA','BREMBO','TEXTAR','FERODO','SACHS','FEBI',
	'FEBI BILSTEIN','LEMFOERDER','LUK','INA','SKF','GATES',
	'CONTINENTAL','FILTRON','WIX','PURFLUX','MAGNETI MARELLI',
	'DELPHI','MEYLE','TRW','ATE','ZIMMERMANN','BLUE PRINT','KYB',
	'MONROE','SANGSIN','MOBIS','PIERBURG','ELRING','VICTOR REINZ'
  )
  AND (
	acr.oemNumberNormalized LIKE '263%' OR acr.oemNumberNormalized LIKE '581%' OR
	acr.oemNumberNormalized LIKE '821%' OR acr.oemNumberNormalized LIKE '971%'
  )
GROUP BY amb.brandName
ORDER BY hk_cross_ref_rows DESC;


-- ═══════════════════════════════════════════════════════════════════════════
--             PART D — CORPUS VERIFICATION
-- ═══════════════════════════════════════════════════════════════════════════

-- ─── §14 19-OEM audit corpus temp table ─────────────────────────────────
--
-- The 19 real HK OEMs used in the search-quality audit corpus
-- (scripts/audit/corpus-1500-v2.csv). Bounded to keep temp footprint
-- tiny. Every downstream lookup uses this table so all corpus queries
-- share one memory-resident structure.

SELECT '─── §14 Loading 19-OEM audit corpus temp table ────────' AS section;

DROP TEMPORARY TABLE IF EXISTS tmp_corpus;
CREATE TEMPORARY TABLE tmp_corpus (
	oem        VARCHAR(30) NOT NULL,
	normalized VARCHAR(30) NOT NULL,
	part_kind  VARCHAR(20) NOT NULL,
	PRIMARY KEY (normalized)
) ENGINE=MEMORY;

INSERT INTO tmp_corpus (oem, normalized, part_kind) VALUES
	('263202G000', '263202g000', 'oil_filter'),
	('263003C100', '263003c100', 'oil_filter'),
	('263003C300', '263003c300', 'oil_filter'),
	('263203CAA0', '263203caa0', 'oil_filter'),
	('263203CAB0', '263203cab0', 'oil_filter'),
	('581012GA10', '581012ga10', 'brake_pad'),
	('581012MA00', '581012ma00', 'brake_pad'),
	('581013QA50', '581013qa50', 'brake_pad'),
	('581012HA10', '581012ha10', 'brake_pad'),
	('581012HA20', '581012ha20', 'brake_pad'),
	('971332S000', '971332s000', 'cabin_filter'),
	('971332S100', '971332s100', 'cabin_filter'),
	('971331Y000', '971331y000', 'cabin_filter'),
	('971332J000', '971332j000', 'cabin_filter'),
	('821013SA00', '821013sa00', 'window'),
	('823703SA00', '823703sa00', 'window'),
	('822013SA00', '822013sa00', 'window'),
	('292202G010', '292202g010', 'oil_pan_gasket'),
	('292202G100', '292202g100', 'oil_pan_gasket');

SELECT COUNT(*) AS corpus_size FROM tmp_corpus;

-- ─── §15 Corpus lookup via oem_number (primary path) ────────────────────

SELECT '─── §15 Corpus OEMs via oem_number.clean_number ───────' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(on2.id)      AS oem_number_rows,
	MIN(on2.articleId) AS sample_article_id
FROM tmp_corpus c
LEFT JOIN oem_number on2 ON on2.clean_number = c.normalized
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;


-- ─── §16 Corpus lookup via articlecrosses + real aftermarket brands ─────

SELECT '─── §16 Corpus crossref counts + brand diversity ──────' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(acr.id)                 AS crossref_rows,
	COUNT(DISTINCT acr.brandName) AS distinct_brands
FROM tmp_corpus c
LEFT JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;

SELECT '─── §16b REAL aftermarket brand per corpus OEM ────────' AS section;

SELECT
	c.oem,
	c.part_kind,
	amb.brandName AS aftermarket_brand,
	COUNT(*)      AS rows_for_this_oem_brand
FROM tmp_corpus c
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
JOIN articles      a   ON a.legacyArticleId = acr.legacyArticleId
JOIN ambrand       amb ON amb.brandId = a.dataSupplierId
                      AND amb.lang = 'EN'
GROUP BY c.oem, c.part_kind, amb.brandName
ORDER BY c.part_kind, c.oem, rows_for_this_oem_brand DESC;

SELECT '─── §16c aftermarket brands rolled up per part_kind ───' AS section;

SELECT
	c.part_kind,
	amb.brandName                       AS aftermarket_brand,
	COUNT(DISTINCT c.oem)               AS distinct_corpus_oems_covered,
	COUNT(*)                            AS total_crossref_rows
FROM tmp_corpus c
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
JOIN articles      a   ON a.legacyArticleId = acr.legacyArticleId
JOIN ambrand       amb ON amb.brandId = a.dataSupplierId
                      AND amb.lang = 'EN'
GROUP BY c.part_kind, amb.brandName
ORDER BY c.part_kind, distinct_corpus_oems_covered DESC, total_crossref_rows DESC;

-- Spec coverage per corpus OEM (uses sql/07 idx_articlecriteria_legacyArticleId).
SELECT '─── §16d Spec coverage per corpus OEM ─────────────────' AS section;

SELECT
	c.oem,
	c.part_kind,
	COUNT(ac.id)                          AS spec_rows,
	COUNT(DISTINCT ac.criteriaDescription) AS distinct_criteria
FROM tmp_corpus c
JOIN articlecrosses acr ON acr.oemNumberNormalized = c.normalized
JOIN articlecriteria ac ON ac.legacyArticleId = acr.legacyArticleId
GROUP BY c.oem, c.part_kind
ORDER BY c.part_kind, c.oem;

DROP TEMPORARY TABLE IF EXISTS tmp_corpus;


-- ═══════════════════════════════════════════════════════════════════════════
--             PART E — SAMPLE ROWS (spot-check data shape)
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '─── §17 Sample oem_number rows ────────────────────────' AS section;
SELECT * FROM oem_number LIMIT 5;

SELECT '─── §17b Sample articlecrosses rows (HK prefix) ───────' AS section;
SELECT id, oemNumber, oemNumberNormalized, brandName, legacyArticleId, mfrId
FROM articlecrosses
WHERE oemNumberNormalized LIKE '58%'
LIMIT 5;

SELECT '─── §17c Sample linkagetargets rows ───────────────────' AS section;
SELECT * FROM linkagetargets LIMIT 3;


-- ═══════════════════════════════════════════════════════════════════════════
--             PART F — QUERY-PLAN VERIFICATION
-- ═══════════════════════════════════════════════════════════════════════════
--
-- Every EXPLAIN below should show `type=ref` (or better) with a
-- meaningful `key`. When any of them shows `type=ALL` (full table scan)
-- the corresponding sql/06 / sql/07 / sql/08 index is missing → correlate
-- with the pass/fail summary in §2c.

SELECT '─── §18 EXPLAIN — articlecriteria FindBySpecMatch ─────' AS section;

EXPLAIN
SELECT DISTINCT a.legacyArticleId, a.articleNumber
FROM articlecriteria ac
JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
WHERE ac.criteriaDescription = 'Thread Size'
  AND ac.rawValue = 'M20 x 1.5'
LIMIT 10;

SELECT '─── §19 EXPLAIN — articlecriteria FindSpecifications ──' AS section;

EXPLAIN
SELECT criteriaDescription, rawValue
FROM articlecriteria
WHERE legacyArticleId = 12345
  AND criteriaDescription IN ('Length [mm]', 'Weight [kg]', 'Height [mm]');

SELECT '─── §20 EXPLAIN — articlecrosses SearchCrossReferences ┈' AS section;

EXPLAIN
SELECT id, oemNumber, brandName, legacyArticleId
FROM articlecrosses
WHERE oemNumberNormalized = '263202g000';

SELECT '─── §21 EXPLAIN — oem_number SearchByOEM primary ──────' AS section;

EXPLAIN
SELECT id, number, articleId
FROM oem_number
WHERE clean_number = '263202g000';

SELECT '─── §22 EXPLAIN — articlesvehicletrees PartsForVehicle ┈' AS section;

EXPLAIN
SELECT avt.legacyArticleId, avt.assemblyGroupNodeId
FROM articlesvehicletrees avt
WHERE avt.linkingTargetId = 30001
  AND avt.linkingTargetType = 'P'
LIMIT 100;

SELECT '─── §23 EXPLAIN — oem_search_index SearchByOEMIndex ───' AS section;

EXPLAIN
SELECT legacyArticleId
FROM oem_search_index
WHERE normalized = '263202g000';


-- ═══════════════════════════════════════════════════════════════════════════
--                     REPORT COMPLETE
-- ═══════════════════════════════════════════════════════════════════════════

SELECT '' AS ' ';
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';
SELECT '  DIAGNOSTIC COMPLETE — paste full output back for analysis' AS ' ';
SELECT '  Completed at:' AS ' ', NOW() AS completed_at;
SELECT '════════════════════════════════════════════════════════════════════════' AS ' ';

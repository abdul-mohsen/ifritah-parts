package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"parts-engine/internal/model"
)

// TecDoc provides direct queries against the full TecDoc MySQL schema.
// This is used when MySQL (dev_ifritah) is connected, giving access to
// 651M vehicle-part linkages, 21.5M OEM numbers, and full cross-references.
//
// The optional `online` field is the M8 online-search dispatcher: when
// wired, FindAftermarketForOEM runs a 4th UNION path against
// aftermarket_online_cache (Postgres) with fall-through to eBay Motors
// API and the G5 public-reference adapters. When nil (default),
// FindAftermarketForOEM behaves as its M2.S1 3-path predecessor.
type TecDoc struct {
	db     *sql.DB
	online *OnlineSearch
}

func NewTecDoc(db *sql.DB) *TecDoc {
	if db == nil {
		return nil
	}
	return &TecDoc{db: db}
}

// WithOnlineSearch wires the M8 online-search dispatcher into TecDoc so
// FindAftermarketForOEM includes the online UNION path. Pass nil to
// disable (equivalent to constructing TecDoc without ever calling this).
// Returns the same *TecDoc for chaining.
func (t *TecDoc) WithOnlineSearch(online *OnlineSearch) *TecDoc {
	if t != nil {
		t.online = online
	}
	return t
}

// ---------- OEM number search ----------

// SearchByOEM finds aftermarket parts matching an OEM number using the
// oem_number table (21.5M rows, FULLTEXT indexed) + articlecrosses.
func (t *TecDoc) SearchByOEM(oemNumber string, limit int) ([]model.OEMReference, error) {
	start := time.Now()
	log.Printf("[TecDoc.SearchByOEM] START oem=%q limit=%d", oemNumber, limit)

	if limit <= 0 || limit > 100 {
		limit = 30
	}
	clean := NormalizeOEM(oemNumber)
	if clean == "" {
		log.Printf("[TecDoc.SearchByOEM] ABORT empty after normalize")
		return nil, fmt.Errorf("empty OEM number")
	}
	log.Printf("[TecDoc.SearchByOEM] normalized=%q", clean)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Primary: oem_number table with clean_number match (fastest path)
	query := `
		SELECT DISTINCT
			on2.number AS oem_raw,
			a.legacyArticleId,
			a.articleNumber,
			a.genericArticleDescription,
			COALESCE(ab.brandName, '') AS brand
		FROM oem_number on2
		JOIN articles a ON a.legacyArticleId = on2.articleId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE on2.clean_number = ?
		LIMIT ?`

	rows, err := logQueryCtx(t.db, ctx, "TecDoc.SearchByOEM.primary", query, clean, limit)
	if err != nil {
		log.Printf("[TecDoc.SearchByOEM] PRIMARY query ERROR after %v: %v", time.Since(start), err)
		return nil, fmt.Errorf("oem_number search: %w", err)
	}
	log.Printf("[TecDoc.SearchByOEM] PRIMARY query returned in %v", time.Since(start))
	defer rows.Close()

	var refs []model.OEMReference
	seen := map[int]bool{}
	for rows.Next() {
		var ref model.OEMReference
		var desc, brand sql.NullString
		if err := rows.Scan(&ref.RawNumber, &ref.LegacyArticleId, &ref.ArticleNumber,
			&desc, &brand); err != nil {
			continue
		}
		if seen[ref.LegacyArticleId] {
			continue
		}
		seen[ref.LegacyArticleId] = true
		ref.Description = desc.String
		ref.BrandName = brand.String
		ref.Manufacturer = "OEM"
		refs = append(refs, ref)
	}

	log.Printf("[TecDoc.SearchByOEM] PRIMARY results=%d, starting SECONDARY (oem_search_index)", len(refs))

	// Secondary: also check oem_search_index for cross-ref matches
	// (replaces 30M-row articlecrosses full table scan with indexed lookup)
	query2 := `
		SELECT osi.raw_number, osi.legacyArticleId,
		       COALESCE(a.articleNumber,''), COALESCE(a.genericArticleDescription,''),
		       COALESCE(ab.brandName,'') AS brand
		FROM oem_search_index osi
		LEFT JOIN articles a ON a.legacyArticleId = osi.legacyArticleId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE osi.normalized = ?
		LIMIT ?`

	rows2, err := logQueryCtx(t.db, ctx, "TecDoc.SearchByOEM.secondary", query2, clean, limit)
	if err == nil {
		log.Printf("[TecDoc.SearchByOEM] SECONDARY query returned in %v", time.Since(start))
		defer rows2.Close()
		for rows2.Next() {
			var ref model.OEMReference
			var desc, brand sql.NullString
			if err := rows2.Scan(&ref.RawNumber, &ref.LegacyArticleId, &ref.ArticleNumber,
				&desc, &brand); err != nil {
				continue
			}
			if seen[ref.LegacyArticleId] {
				continue
			}
			seen[ref.LegacyArticleId] = true
			ref.Description = desc.String
			ref.BrandName = brand.String
			ref.Manufacturer = "CROSSREF"
			refs = append(refs, ref)
		}
	} else {
		log.Printf("[TecDoc.SearchByOEM] SECONDARY query ERROR: %v", err)
	}

	log.Printf("[TecDoc.SearchByOEM] DONE total=%d elapsed=%v", len(refs), time.Since(start))
	return refs, nil
}

// SearchByOEMIndex is the third-level article-id promotion path used by
// enrichResults (M3.S1.T1). Queries oem_search_index directly instead of
// going through the primary oem_number path. Some HK OEMs land here only
// (fuzzy cross-refs stored against slightly different OEM strings).
//
// Same query shape as SearchByOEM's secondary block, extracted so it can
// run independently. Returns []model.OEMReference with Manufacturer set
// to "CROSSREF" so downstream consumers can tell where the ref came from.
func (t *TecDoc) SearchByOEMIndex(oemNumber string, limit int) ([]model.OEMReference, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	clean := NormalizeOEM(oemNumber)
	if clean == "" {
		return nil, fmt.Errorf("empty OEM number")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `
		SELECT osi.raw_number, osi.legacyArticleId,
		       COALESCE(a.articleNumber,''), COALESCE(a.genericArticleDescription,''),
		       COALESCE(ab.brandName,'') AS brand
		FROM oem_search_index osi
		LEFT JOIN articles a ON a.legacyArticleId = osi.legacyArticleId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE osi.normalized = ?
		LIMIT ?`

	rows, err := logQueryCtx(t.db, ctx, "TecDoc.SearchByOEMIndex", q, clean, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []model.OEMReference
	seen := map[int]bool{}
	for rows.Next() {
		var ref model.OEMReference
		var desc, brand sql.NullString
		if err := rows.Scan(&ref.RawNumber, &ref.LegacyArticleId, &ref.ArticleNumber,
			&desc, &brand); err != nil {
			continue
		}
		if ref.LegacyArticleId != 0 && seen[ref.LegacyArticleId] {
			continue
		}
		if ref.LegacyArticleId != 0 {
			seen[ref.LegacyArticleId] = true
		}
		ref.Description = desc.String
		ref.BrandName = brand.String
		ref.Manufacturer = "CROSSREF"
		refs = append(refs, ref)
	}
	return refs, nil
}

// SearchByKeyword uses the searchindex FULLTEXT index (5.8M rows) to find parts.
func (t *TecDoc) SearchByKeyword(keyword string, limit int) ([]model.OEMReference, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, fmt.Errorf("empty keyword")
	}

	// Use MATCH ... AGAINST in boolean mode for fulltext search
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT
			si.legacyArticleId,
			a.articleNumber,
			a.genericArticleDescription,
			COALESCE(ab.brandName, '') AS brand
		FROM searchindex si
		JOIN articles a ON a.legacyArticleId = si.legacyArticleId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE MATCH(si.keywords) AGAINST(? IN BOOLEAN MODE)
		LIMIT ?`

	rows, err := logQueryCtx(t.db, ctx, "TecDoc.SearchByKeyword", query, keyword, limit)
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	defer rows.Close()

	var refs []model.OEMReference
	for rows.Next() {
		var ref model.OEMReference
		var desc, brand sql.NullString
		if err := rows.Scan(&ref.LegacyArticleId, &ref.ArticleNumber, &desc, &brand); err != nil {
			continue
		}
		ref.Description = desc.String
		ref.BrandName = brand.String
		refs = append(refs, ref)
	}
	return refs, nil
}

// ---------- Vehicle → Parts lookup ----------

// PartsForVehicle returns all parts for a given linkageTargetId (vehicle variant).
// Uses the massive articlesvehicletrees table (651M rows).
func (t *TecDoc) PartsForVehicle(linkageTargetId int, category string, page, limit int) ([]SmartResult, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	where := "avt.linkingTargetId = ? AND avt.linkingTargetType = 'P'"
	args := []any{linkageTargetId}
	if category != "" {
		where += " AND a.genericArticleDescription LIKE ?"
		args = append(args, "%"+category+"%")
	}

	// Count
	countQ := `SELECT COUNT(DISTINCT a.legacyArticleId) FROM articlesvehicletrees avt
		JOIN articles a ON a.legacyArticleId = avt.legacyArticleId WHERE ` + where
	var total int
	if err := logQueryRow(t.db, "TecDoc.PartsForVehicle.count", countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count parts: %w", err)
	}

	// Data
	dataQ := `
		SELECT DISTINCT
			a.legacyArticleId,
			a.articleNumber,
			a.genericArticleDescription,
			COALESCE(ab.brandName, '') AS brand,
			COALESCE(agn.assemblyGroupName, '') AS groupName
		FROM articlesvehicletrees avt
		JOIN articles a ON a.legacyArticleId = avt.legacyArticleId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		LEFT JOIN assemblygroupnodenames agn ON agn.assemblyGroupNodeId = avt.assemblyGroupNodeId AND agn.lang = 'en'
		WHERE ` + where + `
		ORDER BY COALESCE(agn.assemblyGroupName, ''), a.genericArticleDescription, ab.brandName
		LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := logQuery(t.db, "TecDoc.PartsForVehicle.data", dataQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("parts for vehicle: %w", err)
	}
	defer rows.Close()

	var results []SmartResult
	for rows.Next() {
		var p model.Part
		var desc, brand, group sql.NullString
		if err := rows.Scan(&p.LegacyArticleId, &p.ArticleNumber, &desc, &brand, &group); err != nil {
			continue
		}
		p.Description = desc.String
		p.BrandName = brand.String
		p.Category = group.String

		rule := ClassifyCategory(p.Description)
		results = append(results, SmartResult{
			Part:          p,
			Confidence:    0.95,
			FitmentDriver: driverName(rule.Driver),
			BrandResolved: brand.String,
		})
	}
	return results, total, nil
}

// ---------- Assembly group / category browsing ----------

// AssemblyGroups returns the top-level assembly groups for a vehicle.
func (t *TecDoc) AssemblyGroups(linkageTargetId int) ([]model.CategoryInfo, error) {
	query := `
		SELECT
			agn.assemblyGroupName,
			COUNT(DISTINCT avt.legacyArticleId) AS partCount
		FROM articlesvehicletrees avt
		JOIN assemblygroupnodenames agn ON agn.assemblyGroupNodeId = avt.assemblyGroupNodeId AND agn.lang = 'en'
		WHERE avt.linkingTargetId = ? AND avt.linkingTargetType = 'P'
		GROUP BY agn.assemblyGroupName
		ORDER BY agn.assemblyGroupName`

	rows, err := logQuery(t.db, "TecDoc.AssemblyGroups", query, linkageTargetId)
	if err != nil {
	}
	defer rows.Close()

	var cats []model.CategoryInfo
	for rows.Next() {
		var c model.CategoryInfo
		if err := rows.Scan(&c.Name, &c.PartCount); err != nil {
			continue
		}
		c.FitmentDriver = driverName(ClassifyCategory(c.Name).Driver)
		cats = append(cats, c)
	}
	return cats, nil
}

// ---------- Cross-reference expansion ----------

// FindReplacements returns supersession (replaced-by and replaces) for a part.
func (t *TecDoc) FindReplacements(legacyArticleId int) ([]model.SupersessionLink, error) {
	var chain []model.SupersessionLink

	// Forward: this part is replaced by...
	fwdQ := `
		SELECT rba.articleNumber, COALESCE(ab.brandName, '') AS brand
		FROM replacedbyarticles rba
		LEFT JOIN articles a ON UPPER(a.articleNumber) = UPPER(rba.articleNumber) AND a.mfrId = rba.mfrId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE rba.legacyArticleId = ?
		LIMIT 20`
	rows, err := logQuery(t.db, "TecDoc.FindReplacements.fwd", fwdQ, legacyArticleId)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var link model.SupersessionLink
			var brand sql.NullString
			if err := rows.Scan(&link.ArticleNumber, &brand); err != nil {
				continue
			}
			link.Direction = "replaced_by"
			link.BrandName = brand.String
			link.LegacyArticleId = legacyArticleId
			chain = append(chain, link)
		}
	}

	// Backward: this part replaces...
	bwdQ := `
		SELECT ra.articleNumber, COALESCE(ab.brandName, '') AS brand
		FROM replacesarticles ra
		LEFT JOIN articles a ON UPPER(a.articleNumber) = UPPER(ra.articleNumber) AND a.mfrId = ra.mfrId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE ra.legacyArticleId = ?
		LIMIT 20`
	rows2, err := logQuery(t.db, "TecDoc.FindReplacements.bwd", bwdQ, legacyArticleId)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var link model.SupersessionLink
			var brand sql.NullString
			if err := rows2.Scan(&link.ArticleNumber, &brand); err != nil {
				continue
			}
			link.Direction = "replaces"
			link.BrandName = brand.String
			link.LegacyArticleId = legacyArticleId
			chain = append(chain, link)
		}
	}
	return chain, nil
}

// FindAftermarketForOEM returns aftermarket alternatives for an OEM number
// by running FOUR lookup paths in parallel and unioning the deduped
// results. Each path has different HK coverage — the union catches parts
// that only one path knows about.
//
//	Path 1: articlecrosses.oemNumberNormalized  - 30M rows, indexed by sql/06
//	Path 2: oem_number.clean_number             - 21.5M rows, indexed by sql/06
//	Path 3: oem_search_index.normalized         - PR #14 secondary xref
//	Path 4: OnlineSearch dispatcher (M8)        - eBay Motors + G5 sites,
//	                                              cache-first via
//	                                              aftermarket_online_cache
//
// The 2026-08-23 quality audit found 6.7% aftermarket coverage when only
// path 1 ran (post-PR-#20 rewrite). M2.S1.T1 adds paths 2 and 3 so the
// data-sparse case where one path returns 0 while another returns rows
// still yields a full aftermarket list. M8.T11 adds path 4 to fill the
// structural HK-aftermarket gap that the 2026-08-26 diagnostic surfaced.
//
// Path 4 is opt-in: when t.online is nil (default in tests, in
// environments without the online cache Postgres connection, or when
// ONLINE_SEARCH_ENABLED=false) the online path returns nil immediately
// and the function behaves exactly as the M2.S1 3-path version.
//
// Dedup key: (NormalizeBrand(brand), lower(partNumber)) — so "BOSCH" /
// "Bosch" / "Robert Bosch GmbH" collapse to one canonical entry.
//
// Per-path budget: 3s wall clock via ctx. If a path is slow, the others
// still return what they have. Online path has its own longer 8s
// dispatcher budget internally, but the 3s ctx here caps overall wait.
func (t *TecDoc) FindAftermarketForOEM(oemNumber string) ([]model.AftermarketPart, error) {
	clean := NormalizeOEM(oemNumber)
	if clean == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	type pathResult struct {
		parts []model.AftermarketPart
		err   error
		name  string
	}
	resultCh := make(chan pathResult, 4)

	go func() {
		p, err := t.findAftermarketFromArticlecrosses(ctx, clean)
		resultCh <- pathResult{p, err, "articlecrosses"}
	}()
	go func() {
		p, err := t.findAftermarketFromOemNumber(ctx, clean)
		resultCh <- pathResult{p, err, "oem_number"}
	}()
	go func() {
		p, err := t.findAftermarketFromOemSearchIndex(ctx, clean)
		resultCh <- pathResult{p, err, "oem_search_index"}
	}()
	// Path 4 (M8): federated online-search dispatcher. Cache-first;
	// falls back to eBay Motors API + G5 sites when the entry is stale
	// or missing. Always safe when t.online == nil (production without
	// the online cache DB connection).
	pathCount := 3
	if t.online != nil {
		pathCount = 4
		go func() {
			resultCh <- pathResult{t.online.Search(ctx, clean), nil, "online"}
		}()
	}

	seen := make(map[string]bool, 100)
	var out []model.AftermarketPart

	for i := 0; i < pathCount; i++ {
		select {
		case r := <-resultCh:
			if r.err != nil {
				log.Printf("[FindAftermarketForOEM] path=%s oem=%s err=%v", r.name, clean, r.err)
				continue
			}
			for _, p := range r.parts {
				if p.PartNumber == "" || p.Brand == "" {
					continue
				}
				key := NormalizeBrand(p.Brand) + "|" + strings.ToLower(p.PartNumber)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, p)
			}
		case <-ctx.Done():
			log.Printf("[FindAftermarketForOEM] ctx deadline exceeded oem=%s returning %d partial results", clean, len(out))
			// Still tier-sort + cap what we have before returning.
			SortAftermarketByTier(out)
			return CapAftermarketList(out, 20, 3), nil
		}
	}

	// M2.S2: tier-sort (Bosch/MANN/MAHLE first, alphabetical inside tier)
	// then cap at 20 total / 3 per brand so the response doesn't overwhelm
	// the UI or let one brand dominate.
	SortAftermarketByTier(out)
	return CapAftermarketList(out, 20, 3), nil
}

// findAftermarketFromArticlecrosses queries the articlecrosses 30M-row
// cross-reference table via the sql/06 generated column index. This is
// the primary aftermarket path — TecDoc's canonical OEM↔aftermarket
// mapping.
func (t *TecDoc) findAftermarketFromArticlecrosses(ctx context.Context, clean string) ([]model.AftermarketPart, error) {
	const query = `
		SELECT DISTINCT
			COALESCE(a.articleNumber, ''),
			COALESCE(a.genericArticleDescription, ''),
			COALESCE(ac.brandName, ''),
			COALESCE(m.manuName, '') AS mfrName
		FROM articlecrosses ac
		LEFT JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
		LEFT JOIN manufacturers m ON m.manuId = ac.mfrId AND m.linkingTargetType = 'P'
		WHERE ac.oemNumberNormalized = ?
		ORDER BY ac.brandName, a.articleNumber
		LIMIT 50`
	rows, err := logQueryCtx(t.db, ctx, "TecDoc.FindAftermarketForOEM.articlecrosses", query, clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAftermarketRows(rows), nil
}

// findAftermarketFromOemNumber queries the oem_number 21.5M-row TecDoc
// OEM catalog. Some parts appear here but NOT in articlecrosses (the
// aftermarket cross-ref only records brands that publish crosses; OEM
// suppliers who don't publish appear here only).
func (t *TecDoc) findAftermarketFromOemNumber(ctx context.Context, clean string) ([]model.AftermarketPart, error) {
	const query = `
		SELECT DISTINCT
			COALESCE(a.articleNumber, ''),
			COALESCE(a.genericArticleDescription, ''),
			COALESCE(ab.brandName, '') AS brand,
			'' AS mfrName
		FROM oem_number on2
		JOIN articles a ON a.legacyArticleId = on2.articleId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE on2.clean_number = ?
		ORDER BY ab.brandName, a.articleNumber
		LIMIT 50`
	rows, err := logQueryCtx(t.db, ctx, "TecDoc.FindAftermarketForOEM.oem_number", query, clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAftermarketRows(rows), nil
}

// findAftermarketFromOemSearchIndex queries the oem_search_index secondary
// cross-ref table (introduced by PR #14). Fewer rows than articlecrosses
// but catches a different slice of cross-refs — some OEMs only appear
// here after supersession-chain walks.
func (t *TecDoc) findAftermarketFromOemSearchIndex(ctx context.Context, clean string) ([]model.AftermarketPart, error) {
	const query = `
		SELECT DISTINCT
			COALESCE(a.articleNumber, ''),
			COALESCE(a.genericArticleDescription, ''),
			COALESCE(ab.brandName, '') AS brand,
			'' AS mfrName
		FROM oem_search_index osi
		LEFT JOIN articles a ON a.legacyArticleId = osi.legacyArticleId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE osi.normalized = ?
		LIMIT 50`
	rows, err := logQueryCtx(t.db, ctx, "TecDoc.FindAftermarketForOEM.oem_search_index", query, clean)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAftermarketRows(rows), nil
}

// scanAftermarketRows is the shared row-scanner used by all three
// aftermarket lookup paths. Consolidates the (articleNumber, description,
// brand, mfrName) column shape so per-path logic stays SQL-only.
func scanAftermarketRows(rows *sql.Rows) []model.AftermarketPart {
	var parts []model.AftermarketPart
	for rows.Next() {
		var p model.AftermarketPart
		var desc, brand, mfrName sql.NullString
		if err := rows.Scan(&p.PartNumber, &desc, &brand, &mfrName); err != nil {
			continue
		}
		p.Description = desc.String
		p.Brand = firstNonEmpty(brand.String, mfrName.String)
		if p.PartNumber == "" || p.Brand == "" {
			continue
		}
		parts = append(parts, p)
	}
	return parts
}

// ---------- Vehicle resolution ----------

// LinkageTargetsForNHTSA resolves an NHTSA-decoded vehicle (make + model +
// year) to a set of matching TecDoc linkageTargetIds. Powers M5.S2.T2:
// VIN → parts pipeline. Two-step lookup:
//
//  1. manuId lookup on manufacturers table (make name is upper-cased, e.g.
//     "HYUNDAI" -> manuId)
//  2. modelseries → linkagetargets join with LIKE-fuzzy model match + year
//     filter (beginYearMonth <= year*100+12 AND endYearMonth >= year*100+1
//     OR endYearMonth = 0)
//
// Fuzzy model match uses `LIKE %model%` on the exact NHTSA model string.
// Some NHTSA models don't exactly match TecDoc's naming (e.g. "Elantra"
// vs "ELANTRA (HD)"), so LIKE catches both. Caller should sort results
// by year-range proximity and pick the top-N.
//
// Returns [] when no match; empty is not an error.
// Bounded to 20 linkage targets so a bad match doesn't fan out unbounded.
func (t *TecDoc) LinkageTargetsForNHTSA(makeName, modelName string, year int) ([]int, error) {
	if t.db == nil {
		return nil, nil
	}
	if makeName == "" || modelName == "" || year == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	const q = `
		SELECT DISTINCT lt.linkageTargetId
		FROM linkagetargets lt
		JOIN modelseries ms ON lt.vehicleModelSeriesId = ms.modelId
		JOIN manufacturers m ON m.manuId = ms.manuId
		WHERE UPPER(m.manuName) LIKE UPPER(?)
		  AND UPPER(ms.modelname) LIKE UPPER(?)
		  AND ms.linkingTargetType = 'P'
		  AND lt.lang = 'en'
		  AND (
		    (lt.beginYearMonth <= ? AND lt.endYearMonth = 0)
		    OR
		    (lt.beginYearMonth <= ? AND lt.endYearMonth >= ?)
		  )
		LIMIT 20`

	makeLike := "%" + strings.ToUpper(makeName) + "%"
	modelLike := "%" + strings.ToUpper(modelName) + "%"
	yearEnd := year*100 + 12  // Dec of the target year
	yearStart := year*100 + 1 // Jan of the target year

	rows, err := logQueryCtx(t.db, ctx, "TecDoc.LinkageTargetsForNHTSA", q,
		makeLike, modelLike, yearEnd, yearEnd, yearStart)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		if id > 0 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ResolveVehicle finds TecDoc vehicle variants by make/model/year via modelseries+linkagetargets.
func (t *TecDoc) ResolveVehicle(manuId int, modelPattern string, year int) ([]model.Vehicle, error) {
	query := `
		SELECT DISTINCT
			lt.linkageTargetId,
			lt.description,
			lt.fuelType,
			lt.capacityCC,
			lt.horsePowerFrom,
			lt.bodyStyle,
			lt.driveType,
			lt.beginYearMonth,
			lt.endYearMonth,
			m.manuName
		FROM modelseries ms
		JOIN linkagetargets lt ON lt.vehicleModelSeriesId = ms.modelId AND lt.lang = 'en'
		JOIN manufacturers m ON m.manuId = ms.manuId
		WHERE ms.manuId = ?
		  AND ms.modelname LIKE ?
		  AND ms.linkingTargetType = 'P'
		  AND lt.beginYearMonth <= ? * 100 + 12
		  AND (lt.endYearMonth >= ? * 100 + 1 OR lt.endYearMonth = 0)
		ORDER BY lt.capacityCC, lt.horsePowerFrom`

	rows, err := logQuery(t.db, "TecDoc.ResolveVehicle", query, manuId, "%"+modelPattern+"%", year, year)
	if err != nil {
		return nil, fmt.Errorf("resolve vehicle: %w", err)
	}
	defer rows.Close()

	var vehicles []model.Vehicle
	for rows.Next() {
		var v model.Vehicle
		var desc, fuel, body, drive, make sql.NullString
		var cc, hp sql.NullInt32
		if err := rows.Scan(&v.LinkageTargetId, &desc, &fuel, &cc, &hp, &body, &drive,
			&v.BeginYearMonth, &v.EndYearMonth, &make); err != nil {
			continue
		}
		v.Description = desc.String
		v.FuelType = fuel.String
		v.Make = make.String
		if cc.Valid {
			v.CapacityCC = int(cc.Int32)
		}
		if hp.Valid {
			v.HorsePower = int(hp.Int32)
		}
		vehicles = append(vehicles, v)
	}
	return vehicles, nil
}

// ---------- Article criteria (specs) ----------

// ArticleSpecs returns the technical specifications for a part (dimensions, material, etc.).
func (t *TecDoc) ArticleSpecs(legacyArticleId int) (map[string]string, error) {
	query := `
		SELECT criteriaDescription, rawValue, COALESCE(criteriaUnitDescription, '')
		FROM articlecriteria
		WHERE legacyArticleId = ?
		ORDER BY criteriaDescription`

	rows, err := logQuery(t.db, "TecDoc.ArticleSpecs", query, legacyArticleId)
	if err != nil {
	}
	defer rows.Close()

	specs := make(map[string]string)
	for rows.Next() {
		var name, val, unit string
		if err := rows.Scan(&name, &val, &unit); err != nil {
			continue
		}
		if unit != "" {
			val += " " + unit
		}
		specs[name] = val
	}
	return specs, nil
}

// ---------- Category hierarchy ----------

// GenericArticleGroups returns the assembly group → category hierarchy for browsing.
func (t *TecDoc) GenericArticleGroups() ([]map[string]interface{}, error) {
	query := `
		SELECT assemblyGroup, masterDesignation,
		       COUNT(DISTINCT genericArticleId) AS subCategories,
		       GROUP_CONCAT(DISTINCT designation ORDER BY designation SEPARATOR ' | ') AS designations
		FROM genericarticlesgroups
		WHERE lang = 'en'
		GROUP BY assemblyGroup, masterDesignation
		ORDER BY assemblyGroup`

	rows, err := logQuery(t.db, "TecDoc.GenericArticleGroups", query)
	if err != nil {
	}
	defer rows.Close()

	var groups []map[string]interface{}
	for rows.Next() {
		var group, master, desigs sql.NullString
		var count int
		if err := rows.Scan(&group, &master, &count, &desigs); err != nil {
			continue
		}
		groups = append(groups, map[string]interface{}{
			"assemblyGroup":     group.String,
			"masterDesignation": master.String,
			"subCategories":     count,
			"designations":      desigs.String,
		})
	}
	return groups, nil
}

// ---------- Fitment check ----------

// CheckFitment verifies if a specific article fits a specific vehicle.
func (t *TecDoc) CheckFitment(legacyArticleId, linkageTargetId int) (bool, error) {
	var count int
	err := logQueryRow(t.db, "TecDoc.CheckFitment", `
		SELECT COUNT(*) FROM articlesvehicletrees
		WHERE legacyArticleId = ? AND linkingTargetId = ? AND linkingTargetType = 'P'`,
		legacyArticleId, linkageTargetId).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check fitment: %w", err)
	}
	return count > 0, nil
}

// LogStats prints a summary of available TecDoc data.
func (t *TecDoc) LogStats() {
	tables := []struct{ name, label string }{
		{"articles", "Articles"},
		{"articlesvehicletrees", "Vehicle-Part Links"},
		{"oem_number", "OEM Numbers"},
		{"articlecrosses", "Cross-References"},
		{"linkagetargets", "Vehicle Variants"},
		{"searchindex", "Search Index"},
		{"replacedbyarticles", "Supersessions (fwd)"},
		{"replacesarticles", "Supersessions (bwd)"},
		{"articlecriteria", "Part Specs"},
	}
	for _, tbl := range tables {
		var count int64
		err := t.db.QueryRow("SELECT COUNT(*) FROM " + tbl.name).Scan(&count)
		if err != nil {
			log.Printf("  ⚠ %s: %v", tbl.label, err)
		} else {
			log.Printf("  ✓ %s: %s rows", tbl.label, formatCount(count))
		}
	}
}

func formatCount(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

// LinkageTargetToSpecs reads linkagetargets columns (capacityCC, cylinders,
// fuelType, engineType, horsePowerFrom) and maps them to model.Specification
// slices so they can be fed into AssemblyContextStrategy / VinAssemblyStrategy
// as if they were article specifications. (S8-T1)
func (t *TecDoc) LinkageTargetToSpecs(ctx context.Context, linkageTargetId int) ([]model.Specification, error) {
	if t.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	const q = `
		SELECT
			COALESCE(capacityCC, 0),
			COALESCE(cylinders, 0),
			COALESCE(fuelType, ''),
			COALESCE(engineType, ''),
			COALESCE(horsePowerFrom, 0)
		FROM linkagetargets
		WHERE linkageTargetId = ?
		LIMIT 1`

	row := t.db.QueryRowContext(ctx, q, linkageTargetId)
	var capacityCC, cylinders, horsePower int
	var fuelType, engineType string
	if err := row.Scan(&capacityCC, &cylinders, &fuelType, &engineType, &horsePower); err != nil {
		return nil, fmt.Errorf("linkagetargets lookup: %w", err)
	}

	var specs []model.Specification
	if capacityCC > 0 {
		specs = append(specs, model.Specification{
			Name:   "displacement",
			Value:  fmt.Sprintf("%d", capacityCC),
			Unit:   "cc",
			Source: "tecdoc:linkagetargets",
		})
	}
	if cylinders > 0 {
		specs = append(specs, model.Specification{
			Name:   "cylinders",
			Value:  fmt.Sprintf("%d", cylinders),
			Source: "tecdoc:linkagetargets",
		})
	}
	if fuelType != "" {
		specs = append(specs, model.Specification{
			Name:   "fuel_type",
			Value:  fuelType,
			Source: "tecdoc:linkagetargets",
		})
	}
	if horsePower > 0 {
		specs = append(specs, model.Specification{
			Name:   "horse_power",
			Value:  fmt.Sprintf("%d", horsePower),
			Unit:   "hp",
			Source: "tecdoc:linkagetargets",
		})
	}
	return specs, nil
}

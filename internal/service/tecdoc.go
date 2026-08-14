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
type TecDoc struct {
	db *sql.DB
}

func NewTecDoc(db *sql.DB) *TecDoc {
	if db == nil {
		return nil
	}
	return &TecDoc{db: db}
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

	rows, err := logQuery(t.db, "TecDoc.SearchByKeyword", query, keyword, limit)
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

// FindAftermarketForOEM finds aftermarket alternatives via the full TecDoc cross-ref chain:
// OEM number → oem_number → articles → articlecrosses → aftermarket articles.
func (t *TecDoc) FindAftermarketForOEM(oemNumber string) ([]model.AftermarketPart, error) {
	clean := NormalizeOEM(oemNumber)
	if clean == "" {
		return nil, nil
	}

	query := `
		SELECT DISTINCT
			a.articleNumber,
			a.genericArticleDescription,
			COALESCE(ab.brandName, '') AS brand
		FROM oem_number on2
		JOIN articles a ON a.legacyArticleId = on2.articleId
		LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
		WHERE on2.clean_number = ?
		ORDER BY brand, a.articleNumber
		LIMIT 50`

	rows, err := logQuery(t.db, "TecDoc.FindAftermarketForOEM", query, clean)
	if err != nil {
	}
	defer rows.Close()

	var parts []model.AftermarketPart
	for rows.Next() {
		var p model.AftermarketPart
		var desc sql.NullString
		if err := rows.Scan(&p.PartNumber, &desc, &p.Brand); err != nil {
			continue
		}
		p.Description = desc.String
		parts = append(parts, p)
	}
	return parts, nil
}

// ---------- Vehicle resolution ----------

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

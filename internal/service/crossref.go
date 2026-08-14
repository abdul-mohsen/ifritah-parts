package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"parts-engine/internal/model"
)

// CrossRef provides OEM cross-reference lookups using the articlecrosses table.
type CrossRef struct {
	db      *sql.DB
	localDB *sql.DB // SQLite for aftermarket_crossref (local curated data)
	offline bool
}

func NewCrossRef(db *sql.DB, offline bool) *CrossRef {
	return &CrossRef{db: db, offline: offline}
}

// SetLocalDB sets the SQLite handle for local curated tables (aftermarket_crossref).
func (s *CrossRef) SetLocalDB(localDB *sql.DB) {
	s.localDB = localDB
}

// FindOEMNumbers returns OEM part numbers for an aftermarket article.
// Uses articlecrosses (30M rows) — the correct OEM cross-ref table.
func (s *CrossRef) FindOEMNumbers(legacyArticleId int) ([]model.OEMReference, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	var query string
	if s.offline {
		// SQLite: no articles table, get article info from hk_parts_cache
		query = `
			SELECT ac.oemNumber, ac.brandName, hk.articleNumber, hk.genericArticleDesc
			FROM articlecrosses ac
			LEFT JOIN hk_parts_cache hk ON hk.legacyArticleId = ac.legacyArticleId
			WHERE ac.legacyArticleId = ?
			GROUP BY ac.oemNumber, ac.brandName, hk.articleNumber, hk.genericArticleDesc
			LIMIT 50`
	} else {
		query = `
			SELECT ac.oemNumber, ac.brandName, a.articleNumber, a.genericArticleDescription
			FROM articlecrosses ac
			JOIN articles a ON a.legacyArticleId = ac.legacyArticleId
			WHERE ac.legacyArticleId = ?
			LIMIT 50`
	}

	rows, err := logQuery(s.db, "CrossRef.FindOEMNumbers", query, legacyArticleId)
	if err != nil {
	}
	defer rows.Close()

	var refs []model.OEMReference
	for rows.Next() {
		var ref model.OEMReference
		var brand, artNum, desc sql.NullString
		if err := rows.Scan(&ref.RawNumber, &brand, &artNum, &desc); err != nil {
			return nil, fmt.Errorf("scan OEM ref: %w", err)
		}
		ref.LegacyArticleId = legacyArticleId
		ref.Manufacturer = brand.String
		ref.ArticleNumber = artNum.String
		ref.Description = desc.String
		refs = append(refs, ref)
	}
	return refs, nil
}

// FindByOEM returns aftermarket articles that cross-reference a given OEM number.
// Uses articlecrosses table with brand filtering for Hyundai/Kia OEM numbers.
func (s *CrossRef) FindByOEM(oemNumber string, limit int) ([]model.OEMReference, error) {
	start := time.Now()
	log.Printf("[CrossRef.FindByOEM] START oem=%q limit=%d offline=%v", oemNumber, limit, s.offline)

	if s.db == nil {
		log.Printf("[CrossRef.FindByOEM] ABORT db=nil")
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	normalized := NormalizeOEM(oemNumber)
	log.Printf("[CrossRef.FindByOEM] normalized=%q", normalized)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var query string
	if s.offline {
		// SQLite: use oem_search_index or hk_parts_cache with normalized comparison
		query = `
			SELECT osi.raw_number, COALESCE(osi.mfr_name,''), osi.legacyArticleId,
			       COALESCE(osi.article_number,''), COALESCE(osi.description,''),
			       COALESCE(osi.brand_name,'')
			FROM oem_search_index osi
			WHERE osi.normalized = ?
			LIMIT ?`
	} else {
		// MySQL: use oem_search_index (pre-normalized, indexed) instead of
		// scanning 30M-row articlecrosses with runtime REPLACE/LOWER.
		query = `
			SELECT osi.raw_number, COALESCE(osi.mfr_name,''), osi.legacyArticleId,
			       COALESCE(a.articleNumber,''), COALESCE(a.genericArticleDescription,''),
			       COALESCE(ab.brandName,'') AS aftermarketBrand
			FROM oem_search_index osi
			LEFT JOIN articles a ON a.legacyArticleId = osi.legacyArticleId
			LEFT JOIN ambrand ab ON ab.brandId = a.dataSupplierId AND ab.lang = 'en'
			WHERE osi.normalized = ?
			LIMIT ?`
	}

	rows, err := logQueryCtx(s.db, ctx, "CrossRef.FindByOEM", query, normalized, limit*3)
	if err != nil {
		log.Printf("[CrossRef.FindByOEM] QUERY ERROR after %v: %v", time.Since(start), err)
		return nil, fmt.Errorf("find by OEM: %w", err)
	}
	log.Printf("[CrossRef.FindByOEM] query returned in %v", time.Since(start))
	defer rows.Close()

	var refs []model.OEMReference
	seenArticle := make(map[int]bool)
	for rows.Next() {
		var ref model.OEMReference
		var brand, artNum, desc, afterBrand sql.NullString
		if err := rows.Scan(&ref.RawNumber, &brand, &ref.LegacyArticleId,
			&artNum, &desc, &afterBrand); err != nil {
			log.Printf("[CrossRef.FindByOEM] SCAN ERROR: %v", err)
			return nil, fmt.Errorf("scan cross-ref: %w", err)
		}
		ref.Manufacturer = brand.String
		ref.ArticleNumber = artNum.String
		ref.Description = desc.String
		if afterBrand.Valid {
			ref.BrandName = afterBrand.String
		}

		// Filter self-references: skip rows where raw_number IS the queried OEM
		if NormalizeOEM(ref.ArticleNumber) == normalized {
			continue
		}

		// Deduplicate by legacyArticleId
		if seenArticle[ref.LegacyArticleId] {
			continue
		}
		seenArticle[ref.LegacyArticleId] = true

		refs = append(refs, ref)
		if len(refs) >= limit {
			break
		}
	}
	log.Printf("[CrossRef.FindByOEM] DONE results=%d (deduped) elapsed=%v", len(refs), time.Since(start))
	return refs, nil
}

// FindVehiclesForArticle returns all vehicles that an article fits.
// Uses articlesvehicletrees (359M rows) for full cross-reference across all brands.
// Applies category-aware CC filtering when vehicleCC > 0 and category is engine-dependent.
func (s *CrossRef) FindVehiclesForArticle(legacyArticleId int, vehicleCC int, category string, limit int) ([]model.Vehicle, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var query string
	if s.offline {
		// SQLite: use hk_parts_cache directly (no articlesvehicletrees)
		query = `
			SELECT DISTINCT hk.linkingTargetId, hk.vehicleDesc, hk.beginYearMonth, hk.endYearMonth,
			       hk.fuelType, hk.capacityCC, hk.horsePowerFrom,
			       CASE hk.manuId WHEN 183 THEN 'HYUNDAI' WHEN 184 THEN 'KIA' ELSE '' END
			FROM hk_parts_cache hk
			WHERE hk.legacyArticleId = ?
			ORDER BY CASE hk.manuId WHEN 183 THEN 'HYUNDAI' WHEN 184 THEN 'KIA' ELSE '' END, hk.vehicleDesc
			LIMIT ?`
	} else {
		query = `
			SELECT DISTINCT lt.linkageTargetId, lt.description, lt.beginYearMonth, lt.endYearMonth,
			       lt.fuelType, lt.capacityCC, lt.horsePowerFrom,
			       m.manuName
			FROM articlesvehicletrees avt
			JOIN linkagetargets lt ON lt.linkageTargetId = avt.linkingTargetId AND lt.lang = 'en'
			JOIN modelseries ms ON ms.modelId = lt.vehicleModelSeriesId
			JOIN manufacturers m ON m.manuId = ms.manuId AND m.lang = 'en' AND m.linkingTargetType = 'P'
			WHERE avt.legacyArticleId = ? AND avt.linkingTargetType = 'P'
			ORDER BY m.manuName, lt.description
			LIMIT ?`
	}

	rows, err := logQuery(s.db, "CrossRef.FindVehicles", query, legacyArticleId, limit)
	if err != nil {
		return nil, fmt.Errorf("vehicles for article: %w", err)
	}
	defer rows.Close()

	rule := ClassifyCategory(category)

	var vehicles []model.Vehicle
	for rows.Next() {
		var v model.Vehicle
		var desc, fuel, makeName sql.NullString
		var cap, hp sql.NullInt32
		if err := rows.Scan(&v.LinkageTargetId, &desc, &v.BeginYearMonth, &v.EndYearMonth,
			&fuel, &cap, &hp, &makeName); err != nil {
			return nil, fmt.Errorf("scan vehicle: %w", err)
		}
		v.Description = desc.String
		v.FuelType = fuel.String
		v.Make = makeName.String
		if cap.Valid {
			v.CapacityCC = int(cap.Int32)
		}
		if hp.Valid {
			v.HorsePower = int(hp.Int32)
		}

		// Apply CC filter for engine-dependent parts
		if vehicleCC > 0 && rule.Driver == FitEngine && v.CapacityCC > 0 {
			margin := rule.CCMargin
			if margin == 0 {
				margin = 500
			}
			diff := vehicleCC - v.CapacityCC
			if diff < 0 {
				diff = -diff
			}
			if rule.Strict && diff > margin {
				continue // skip — engine too different
			}
		}

		vehicles = append(vehicles, v)
	}
	return vehicles, nil
}

// FindAftermarketByOEM returns aftermarket alternatives for an OEM part number
// from the aftermarket_crossref table (curated cross-reference database).
func (s *CrossRef) FindAftermarketByOEM(oemNumber string) ([]model.AftermarketPart, error) {
	db := s.localDB
	if db == nil {
		db = s.db // fallback to activeDB (offline mode)
	}
	if db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	// Try both raw and normalized forms
	normalized := NormalizeOEM(oemNumber)

	rows, err := logQuery(db, "CrossRef.FindAftermarketByOEM", `
		SELECT brand, part_number, description, category
		FROM aftermarket_crossref
		WHERE LOWER(REPLACE(REPLACE(REPLACE(REPLACE(oem_number,'-',''),' ',''),'.',''),'/','')) = ?
		ORDER BY brand`,
		normalized)
	if err != nil {
		// Table might not exist yet — not fatal
		return nil, nil
	}
	defer rows.Close()

	var parts []model.AftermarketPart
	for rows.Next() {
		var p model.AftermarketPart
		var category sql.NullString
		if err := rows.Scan(&p.Brand, &p.PartNumber, &p.Description, &category); err != nil {
			continue
		}
		parts = append(parts, p)
	}
	return parts, nil
}

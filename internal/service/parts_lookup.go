package service

import (
	"database/sql"
	"fmt"

	"parts-engine/internal/model"
)

// PartWithOEM extends a Part with optional OEM numbers for enriched responses.
type PartWithOEM struct {
	model.Part
	OEMNumbers []string `json:"oemNumbers,omitempty"`
}

// PartsLookup queries the pre-computed hk_parts_cache table.
type PartsLookup struct {
	db      *sql.DB
	offline bool // true when using SQLite (no linkagetargets/modelseries tables)
}

func NewPartsLookup(db *sql.DB, offline bool) *PartsLookup {
	return &PartsLookup{db: db, offline: offline}
}

// FindByLinkageTarget returns parts for a specific vehicle variant.
func (s *PartsLookup) FindByLinkageTarget(linkageTargetId int, category string, page, limit int) ([]model.Part, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	// Count query
	countSQL := "SELECT COUNT(DISTINCT legacyArticleId) FROM hk_parts_cache WHERE linkingTargetId = ?"
	args := []any{linkageTargetId}
	if category != "" {
		countSQL += " AND categoryName LIKE ?"
		args = append(args, "%"+category+"%")
	}

	var total int
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count parts: %w", err)
	}

	// Data query
	dataSQL := `SELECT DISTINCT legacyArticleId, articleNumber, genericArticleDesc, brandName, categoryName, assemblyGroupNodeId
		FROM hk_parts_cache WHERE linkingTargetId = ?`
	dataArgs := []any{linkageTargetId}
	if category != "" {
		dataSQL += " AND categoryName LIKE ?"
		dataArgs = append(dataArgs, "%"+category+"%")
	}
	dataSQL += " ORDER BY categoryName, brandName LIMIT ? OFFSET ?"
	dataArgs = append(dataArgs, limit, offset)

	rows, err := s.db.Query(dataSQL, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query parts: %w", err)
	}
	defer rows.Close()

	var parts []model.Part
	for rows.Next() {
		var p model.Part
		var desc, brand, cat sql.NullString
		if err := rows.Scan(&p.LegacyArticleId, &p.ArticleNumber, &desc, &brand, &cat, &p.AssemblyGroupId); err != nil {
			return nil, 0, fmt.Errorf("scan part: %w", err)
		}
		p.Description = desc.String
		p.BrandName = brand.String
		p.Category = cat.String
		parts = append(parts, p)
	}
	return parts, total, nil
}

// ResolveLinkageTargets finds TecDoc linkageTargetIds matching NHTSA make/model/year.
func (s *PartsLookup) ResolveLinkageTargets(make, modelName string, year int) ([]model.Vehicle, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	var query string
	if s.offline {
		// SQLite: use pre-flattened vehicle_lookup table
		query = `
			SELECT DISTINCT vl.linkageTargetId, vl.description, vl.beginYearMonth, vl.endYearMonth,
			       vl.fuelType, vl.capacityCC, vl.horsePowerFrom
			FROM vehicle_lookup vl
			WHERE vl.nhtsa_make = ? AND vl.nhtsa_model = ?
			  AND ? BETWEEN vl.year_from AND vl.year_to
			ORDER BY vl.beginYearMonth DESC`
	} else {
		// MySQL: join through modelseries → linkagetargets
		query = `
			SELECT DISTINCT lt.linkageTargetId, lt.description, lt.beginYearMonth, lt.endYearMonth,
			       lt.fuelType, lt.capacityCC, lt.horsePowerFrom
			FROM nhtsa_tecdoc_bridge b
			JOIN modelseries ms ON ms.modelId = b.tecdoc_model_id
			JOIN linkagetargets lt ON lt.vehicleModelSeriesId = ms.modelId AND lt.lang = 'en'
			WHERE b.nhtsa_make = ? AND b.nhtsa_model = ?
			  AND ? BETWEEN b.year_from AND b.year_to
			ORDER BY lt.beginYearMonth DESC`
	}

	rows, err := s.db.Query(query, make, modelName, year)
	if err != nil {
		return nil, fmt.Errorf("resolve targets: %w", err)
	}
	defer rows.Close()

	var vehicles []model.Vehicle
	for rows.Next() {
		var v model.Vehicle
		var desc, fuel sql.NullString
		var cap, hp sql.NullInt32
		if err := rows.Scan(&v.LinkageTargetId, &desc, &v.BeginYearMonth, &v.EndYearMonth, &fuel, &cap, &hp); err != nil {
			return nil, fmt.Errorf("scan vehicle: %w", err)
		}
		v.Make = make
		v.Model = modelName
		v.ModelYear = year
		v.Description = desc.String
		v.FuelType = fuel.String
		if cap.Valid {
			v.CapacityCC = int(cap.Int32)
		}
		if hp.Valid {
			v.HorsePower = int(hp.Int32)
		}
		vehicles = append(vehicles, v)
	}
	return vehicles, nil
}

// BestLinkageTarget picks the variant with the most cataloged parts.
// When engineCC > 0 or fuelType != "", it scores variants by attribute match
// instead of just picking the one with the most parts.
func (s *PartsLookup) BestLinkageTarget(make, modelName string, year int) (*model.Vehicle, error) {
	return s.BestLinkageTargetWithHints(make, modelName, year, 0, "")
}

// BestLinkageTargetWithHints picks the best variant using engine CC and fuel type hints
// from the VIN decode. If hints are zero/empty, falls back to most-parts heuristic.
func (s *PartsLookup) BestLinkageTargetWithHints(make, modelName string, year int, engineCC int, fuelType string) (*model.Vehicle, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	query := ``
	if s.offline {
		query = `
			SELECT vl.linkageTargetId, vl.description, vl.beginYearMonth, vl.endYearMonth,
			       vl.fuelType, vl.capacityCC, vl.horsePowerFrom, COUNT(*) as cnt
			FROM vehicle_lookup vl
			JOIN hk_parts_cache hk ON hk.linkingTargetId = vl.linkageTargetId
			WHERE vl.nhtsa_make = ? AND vl.nhtsa_model = ?
			  AND ? BETWEEN vl.year_from AND vl.year_to
			GROUP BY vl.linkageTargetId, vl.description, vl.beginYearMonth, vl.endYearMonth,
			         vl.fuelType, vl.capacityCC, vl.horsePowerFrom
			ORDER BY cnt DESC
			LIMIT 20`
	} else {
		query = `
			SELECT lt.linkageTargetId, lt.description, lt.beginYearMonth, lt.endYearMonth,
			       lt.fuelType, lt.capacityCC, lt.horsePowerFrom, COUNT(*) as cnt
			FROM nhtsa_tecdoc_bridge b
			JOIN modelseries ms ON ms.modelId = b.tecdoc_model_id
			JOIN linkagetargets lt ON lt.vehicleModelSeriesId = ms.modelId AND lt.lang = 'en'
			JOIN hk_parts_cache hk ON hk.linkingTargetId = lt.linkageTargetId
			WHERE b.nhtsa_make = ? AND b.nhtsa_model = ?
			  AND ? BETWEEN b.year_from AND b.year_to
			GROUP BY lt.linkageTargetId, lt.description, lt.beginYearMonth, lt.endYearMonth,
			         lt.fuelType, lt.capacityCC, lt.horsePowerFrom
			ORDER BY cnt DESC
			LIMIT 20`
	}

	rows, err := s.db.Query(query, make, modelName, year)
	if err != nil {
		return nil, fmt.Errorf("best target: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		vehicle model.Vehicle
		cnt     int
		score   int
	}
	var candidates []candidate

	for rows.Next() {
		var v model.Vehicle
		var desc, fuel sql.NullString
		var cap, hp sql.NullInt32
		var cnt int
		if err := rows.Scan(&v.LinkageTargetId, &desc, &v.BeginYearMonth, &v.EndYearMonth,
			&fuel, &cap, &hp, &cnt); err != nil {
			return nil, fmt.Errorf("scan candidate: %w", err)
		}
		v.Make = make
		v.Model = modelName
		v.ModelYear = year
		v.Description = desc.String
		v.FuelType = fuel.String
		if cap.Valid {
			v.CapacityCC = int(cap.Int32)
		}
		if hp.Valid {
			v.HorsePower = int(hp.Int32)
		}

		// Score: start with parts count as base
		score := cnt

		// Bonus for matching engine CC (±200cc = +1000, ±500cc = +500)
		if engineCC > 0 && v.CapacityCC > 0 {
			diff := engineCC - v.CapacityCC
			if diff < 0 {
				diff = -diff
			}
			if diff <= 200 {
				score += 1000
			} else if diff <= 500 {
				score += 500
			} else if diff <= 1000 {
				score += 100
			}
			// penalty for large CC mismatch
			if diff > 2000 {
				score -= 500
			}
		}

		// Bonus for matching fuel type
		if fuelType != "" && v.FuelType != "" {
			if fuelMatchScore(fuelType, v.FuelType) {
				score += 800
			}
		}

		candidates = append(candidates, candidate{vehicle: v, cnt: cnt, score: score})
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Pick the highest-scoring candidate
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}
	return &best.vehicle, nil
}

// fuelMatchScore checks if two fuel type strings are compatible.
func fuelMatchScore(nhtsaFuel, tecdocFuel string) bool {
	nf := normFuel(nhtsaFuel)
	tf := normFuel(tecdocFuel)
	if nf == "" || tf == "" {
		return false
	}
	return nf == tf
}

func normFuel(f string) string {
	for i := range f {
		if f[i] >= 'A' && f[i] <= 'Z' {
			f = f[:i] + string(rune(f[i]+32)) + f[i+1:]
		}
	}
	switch {
	case containsIgnoreCase(f, "petrol"), containsIgnoreCase(f, "gasoline"), containsIgnoreCase(f, "benzin"):
		return "petrol"
	case containsIgnoreCase(f, "diesel"):
		return "diesel"
	case containsIgnoreCase(f, "electric"):
		return "electric"
	case containsIgnoreCase(f, "hybrid"):
		return "hybrid"
	case containsIgnoreCase(f, "lpg"):
		return "lpg"
	case containsIgnoreCase(f, "cng"):
		return "cng"
	}
	return ""
}

// ReverseByArticle returns vehicles that use a given article.
func (s *PartsLookup) ReverseByArticle(legacyArticleId int, limit int) ([]model.Vehicle, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var query string
	if s.offline {
		// SQLite: use hk_parts_cache directly (no linkagetargets/modelseries)
		query = `
			SELECT DISTINCT hk.linkingTargetId, hk.vehicleDesc, hk.beginYearMonth, hk.endYearMonth,
			       hk.fuelType, hk.capacityCC, hk.horsePowerFrom,
			       hk.modelName,
			       CASE hk.manuId WHEN 183 THEN 'HYUNDAI' WHEN 184 THEN 'KIA' ELSE '' END
			FROM hk_parts_cache hk
			WHERE hk.legacyArticleId = ?
			ORDER BY CASE hk.manuId WHEN 183 THEN 'HYUNDAI' WHEN 184 THEN 'KIA' ELSE '' END, hk.modelName
			LIMIT ?`
	} else {
		// MySQL: join through linkagetargets → modelseries → manufacturers
		query = `
			SELECT DISTINCT hk.linkingTargetId, lt.description, lt.beginYearMonth, lt.endYearMonth,
			       lt.fuelType, lt.capacityCC, lt.horsePowerFrom,
			       ms.description AS modelName, m.description AS makeName
			FROM hk_parts_cache hk
			JOIN linkagetargets lt ON lt.linkageTargetId = hk.linkingTargetId AND lt.lang = 'en'
			JOIN modelseries ms ON ms.modelId = lt.vehicleModelSeriesId
			JOIN manufacturers m ON m.manuId = ms.manuId AND m.lang = 'en'
			WHERE hk.legacyArticleId = ?
			ORDER BY m.description, ms.description
			LIMIT ?`
	}

	rows, err := s.db.Query(query, legacyArticleId, limit)
	if err != nil {
		return nil, fmt.Errorf("reverse lookup: %w", err)
	}
	defer rows.Close()

	var vehicles []model.Vehicle
	for rows.Next() {
		var v model.Vehicle
		var desc, fuel, modelName, makeName sql.NullString
		var cap, hp sql.NullInt32
		if err := rows.Scan(&v.LinkageTargetId, &desc, &v.BeginYearMonth, &v.EndYearMonth,
			&fuel, &cap, &hp, &modelName, &makeName); err != nil {
			return nil, fmt.Errorf("scan reverse: %w", err)
		}
		v.Description = desc.String
		v.FuelType = fuel.String
		v.Make = makeName.String
		v.Model = modelName.String
		if cap.Valid {
			v.CapacityCC = int(cap.Int32)
		}
		if hp.Valid {
			v.HorsePower = int(hp.Int32)
		}
		vehicles = append(vehicles, v)
	}
	return vehicles, nil
}

// ── Catalog browsing ─────────────────────────────────────────────────

// ModelInfo represents a model with year range and variant count.
type ModelInfo struct {
	Model    string `json:"model"`
	YearFrom int    `json:"yearFrom"`
	YearTo   int    `json:"yearTo"`
	Variants int    `json:"variants"`
}

// ListModels returns distinct models for a make.
func (s *PartsLookup) ListModels(make string) ([]ModelInfo, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	rows, err := s.db.Query(`
		SELECT nhtsa_model, MIN(year_from), MAX(year_to), COUNT(DISTINCT linkageTargetId)
		FROM vehicle_lookup WHERE nhtsa_make = ?
		GROUP BY nhtsa_model ORDER BY nhtsa_model`, make)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var models []ModelInfo
	for rows.Next() {
		var m ModelInfo
		if err := rows.Scan(&m.Model, &m.YearFrom, &m.YearTo, &m.Variants); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, nil
}

// VehicleVariant is a specific engine/year variant for catalog browsing.
type VehicleVariant struct {
	LinkageTargetId int    `json:"linkageTargetId"`
	Description     string `json:"description"`
	FuelType        string `json:"fuelType"`
	CapacityCC      int    `json:"capacityCC"`
	HorsePower      int    `json:"horsePower"`
	YearFrom        int    `json:"yearFrom"`
	YearTo          int    `json:"yearTo"`
}

// ListVehicleVariants returns all engine/year variants for a make+model.
func (s *PartsLookup) ListVehicleVariants(make, model string) ([]VehicleVariant, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	rows, err := s.db.Query(`
		SELECT linkageTargetId, description, fuelType, capacityCC, horsePowerFrom, year_from, year_to
		FROM vehicle_lookup WHERE nhtsa_make = ? AND nhtsa_model = ?
		ORDER BY year_from DESC, capacityCC`, make, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var variants []VehicleVariant
	for rows.Next() {
		var v VehicleVariant
		if err := rows.Scan(&v.LinkageTargetId, &v.Description, &v.FuelType, &v.CapacityCC, &v.HorsePower, &v.YearFrom, &v.YearTo); err != nil {
			return nil, err
		}
		variants = append(variants, v)
	}
	return variants, nil
}

// AssemblyGroup represents a catalog section (e.g., "Front Brake System").
type AssemblyGroup struct {
	GroupId   int    `json:"groupId"`
	GroupName string `json:"groupName"`
	PartCount int    `json:"partCount"`
}

// ListAssemblyGroups returns assembly groups with part counts for a vehicle.
func (s *PartsLookup) ListAssemblyGroups(linkageTargetId int) ([]AssemblyGroup, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	rows, err := s.db.Query(`
		SELECT assemblyGroupNodeId, categoryName, COUNT(DISTINCT legacyArticleId)
		FROM hk_parts_cache WHERE linkingTargetId = ? AND assemblyGroupNodeId > 0
		GROUP BY assemblyGroupNodeId, categoryName
		ORDER BY categoryName`, linkageTargetId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []AssemblyGroup
	for rows.Next() {
		var g AssemblyGroup
		if err := rows.Scan(&g.GroupId, &g.GroupName, &g.PartCount); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// ListPartsByGroup returns parts for a vehicle in a specific assembly group.
func (s *PartsLookup) ListPartsByGroup(linkageTargetId, groupId int) ([]model.Part, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	query := `SELECT DISTINCT legacyArticleId, articleNumber, genericArticleDesc, brandName, categoryName, assemblyGroupNodeId
		FROM hk_parts_cache WHERE linkingTargetId = ?`
	args := []any{linkageTargetId}
	if groupId > 0 {
		query += " AND assemblyGroupNodeId = ?"
		args = append(args, groupId)
	}
	query += " ORDER BY categoryName, genericArticleDesc, brandName"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var parts []model.Part
	for rows.Next() {
		var p model.Part
		var desc, brand, cat sql.NullString
		if err := rows.Scan(&p.LegacyArticleId, &p.ArticleNumber, &desc, &brand, &cat, &p.AssemblyGroupId); err != nil {
			return nil, err
		}
		p.Description = desc.String
		p.BrandName = brand.String
		p.Category = cat.String
		parts = append(parts, p)
	}
	return parts, nil
}

// FindByMotorCodes returns parts for a vehicle that are also linked to specific engine codes.
// Used for ENGINE_STRICT categories where parts must match the actual engine.
// Join path: hk_parts_cache.modelId → cars.modId, cars.carId → vehiclemotorcodes.carId
func (s *PartsLookup) FindByMotorCodes(linkageTargetId int, motorCodes []string, category string, page, limit int) ([]model.Part, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not connected")
	}
	if s.offline {
		// SQLite has no vehiclemotorcodes — fall back to standard lookup
		return s.FindByLinkageTarget(linkageTargetId, category, page, limit)
	}
	if len(motorCodes) == 0 {
		return s.FindByLinkageTarget(linkageTargetId, category, page, limit)
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	// Build motor code placeholders
	placeholders := make([]string, len(motorCodes))
	mcArgs := make([]any, len(motorCodes))
	for i, mc := range motorCodes {
		placeholders[i] = "?"
		mcArgs[i] = mc
	}
	mcIn := "(" + joinStrings(placeholders, ",") + ")"

	// Count query
	countSQL := `SELECT COUNT(DISTINCT hk.legacyArticleId)
		FROM hk_parts_cache hk
		JOIN cars c ON hk.modelId = c.modId AND c.manuId IN (183, 184)
		JOIN vehiclemotorcodes vmc ON c.carId = vmc.carId
		WHERE hk.linkingTargetId = ? AND vmc.motorCode IN ` + mcIn
	countArgs := append([]any{linkageTargetId}, mcArgs...)
	if category != "" {
		countSQL += " AND hk.genericArticleDesc LIKE ?"
		countArgs = append(countArgs, "%"+category+"%")
	}

	var total int
	if err := s.db.QueryRow(countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count strict parts: %w", err)
	}

	// Data query
	dataSQL := `SELECT DISTINCT hk.legacyArticleId, hk.articleNumber, hk.genericArticleDesc,
		hk.brandName, hk.categoryName, hk.assemblyGroupNodeId
		FROM hk_parts_cache hk
		JOIN cars c ON hk.modelId = c.modId AND c.manuId IN (183, 184)
		JOIN vehiclemotorcodes vmc ON c.carId = vmc.carId
		WHERE hk.linkingTargetId = ? AND vmc.motorCode IN ` + mcIn
	dataArgs := append([]any{linkageTargetId}, mcArgs...)
	if category != "" {
		dataSQL += " AND hk.genericArticleDesc LIKE ?"
		dataArgs = append(dataArgs, "%"+category+"%")
	}
	dataSQL += " ORDER BY hk.categoryName, hk.brandName LIMIT ? OFFSET ?"
	dataArgs = append(dataArgs, limit, offset)

	rows, err := s.db.Query(dataSQL, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query strict parts: %w", err)
	}
	defer rows.Close()

	var parts []model.Part
	for rows.Next() {
		var p model.Part
		var desc, brand, cat sql.NullString
		if err := rows.Scan(&p.LegacyArticleId, &p.ArticleNumber, &desc, &brand, &cat, &p.AssemblyGroupId); err != nil {
			return nil, 0, fmt.Errorf("scan strict part: %w", err)
		}
		p.Description = desc.String
		p.BrandName = brand.String
		p.Category = cat.String
		parts = append(parts, p)
	}
	return parts, total, nil
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}

// CountSharedParts counts how many distinct parts are shared between a vehicle
// (by linkageTargetId) and another make/model's vehicles.
func (s *PartsLookup) CountSharedParts(linkageTargetId int, siblingMake, siblingModel string) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not connected")
	}

	// In SQLite mode, use hk_parts_cache + vehicle_lookup
	query := `
		SELECT COUNT(DISTINCT a.legacyArticleId)
		FROM hk_parts_cache a
		JOIN hk_parts_cache b ON a.legacyArticleId = b.legacyArticleId
		JOIN vehicle_lookup v ON b.linkingTargetId = v.linkageTargetId
		WHERE a.linkingTargetId = ?
		  AND UPPER(v.make) = UPPER(?)
		  AND UPPER(v.model) = UPPER(?)`

	var count int
	err := s.db.QueryRow(query, linkageTargetId, siblingMake, siblingModel).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count shared parts: %w", err)
	}
	return count, nil
}

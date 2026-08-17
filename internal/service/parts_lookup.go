package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

// PartWithOEM extends a Part with optional OEM numbers for enriched responses.
type PartWithOEM struct {
	model.Part
	OEMNumbers []string `json:"oemNumbers,omitempty"`
}

// PartsLookup queries the PostgreSQL-backed catalog cache tables.
type PartsLookup struct {
	db      *sql.DB
	queries *store.Queries
}

func NewPartsLookup(db *sql.DB, offline bool) *PartsLookup {
	_ = offline
	if db == nil {
		return &PartsLookup{}
	}
	return &PartsLookup{db: db, queries: store.New(db)}
}

// FindByArticle returns a single representative part record for a legacyArticleId.
func (s *PartsLookup) FindByArticle(legacyArticleId int, linkageTargetId int) (*model.Part, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}

	ctx := context.Background()
	if linkageTargetId > 0 {
		row, err := s.queries.FindPartByArticleForVehicle(ctx, store.FindPartByArticleForVehicleParams{
			LegacyArticleID: int32(legacyArticleId),
			LinkingTargetID: int32(linkageTargetId),
		})
		if err == nil {
			return mapStorePart(row.LegacyArticleID, row.ArticleNumber, row.GenericArticleDesc, row.BrandName, row.CategoryName, row.AssemblyGroupNodeID), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("find article: %w", err)
		}
	}

	row, err := s.queries.FindPartByArticleAnyVehicle(ctx, int32(legacyArticleId))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find article: %w", err)
	}
	return mapStorePart(row.LegacyArticleID, row.ArticleNumber, row.GenericArticleDesc, row.BrandName, row.CategoryName, row.AssemblyGroupNodeID), nil
}

// FindByArticleNumber returns exact catalog part-number matches. When a vehicle
// context is supplied, results not linked to that vehicle are withheld.
func (s *PartsLookup) FindByArticleNumber(articleNumber string, linkageTargetId, limit int) ([]model.Part, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	articleNumber = strings.TrimSpace(articleNumber)
	if articleNumber == "" {
		return nil, nil
	}

	rows, err := s.queries.SearchByArticleNumber(context.Background(), store.SearchByArticleNumberParams{
		Upper: strings.ToUpper(articleNumber),
		Limit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("find article number: %w", err)
	}

	// S2-T2 (BUG-4): if exact match finds nothing, try the normalized form.
	if len(rows) == 0 {
		normalized := NormalizeOEM(articleNumber)
		if normalized != strings.ToLower(articleNumber) {
			rows2, err2 := s.queries.SearchByArticleNumber(context.Background(), store.SearchByArticleNumberParams{
				Upper: strings.ToUpper(normalized),
				Limit: int32(limit),
			})
			if err2 == nil {
				rows = rows2
			}
		}
	}

	// BUG-6 stem lookup: for a 5-digit all-numeric stem (e.g. "97133"), also
	// search by OEM prefix so that "97133" finds "97133-D3000" etc.
	if len(rows) == 0 {
		trimmed := strings.TrimSpace(articleNumber)
		isNumericStem := len(trimmed) == 5
		if isNumericStem {
			for _, c := range trimmed {
				if c < '0' || c > '9' {
					isNumericStem = false
					break
				}
			}
		}
		if isNumericStem {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			prefixRows, prefixErr := s.queries.SearchOEMPrefix(ctx, store.SearchOEMPrefixParams{
				Normalized: strings.ToLower(trimmed) + "%",
				Limit:      int32(limit),
			})
			if prefixErr == nil {
				for _, pr := range prefixRows {
					if pr.LegacyArticleID > 0 {
						rows = append(rows, store.SearchByArticleNumberRow{
							LegacyArticleID:    pr.LegacyArticleID,
							ArticleNumber:      pr.ArticleNumber,
							GenericArticleDesc: pr.Description,
							BrandName:          pr.BrandName,
						})
					}
				}
			}
		}
	}

	parts := make([]model.Part, 0, len(rows))
	for _, row := range rows {
		if linkageTargetId > 0 {
			fits, err := s.queries.CheckPartFitsVehicle(context.Background(), store.CheckPartFitsVehicleParams{
				LinkingTargetID: int32(linkageTargetId),
				LegacyArticleID: row.LegacyArticleID,
			})
			if err != nil {
				return nil, fmt.Errorf("check part fitment: %w", err)
			}
			if !fits {
				continue
			}
		}
		parts = append(parts, *mapStorePart(
			row.LegacyArticleID,
			row.ArticleNumber,
			row.GenericArticleDesc,
			row.BrandName,
			row.CategoryName,
			row.AssemblyGroupNodeID,
		))
	}
	return parts, nil
}

// FindByLinkageTarget returns parts for a specific vehicle variant.
func (s *PartsLookup) FindByLinkageTarget(linkageTargetId int, category string, page, limit int) ([]model.Part, int, error) {
	if s.queries == nil {
		return nil, 0, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit
	ctx := context.Background()

	total, err := s.queries.CountPartsByVehicleCategory(ctx, store.CountPartsByVehicleCategoryParams{
		LinkingTargetID: int32(linkageTargetId),
		Column2:         category,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count parts: %w", err)
	}

	rows, err := s.queries.ListPartsByVehicleCategory(ctx, store.ListPartsByVehicleCategoryParams{
		LinkingTargetID: int32(linkageTargetId),
		Column2:         category,
		Limit:           int32(limit),
		Offset:          int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("query parts: %w", err)
	}

	parts := make([]model.Part, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, *mapStorePart(row.LegacyArticleID, row.ArticleNumber, row.GenericArticleDesc, row.BrandName, row.CategoryName, row.AssemblyGroupNodeID))
	}
	return parts, int(total), nil
}

// ResolveLinkageTargets finds linkageTargetIds matching NHTSA make/model/year.
func (s *PartsLookup) ResolveLinkageTargets(vehicleMake, modelName string, year int) ([]model.Vehicle, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}

	rows, err := s.queries.ResolveLinkageTargets(context.Background(), store.ResolveLinkageTargetsParams{
		NhtsaMake:  vehicleMake,
		NhtsaModel: modelName,
		YearFrom:   int32(year),
	})
	if err != nil {
		return nil, fmt.Errorf("resolve targets: %w", err)
	}

	vehicles := make([]model.Vehicle, 0, len(rows))
	for _, row := range rows {
		v := model.Vehicle{
			LinkageTargetId: int(row.LinkageTargetID),
			Make:            vehicleMake,
			Model:           modelName,
			ModelYear:       year,
			Description:     row.Description,
			BeginYearMonth:  parseYearMonth(row.BeginYearMonth.String),
			EndYearMonth:    parseYearMonth(row.EndYearMonth.String),
			FuelType:        row.FuelType.String,
		}
		if row.CapacityCc.Valid {
			v.CapacityCC = int(row.CapacityCc.Int32)
		}
		if row.HorsePowerFrom.Valid {
			v.HorsePower = int(row.HorsePowerFrom.Int32)
		}
		vehicles = append(vehicles, v)
	}
	return vehicles, nil
}

func (s *PartsLookup) BestLinkageTarget(vehicleMake, modelName string, year int) (*model.Vehicle, error) {
	return s.BestLinkageTargetWithHints(vehicleMake, modelName, year, 0, "")
}

// BestLinkageTargetWithHints picks the best variant using engine CC and fuel type hints.
func (s *PartsLookup) BestLinkageTargetWithHints(vehicleMake, modelName string, year int, engineCC int, fuelType string) (*model.Vehicle, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}

	rows, err := s.queries.BestLinkageTargetCandidates(context.Background(), store.BestLinkageTargetCandidatesParams{
		NhtsaMake:  vehicleMake,
		NhtsaModel: modelName,
		YearFrom:   int32(year),
	})
	if err != nil {
		return nil, fmt.Errorf("best target: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	type candidate struct {
		vehicle model.Vehicle
		score   int
	}

	var candidates []candidate
	for _, row := range rows {
		v := model.Vehicle{
			LinkageTargetId: int(row.LinkageTargetID),
			Make:            vehicleMake,
			Model:           modelName,
			ModelYear:       year,
			Description:     row.Description,
			BeginYearMonth:  parseYearMonth(row.BeginYearMonth.String),
			EndYearMonth:    parseYearMonth(row.EndYearMonth.String),
			FuelType:        row.FuelType.String,
		}
		if row.CapacityCc.Valid {
			v.CapacityCC = int(row.CapacityCc.Int32)
		}
		if row.HorsePowerFrom.Valid {
			v.HorsePower = int(row.HorsePowerFrom.Int32)
		}

		score := int(row.PartCount)
		if engineCC > 0 && v.CapacityCC > 0 {
			diff := engineCC - v.CapacityCC
			if diff < 0 {
				diff = -diff
			}
			switch {
			case diff <= 200:
				score += 1000
			case diff <= 500:
				score += 500
			case diff <= 1000:
				score += 100
			case diff > 2000:
				score -= 500
			}
		}
		if fuelType != "" && v.FuelType != "" && fuelMatchScore(fuelType, v.FuelType) {
			score += 800
		}
		candidates = append(candidates, candidate{vehicle: v, score: score})
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}
	return &best.vehicle, nil
}

func fuelMatchScore(nhtsaFuel, tecdocFuel string) bool {
	nf := normFuel(nhtsaFuel)
	tf := normFuel(tecdocFuel)
	return nf != "" && tf != "" && nf == tf
}

func normFuel(f string) string {
	f = strings.ToLower(f)
	switch {
	case strings.Contains(f, "petrol"), strings.Contains(f, "gasoline"), strings.Contains(f, "benzin"):
		return "petrol"
	case strings.Contains(f, "diesel"):
		return "diesel"
	case strings.Contains(f, "electric"):
		return "electric"
	case strings.Contains(f, "hybrid"):
		return "hybrid"
	case strings.Contains(f, "lpg"):
		return "lpg"
	case strings.Contains(f, "cng"):
		return "cng"
	}
	return ""
}

// ReverseByArticle returns vehicles that use a given article.
func (s *PartsLookup) ReverseByArticle(legacyArticleId int, limit int) ([]model.Vehicle, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.queries.ReverseVehiclesByArticle(context.Background(), store.ReverseVehiclesByArticleParams{
		LegacyArticleID: int32(legacyArticleId),
		Limit:           int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("reverse lookup: %w", err)
	}

	vehicles := make([]model.Vehicle, 0, len(rows))
	for _, row := range rows {
		v := model.Vehicle{
			LinkageTargetId: int(row.LinkingTargetID),
			Description:     row.VehicleDesc.String,
			BeginYearMonth:  parseYearMonth(row.BeginYearMonth.String),
			EndYearMonth:    parseYearMonth(row.EndYearMonth.String),
			FuelType:        row.FuelType.String,
			Model:           row.ModelName.String,
			Make:            row.MakeName,
		}
		if row.CapacityCc.Valid {
			v.CapacityCC = int(row.CapacityCc.Int32)
		}
		if row.HorsePowerFrom.Valid {
			v.HorsePower = int(row.HorsePowerFrom.Int32)
		}
		vehicles = append(vehicles, v)
	}
	return vehicles, nil
}

type ModelInfo struct {
	Model    string `json:"model"`
	YearFrom int    `json:"yearFrom"`
	YearTo   int    `json:"yearTo"`
	Variants int    `json:"variants"`
}

func (s *PartsLookup) ListModels(vehicleMake string) ([]ModelInfo, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	rows, err := s.queries.ListModelsByMake(context.Background(), vehicleMake)
	if err != nil {
		return nil, err
	}
	models := make([]ModelInfo, 0, len(rows))
	for _, row := range rows {
		models = append(models, ModelInfo{
			Model:    row.NhtsaModel,
			YearFrom: int(row.YearFrom),
			YearTo:   int(row.YearTo),
			Variants: int(row.Variants),
		})
	}
	return models, nil
}

type VehicleVariant struct {
	LinkageTargetId int    `json:"linkageTargetId"`
	Description     string `json:"description"`
	FuelType        string `json:"fuelType"`
	CapacityCC      int    `json:"capacityCC"`
	HorsePower      int    `json:"horsePower"`
	YearFrom        int    `json:"yearFrom"`
	YearTo          int    `json:"yearTo"`
}

func (s *PartsLookup) ListVehicleVariants(vehicleMake, modelName string) ([]VehicleVariant, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	rows, err := s.queries.ListVehicleVariantsByMakeModel(context.Background(), store.ListVehicleVariantsByMakeModelParams{
		NhtsaMake:  vehicleMake,
		NhtsaModel: modelName,
	})
	if err != nil {
		return nil, err
	}
	out := make([]VehicleVariant, 0, len(rows))
	for _, row := range rows {
		out = append(out, VehicleVariant{
			LinkageTargetId: int(row.LinkageTargetID),
			Description:     row.Description,
			FuelType:        row.FuelType,
			CapacityCC:      int(row.CapacityCc),
			HorsePower:      int(row.HorsePowerFrom),
			YearFrom:        int(row.YearFrom),
			YearTo:          int(row.YearTo),
		})
	}
	return out, nil
}

type AssemblyGroup struct {
	GroupId   int    `json:"groupId"`
	GroupName string `json:"groupName"`
	PartCount int    `json:"partCount"`
}

func (s *PartsLookup) ListAssemblyGroups(linkageTargetId int) ([]AssemblyGroup, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	rows, err := s.queries.ListAssemblyGroupsByVehicle(context.Background(), int32(linkageTargetId))
	if err != nil {
		return nil, err
	}
	out := make([]AssemblyGroup, 0, len(rows))
	for _, row := range rows {
		out = append(out, AssemblyGroup{
			GroupId:   int(row.AssemblyGroupNodeID),
			GroupName: row.CategoryName,
			PartCount: int(row.PartCount),
		})
	}
	return out, nil
}

func (s *PartsLookup) ListPartsByGroup(linkageTargetId, groupId int) ([]model.Part, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	rows, err := s.queries.ListPartsByGroup(context.Background(), store.ListPartsByGroupParams{
		LinkingTargetID: int32(linkageTargetId),
		Column2:         int32(groupId),
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.Part, 0, len(rows))
	for _, row := range rows {
		out = append(out, *mapStorePart(row.LegacyArticleID, row.ArticleNumber, row.GenericArticleDesc, row.BrandName, row.CategoryName, row.AssemblyGroupNodeID))
	}
	return out, nil
}

func (s *PartsLookup) CountSharedParts(linkageTargetId int, siblingMake, siblingModel string) (int, error) {
	if s.queries == nil {
		return 0, fmt.Errorf("database not connected")
	}
	row := s.db.QueryRow(`
		SELECT COUNT(DISTINCT a.legacy_article_id)
		FROM hk_parts_cache a
		JOIN hk_parts_cache b ON a.legacy_article_id = b.legacy_article_id
		JOIN vehicle_lookup v ON b.linking_target_id = v.linking_target_id
		WHERE a.linking_target_id = $1
		  AND UPPER(v.nhtsa_make) = UPPER($2)
		  AND UPPER(v.nhtsa_model) = UPPER($3)
	`, linkageTargetId, siblingMake, siblingModel)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count shared parts: %w", err)
	}
	return count, nil
}

func mapStorePart(id int32, article, desc, brand, category sql.NullString, assemblyGroupID int32) *model.Part {
	return &model.Part{
		LegacyArticleId: int(id),
		ArticleNumber:   article.String,
		Description:     desc.String,
		BrandName:       brand.String,
		Category:        category.String,
		AssemblyGroupId: int(assemblyGroupID),
	}
}

func parseYearMonth(v string) int {
	var out int
	for _, c := range v {
		if c < '0' || c > '9' {
			return 0
		}
		out = out*10 + int(c-'0')
	}
	return out
}

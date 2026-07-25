package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

type CrossRef struct {
	db      *sql.DB
	queries *store.Queries
}

func NewCrossRef(db *sql.DB, offline bool) *CrossRef {
	_ = offline
	if db == nil {
		return &CrossRef{}
	}
	return &CrossRef{db: db, queries: store.New(db)}
}

// SetLocalDB is kept only for call-site compatibility during the migration.
func (s *CrossRef) SetLocalDB(localDB *sql.DB) {
	_ = localDB
}

func (s *CrossRef) FindOEMNumbers(legacyArticleId int) ([]model.OEMReference, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}

	rows, err := s.queries.FindOEMReferencesForArticle(context.Background(), int32(legacyArticleId))
	if err != nil {
		return nil, fmt.Errorf("find OEM refs: %w", err)
	}

	refs := make([]model.OEMReference, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, model.OEMReference{
			RawNumber:       row.RawNumber,
			LegacyArticleId: legacyArticleId,
			Manufacturer:    row.MfrName.String,
			ArticleNumber:   row.ArticleNumber.String,
			Description:     row.Description.String,
		})
	}
	return refs, nil
}

func (s *CrossRef) FindByOEM(oemNumber string, limit int) ([]model.OEMReference, error) {
	start := time.Now()
	log.Printf("[CrossRef.FindByOEM] START oem=%q limit=%d", oemNumber, limit)

	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	normalized := NormalizeOEM(oemNumber)
	rows, err := s.queries.SearchOEMByNormalized(context.Background(), store.SearchOEMByNormalizedParams{
		Normalized: normalized,
		Limit:      int32(limit * 3),
	})
	if err != nil {
		return nil, fmt.Errorf("find by OEM: %w", err)
	}

	var refs []model.OEMReference
	seenArticle := make(map[int]bool)
	for _, row := range rows {
		ref := model.OEMReference{
			RawNumber:       row.RawNumber,
			LegacyArticleId: int(row.LegacyArticleID),
			Manufacturer:    row.MfrName.String,
			BrandName:       row.BrandName.String,
			ArticleNumber:   row.ArticleNumber.String,
			Description:     row.Description.String,
		}
		if NormalizeOEM(ref.ArticleNumber) == normalized || seenArticle[ref.LegacyArticleId] {
			continue
		}
		seenArticle[ref.LegacyArticleId] = true
		refs = append(refs, ref)
		if len(refs) >= limit {
			break
		}
	}

	log.Printf("[CrossRef.FindByOEM] DONE results=%d elapsed=%v", len(refs), time.Since(start))
	return refs, nil
}

func (s *CrossRef) FindVehiclesForArticle(legacyArticleId int, vehicleCC int, category string, limit int) ([]model.Vehicle, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.queries.FindVehiclesForArticle(context.Background(), store.FindVehiclesForArticleParams{
		LegacyArticleID: int32(legacyArticleId),
		Limit:           int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("vehicles for article: %w", err)
	}

	rule := ClassifyCategory(category)
	var vehicles []model.Vehicle
	for _, row := range rows {
		v := model.Vehicle{
			LinkageTargetId: int(row.LinkingTargetID),
			Description:     row.VehicleDesc.String,
			BeginYearMonth:  parseYearMonth(row.BeginYearMonth.String),
			EndYearMonth:    parseYearMonth(row.EndYearMonth.String),
			FuelType:        row.FuelType.String,
			Make:            row.MakeName,
		}
		if row.CapacityCc.Valid {
			v.CapacityCC = int(row.CapacityCc.Int32)
		}
		if row.HorsePowerFrom.Valid {
			v.HorsePower = int(row.HorsePowerFrom.Int32)
		}

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
				continue
			}
		}

		vehicles = append(vehicles, v)
	}
	return vehicles, nil
}

// SQLite-only aftermarket reference data is intentionally not migrated in Story 4.
func (s *CrossRef) FindAftermarketByOEM(oemNumber string) ([]model.AftermarketPart, error) {
	_ = oemNumber
	return nil, nil
}

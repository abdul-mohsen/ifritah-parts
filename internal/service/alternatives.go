package service

import (
	"context"
	"database/sql"
	"fmt"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

type Alternatives struct {
	db      *sql.DB
	queries *store.Queries
}

func NewAlternatives(db *sql.DB, offline bool) *Alternatives {
	_ = offline
	if db == nil {
		return &Alternatives{}
	}
	return &Alternatives{db: db, queries: store.New(db)}
}

type AlternativePart struct {
	model.Part
	SharedVehicles int `json:"sharedVehicles,omitempty"`
}

func (a *Alternatives) FindForArticle(legacyArticleId int, linkageTargetId int, limit int) ([]AlternativePart, error) {
	if a.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	desc, err := a.queries.LookupAlternativeDescription(context.Background(), int32(legacyArticleId))
	if err != nil || !desc.Valid || desc.String == "" {
		return nil, nil
	}

	var out []AlternativePart
	if linkageTargetId > 0 {
		rows, err := a.queries.FindAlternativesForArticleVehicle(context.Background(), store.FindAlternativesForArticleVehicleParams{
			GenericArticleDesc: desc,
			LinkingTargetID:    int32(linkageTargetId),
			LegacyArticleID:    int32(legacyArticleId),
			Limit:              int32(limit),
		})
		if err != nil {
			return nil, fmt.Errorf("alternatives query: %w", err)
		}
		for _, row := range rows {
			out = append(out, AlternativePart{
				Part: *mapStorePart(row.LegacyArticleID, row.ArticleNumber, row.GenericArticleDesc, row.BrandName, row.CategoryName, row.AssemblyGroupNodeID),
			})
		}
		return out, nil
	}

	rows, err := a.queries.FindAlternativesForArticleAnyVehicle(context.Background(), store.FindAlternativesForArticleAnyVehicleParams{
		GenericArticleDesc: desc,
		LegacyArticleID:    int32(legacyArticleId),
		Limit:              int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("alternatives query: %w", err)
	}
	for _, row := range rows {
		out = append(out, AlternativePart{
			Part: *mapStorePart(row.LegacyArticleID, row.ArticleNumber, row.GenericArticleDesc, row.BrandName, row.CategoryName, row.AssemblyGroupNodeID),
		})
	}
	return out, nil
}

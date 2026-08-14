package service

import (
	"database/sql"
	"fmt"

	"parts-engine/internal/model"
)

// Alternatives finds functionally equivalent parts for a given article.
// Two parts are "also compatible" if they share the same genericArticleDesc
// (same function) and both fit the same vehicle (linkageTargetId).
type Alternatives struct {
	db      *sql.DB
	offline bool
}

func NewAlternatives(db *sql.DB, offline bool) *Alternatives {
	return &Alternatives{db: db, offline: offline}
}

// AlternativePart extends Part with compatibility context.
type AlternativePart struct {
	model.Part
	SharedVehicles int `json:"sharedVehicles,omitempty"`
}

// FindForArticle returns parts that serve the same function and fit the same vehicle.
// If linkageTargetId > 0, only alternatives for that vehicle are returned.
func (a *Alternatives) FindForArticle(legacyArticleId int, linkageTargetId int, limit int) ([]AlternativePart, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	// Step 1: Find the genericArticleDesc of the source article
	var desc sql.NullString
	err := a.db.QueryRow(`SELECT genericArticleDesc FROM hk_parts_cache WHERE legacyArticleId = ? LIMIT 1`, legacyArticleId).Scan(&desc)
	if err != nil || !desc.Valid || desc.String == "" {
		return nil, nil // No description → can't find alternatives
	}

	// Step 2: Find other articles with same description
	var query string
	var args []any

	if linkageTargetId > 0 {
		// Same function + same vehicle
		query = `
			SELECT DISTINCT hk.legacyArticleId, hk.articleNumber, hk.genericArticleDesc,
			       hk.brandName, hk.categoryName, hk.assemblyGroupNodeId
			FROM hk_parts_cache hk
			WHERE hk.genericArticleDesc = ?
			  AND hk.linkingTargetId = ?
			  AND hk.legacyArticleId != ?
			ORDER BY hk.brandName
			LIMIT ?`
		args = []any{desc.String, linkageTargetId, legacyArticleId, limit}
	} else {
		// Same function, any vehicle
		query = `
			SELECT DISTINCT hk.legacyArticleId, hk.articleNumber, hk.genericArticleDesc,
			       hk.brandName, hk.categoryName, hk.assemblyGroupNodeId
			FROM hk_parts_cache hk
			WHERE hk.genericArticleDesc = ?
			  AND hk.legacyArticleId != ?
			ORDER BY hk.brandName
			LIMIT ?`
		args = []any{desc.String, legacyArticleId, limit}
	}

	rows, err := a.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("alternatives query: %w", err)
	}
	defer rows.Close()

	var alts []AlternativePart
	for rows.Next() {
		var ap AlternativePart
		var d, brand, cat sql.NullString
		if err := rows.Scan(&ap.LegacyArticleId, &ap.ArticleNumber, &d, &brand, &cat, &ap.AssemblyGroupId); err != nil {
			continue
		}
		ap.Description = d.String
		ap.BrandName = brand.String
		ap.Category = cat.String
		alts = append(alts, ap)
	}

	return alts, nil
}

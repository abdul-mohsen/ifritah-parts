package service

import (
	"database/sql"
	"fmt"

	"parts-engine/internal/model"
)

// Supersession follows replacement chains for a part.
type Supersession struct {
	db *sql.DB
}

func NewSupersession(db *sql.DB) *Supersession {
	return &Supersession{db: db}
}

// GetChain returns the full supersession chain for a part (both directions).
// In offline mode (SQLite), returns empty since supersession tables aren't exported.
func (s *Supersession) GetChain(legacyArticleId int) ([]model.SupersessionLink, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	var chain []model.SupersessionLink

	// Forward: what replaces this part
	fwd := `SELECT rba.legacyArticleId, rba.articleNumber, ab.brandName
	        FROM replacedbyarticles rba
	        LEFT JOIN articles a ON a.legacyArticleId = rba.legacyArticleId
	        LEFT JOIN ambrand ab ON ab.brandId = a.mfrId AND ab.lang = 'en'
	        WHERE rba.legacyArticleId = ?`

	rows, err := s.db.Query(fwd, legacyArticleId)
	if err != nil {
		// Graceful fallback for SQLite (tables not exported)
		return nil, nil
	}
	defer rows.Close()

	for rows.Next() {
		var link model.SupersessionLink
		var brand sql.NullString
		if err := rows.Scan(&link.LegacyArticleId, &link.ArticleNumber, &brand); err != nil {
			return nil, fmt.Errorf("scan supersession: %w", err)
		}
		link.Direction = "replaced_by"
		link.BrandName = brand.String
		chain = append(chain, link)
	}

	// Backward: what this part replaces
	bwd := `SELECT ra.legacyArticleId, ra.articleNumber, ab.brandName
	        FROM replacesarticles ra
	        LEFT JOIN articles a ON a.legacyArticleId = ra.legacyArticleId
	        LEFT JOIN ambrand ab ON ab.brandId = a.mfrId AND ab.lang = 'en'
	        WHERE ra.legacyArticleId = ?`

	rows2, err := s.db.Query(bwd, legacyArticleId)
	if err != nil {
		// Graceful fallback for SQLite (tables not exported)
		return chain, nil
	}
	defer rows2.Close()

	for rows2.Next() {
		var link model.SupersessionLink
		var brand sql.NullString
		if err := rows2.Scan(&link.LegacyArticleId, &link.ArticleNumber, &brand); err != nil {
			return nil, fmt.Errorf("scan supersession: %w", err)
		}
		link.Direction = "replaces"
		link.BrandName = brand.String
		chain = append(chain, link)
	}

	return chain, nil
}

package service

import (
	"context"
	"database/sql"
	"fmt"

	"parts-engine/internal/model"
)

// documentRepo is the injectable DB dep for TecDocDocuments.
type documentRepo interface {
	QueryDocuments(ctx context.Context, legacyArticleId int) ([]documentRow, error)
}

type documentRow struct {
	URL      string
	FileName string
	DocType  string
	Language string
}

// TecDocDocuments reads articledocs (8.2M rows) into structured
// model.Document values.
//
// BUGS.md § "Rejected for ingestion" forbids populating this type from
// dealer pages or unlicensed marketing media; every returned row is
// stamped with LicensedSource="tecdoc:articledocs" so the media-review
// workflow can enforce provenance downstream.
type TecDocDocuments struct {
	repo documentRepo
}

func NewTecDocDocuments(db *sql.DB) *TecDocDocuments {
	if db == nil {
		return &TecDocDocuments{}
	}
	return &TecDocDocuments{repo: &sqlDocumentRepo{db: db}}
}

// FindDocuments returns TecDoc-licensed documents for the given article.
// Language is not filtered here — the caller (usually a handler) is
// responsible for picking the best-match locale from Accept-Language.
func (s *TecDocDocuments) FindDocuments(legacyArticleId int) ([]model.Document, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if legacyArticleId <= 0 {
		return nil, fmt.Errorf("invalid legacyArticleId: %d", legacyArticleId)
	}

	rows, err := s.repo.QueryDocuments(context.Background(), legacyArticleId)
	if err != nil {
		return nil, fmt.Errorf("find documents: %w", err)
	}

	docs := make([]model.Document, 0, len(rows))
	for _, r := range rows {
		if r.URL == "" {
			continue
		}
		docs = append(docs, model.Document{
			LegacyArticleId: legacyArticleId,
			URL:             r.URL,
			FileName:        r.FileName,
			DocType:         r.DocType,
			Language:        r.Language,
			LicensedSource:  "tecdoc:articledocs",
		})
	}
	return docs, nil
}

type sqlDocumentRepo struct {
	db *sql.DB
}

func (r *sqlDocumentRepo) QueryDocuments(ctx context.Context, legacyArticleId int) ([]documentRow, error) {
	const q = `
		SELECT
			COALESCE(ad.docUrl, ''),
			COALESCE(ad.fileName, ''),
			COALESCE(ad.docType, ''),
			COALESCE(ad.language, '')
		FROM articledocs ad
		WHERE ad.legacyArticleId = ?
		ORDER BY ad.docType, ad.language
		LIMIT 100`

	rows, err := logQueryCtx(r.db, ctx, "TecDocDocuments.FindDocuments", q, legacyArticleId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []documentRow
	for rows.Next() {
		var row documentRow
		if err := rows.Scan(&row.URL, &row.FileName, &row.DocType, &row.Language); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

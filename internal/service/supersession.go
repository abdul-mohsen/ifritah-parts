package service

import (
	"context"
	"database/sql"
	"fmt"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

type substitutionLinkFinder interface {
	ListSubstitutionLinksForArticle(context.Context, int32) ([]store.ListSubstitutionLinksForArticleRow, error)
}

type Supersession struct {
	queries substitutionLinkFinder
}

func NewSupersession(db *sql.DB) *Supersession {
	if db == nil {
		return &Supersession{}
	}
	return &Supersession{queries: store.New(db)}
}

func (s *Supersession) GetChain(legacyArticleId int) ([]model.SupersessionLink, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}

	rows, err := s.queries.ListSubstitutionLinksForArticle(context.Background(), int32(legacyArticleId))
	if err != nil {
		return nil, fmt.Errorf("list substitution links: %w", err)
	}

	links := make([]model.SupersessionLink, 0, len(rows))
	for _, row := range rows {
		link := model.SupersessionLink{
			ArticleNumber: row.ArticleNumber,
			Description:   row.Description,
			Direction:     row.Direction,
			Confidence:    row.Confidence,
			Source: model.ReplacementSource{
				Kind:   row.SourceKey,
				Label:  row.SourceLabel,
				Detail: row.SourceDetail,
			},
		}
		if row.SourceWarning != "" {
			link.Warnings = []string{row.SourceWarning}
		}
		links = append(links, link)
	}
	return links, nil
}

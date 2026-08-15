package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

var stripChars = regexp.MustCompile(`[-.\s/]`)

type OEMLookup struct {
	db      *sql.DB
	queries *store.Queries
}

func NewOEMLookup(db *sql.DB) *OEMLookup {
	if db == nil {
		return &OEMLookup{}
	}
	return &OEMLookup{db: db, queries: store.New(db)}
}

func NormalizeOEM(raw string) string {
	return strings.ToLower(stripChars.ReplaceAllString(raw, ""))
}

func (s *OEMLookup) Search(oemNumber string, limit int) (*model.OEMSearchResult, error) {
	start := time.Now()
	log.Printf("[OEMLookup.Search] START oem=%q limit=%d", oemNumber, limit)

	if s.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	normalized := NormalizeOEM(oemNumber)
	if normalized == "" {
		return nil, fmt.Errorf("empty OEM number")
	}

	rows, err := s.queries.SearchOEMByNormalized(context.Background(), store.SearchOEMByNormalizedParams{
		Normalized: normalized,
		Limit:      int32(limit * 3),
	})
	if err != nil {
		return nil, fmt.Errorf("OEM search: %w", err)
	}

	results := make([]model.OEMReference, 0, len(rows))
	seenArticle := make(map[int]bool)
	for _, row := range rows {
		ref := model.OEMReference{
			RawNumber:       row.RawNumber,
			Normalized:      row.Normalized,
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
		results = append(results, ref)
		if len(results) >= limit {
			break
		}
	}

	log.Printf("[OEMLookup.Search] DONE results=%d elapsed=%v", len(results), time.Since(start))
	return &model.OEMSearchResult{
		Query:      oemNumber,
		Normalized: normalized,
		Results:    results,
		Total:      len(results),
	}, nil
}

func (s *OEMLookup) OEMNumbersForArticle(legacyArticleId int) ([]string, error) {
	if s.queries == nil {
		return nil, nil
	}
	return s.queries.ListOEMRawNumbersForArticle(context.Background(), int32(legacyArticleId))
}

func (s *OEMLookup) BatchOEMNumbers(articleIds []int) (map[int][]string, error) {
	if s.queries == nil || len(articleIds) == 0 {
		return nil, nil
	}
	result := make(map[int][]string, len(articleIds))
	for _, id := range articleIds {
		nums, err := s.OEMNumbersForArticle(id)
		if err != nil {
			return nil, err
		}
		if len(nums) > 0 {
			result[id] = nums
		}
	}
	return result, nil
}

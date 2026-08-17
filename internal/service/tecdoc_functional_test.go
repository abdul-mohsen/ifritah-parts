package service

import (
	"context"
	"errors"
	"testing"
)

type stubFunctionalRepo struct {
	genericId       int
	genericErr      error
	rows            []functionalRow
	sameErr         error
	lastGenericArg  int
	lastExcludeArg  int
	lastVehicleArg  int
	lastLimit       int
	genericCalls    int
	sameGenericCalls int
}

func (s *stubFunctionalRepo) QueryGenericId(_ context.Context, legacyArticleId int) (int, error) {
	s.genericCalls++
	if s.genericErr != nil {
		return 0, s.genericErr
	}
	return s.genericId, nil
}

func (s *stubFunctionalRepo) QuerySameGeneric(_ context.Context, genericId, excludeArticleId, vehicleId, limit int) ([]functionalRow, error) {
	s.sameGenericCalls++
	s.lastGenericArg = genericId
	s.lastExcludeArg = excludeArticleId
	s.lastVehicleArg = vehicleId
	s.lastLimit = limit
	if s.sameErr != nil {
		return nil, s.sameErr
	}
	return s.rows, nil
}

func TestTecDocFunctionalFindFunctionalEquivalents(t *testing.T) {
	repo := &stubFunctionalRepo{
		genericId: 501,
		rows: []functionalRow{
			{LegacyArticleId: 200, ArticleNumber: "W 811/80", Description: "Oil Filter", BrandName: "MANN", GenericId: 501},
			{LegacyArticleId: 201, ArticleNumber: "OP 570", Description: "Oil Filter", BrandName: "Filtron", GenericId: 501},
		},
	}
	svc := &TecDocFunctional{repo: repo}
	refs, err := svc.FindFunctionalEquivalents(100, 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].ArticleNumber != "W 811/80" || refs[0].BrandName != "MANN" {
		t.Fatalf("first ref not mapped: %+v", refs[0])
	}
	if refs[0].Manufacturer != "functional:legacy2generic" {
		t.Fatalf("expected provenance stamp, got %q", refs[0].Manufacturer)
	}
	if repo.lastGenericArg != 501 {
		t.Fatalf("expected generic id 501 passed through, got %d", repo.lastGenericArg)
	}
	if repo.lastExcludeArg != 100 {
		t.Fatalf("expected exclude id 100, got %d", repo.lastExcludeArg)
	}
}

func TestTecDocFunctionalWithVehicleFilter(t *testing.T) {
	repo := &stubFunctionalRepo{
		genericId: 12,
		rows: []functionalRow{
			{LegacyArticleId: 200, ArticleNumber: "P1", BrandName: "MANN", GenericId: 12},
		},
	}
	svc := &TecDocFunctional{repo: repo}
	_, err := svc.FindFunctionalEquivalents(100, 42, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.lastVehicleArg != 42 {
		t.Fatalf("vehicle id not passed through, got %d", repo.lastVehicleArg)
	}
}

func TestTecDocFunctionalNoGenericMapping(t *testing.T) {
	repo := &stubFunctionalRepo{genericId: 0}
	svc := &TecDocFunctional{repo: repo}
	refs, err := svc.FindFunctionalEquivalents(100, 0, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs when no generic mapping, got %d", len(refs))
	}
	if repo.sameGenericCalls != 0 {
		t.Fatalf("must not query QuerySameGeneric when generic id is 0")
	}
}

func TestTecDocFunctionalGenericIdError(t *testing.T) {
	svc := &TecDocFunctional{repo: &stubFunctionalRepo{genericErr: errors.New("db")}}
	if _, err := svc.FindFunctionalEquivalents(100, 0, 20); err == nil {
		t.Fatalf("expected error from QueryGenericId to surface")
	}
}

func TestTecDocFunctionalSameGenericError(t *testing.T) {
	svc := &TecDocFunctional{repo: &stubFunctionalRepo{genericId: 501, sameErr: errors.New("db")}}
	if _, err := svc.FindFunctionalEquivalents(100, 0, 20); err == nil {
		t.Fatalf("expected error from QuerySameGeneric to surface")
	}
}

func TestTecDocFunctionalInvalidId(t *testing.T) {
	svc := &TecDocFunctional{repo: &stubFunctionalRepo{}}
	if _, err := svc.FindFunctionalEquivalents(0, 0, 20); err == nil {
		t.Fatalf("expected error for zero id")
	}
	if _, err := svc.FindFunctionalEquivalents(-1, 0, 20); err == nil {
		t.Fatalf("expected error for negative id")
	}
}

func TestTecDocFunctionalNilRepo(t *testing.T) {
	svc := &TecDocFunctional{}
	if _, err := svc.FindFunctionalEquivalents(1, 0, 20); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocFunctionalNilDBConstructor(t *testing.T) {
	svc := NewTecDocFunctional(nil)
	if svc == nil {
		t.Fatalf("NewTecDocFunctional(nil) must not return nil")
	}
	if _, err := svc.FindFunctionalEquivalents(1, 0, 20); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocFunctionalLimitClamp(t *testing.T) {
	repo := &stubFunctionalRepo{genericId: 1}
	svc := &TecDocFunctional{repo: repo}
	_, _ = svc.FindFunctionalEquivalents(100, 0, 0)
	if repo.lastLimit != 30 {
		t.Fatalf("expected zero limit to clamp to 30, got %d", repo.lastLimit)
	}
	_, _ = svc.FindFunctionalEquivalents(100, 0, 500)
	if repo.lastLimit != 30 {
		t.Fatalf("expected oversized limit to clamp to 30, got %d", repo.lastLimit)
	}
	_, _ = svc.FindFunctionalEquivalents(100, 0, 25)
	if repo.lastLimit != 25 {
		t.Fatalf("expected in-range limit to pass through, got %d", repo.lastLimit)
	}
}

func TestTecDocFunctionalExcludesSourceArticle(t *testing.T) {
	repo := &stubFunctionalRepo{
		genericId: 501,
		rows: []functionalRow{
			// The DB might still return the source article if the exclude
			// filter races; the service must strip it defensively.
			{LegacyArticleId: 100, ArticleNumber: "SELF"},
			{LegacyArticleId: 200, ArticleNumber: "OTHER"},
			{LegacyArticleId: 0, ArticleNumber: "ORPHAN"},   // missing id skipped
			{LegacyArticleId: 200, ArticleNumber: "OTHER2"}, // duplicate id skipped
		},
	}
	svc := &TecDocFunctional{repo: repo}
	refs, _ := svc.FindFunctionalEquivalents(100, 0, 20)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref after dedup+self-exclude, got %d: %+v", len(refs), refs)
	}
	if refs[0].ArticleNumber != "OTHER" {
		t.Fatalf("wrong ref survived: %+v", refs[0])
	}
}

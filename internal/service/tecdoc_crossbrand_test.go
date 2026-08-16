package service

import (
	"context"
	"errors"
	"testing"
)

type stubCrossBrandRepo struct {
	rows          []crossBrandRow
	err           error
	lastClean     string
	lastSource    string
	lastLimit     int
	callCount     int
}

func (s *stubCrossBrandRepo) QuerySiblingHits(_ context.Context, cleanOEM, sourceMake string, limit int) ([]crossBrandRow, error) {
	s.callCount++
	s.lastClean = cleanOEM
	s.lastSource = sourceMake
	s.lastLimit = limit
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func TestTecDocCrossBrandFindCrossBrandEquivalents(t *testing.T) {
	repo := &stubCrossBrandRepo{
		rows: []crossBrandRow{
			{SiblingMake: "Kia", SiblingModel: "SPORTAGE", Platform: "NQ5", SharedParts: 17},
			{SiblingMake: "Kia", SiblingModel: "SORENTO", Platform: "MQ4", SharedParts: 8},
		},
	}
	svc := &TecDocCrossBrand{repo: repo}
	hits, err := svc.FindCrossBrandEquivalents("26300-35503", "Hyundai", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].SiblingMake != "Kia" || hits[0].Platform != "NQ5" || hits[0].SharedParts != 17 {
		t.Fatalf("first hit not mapped: %+v", hits[0])
	}
	if repo.lastClean == "" {
		t.Fatalf("normalized OEM not passed to repo")
	}
	if repo.lastSource != "Hyundai" {
		t.Fatalf("source make not passed, got %q", repo.lastSource)
	}
}

func TestTecDocCrossBrandFiltersSameBrand(t *testing.T) {
	repo := &stubCrossBrandRepo{
		rows: []crossBrandRow{
			{SiblingMake: "Hyundai", SiblingModel: "TUCSON", SharedParts: 100}, // filtered
			{SiblingMake: "Kia", SiblingModel: "SPORTAGE", SharedParts: 17},
			{SiblingMake: "HYUNDAI", SiblingModel: "SANTA FE", SharedParts: 40}, // case-insensitive filter
		},
	}
	svc := &TecDocCrossBrand{repo: repo}
	hits, err := svc.FindCrossBrandEquivalents("26300-35503", "Hyundai", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit after same-brand filter, got %d: %+v", len(hits), hits)
	}
	if hits[0].SiblingMake != "Kia" {
		t.Fatalf("expected Kia to survive, got %s", hits[0].SiblingMake)
	}
}

func TestTecDocCrossBrandDedup(t *testing.T) {
	repo := &stubCrossBrandRepo{
		rows: []crossBrandRow{
			{SiblingMake: "Kia", SiblingModel: "SPORTAGE", Platform: "NQ5", SharedParts: 17},
			{SiblingMake: "kia", SiblingModel: "sportage", Platform: "nq5", SharedParts: 3}, // dup (case)
			{SiblingMake: "", SiblingModel: "SPORTAGE", SharedParts: 5},                     // dropped: empty make
			{SiblingMake: "Kia", SiblingModel: "", SharedParts: 5},                          // dropped: empty model
		},
	}
	svc := &TecDocCrossBrand{repo: repo}
	hits, err := svc.FindCrossBrandEquivalents("X", "Hyundai", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit after dedup, got %d: %+v", len(hits), hits)
	}
}

func TestTecDocCrossBrandEmptyOEM(t *testing.T) {
	svc := &TecDocCrossBrand{repo: &stubCrossBrandRepo{}}
	if _, err := svc.FindCrossBrandEquivalents("", "Hyundai", 20); err == nil {
		t.Fatalf("expected error for empty OEM")
	}
	if _, err := svc.FindCrossBrandEquivalents("   ", "Hyundai", 20); err == nil {
		t.Fatalf("expected error for whitespace OEM")
	}
	if _, err := svc.FindCrossBrandEquivalents("---", "Hyundai", 20); err == nil {
		t.Fatalf("expected error for OEM that normalizes to empty")
	}
}

func TestTecDocCrossBrandRepoError(t *testing.T) {
	svc := &TecDocCrossBrand{repo: &stubCrossBrandRepo{err: errors.New("db")}}
	if _, err := svc.FindCrossBrandEquivalents("X-1", "Hyundai", 20); err == nil {
		t.Fatalf("expected repo error to surface")
	}
}

func TestTecDocCrossBrandNilRepo(t *testing.T) {
	svc := &TecDocCrossBrand{}
	if _, err := svc.FindCrossBrandEquivalents("X-1", "Hyundai", 20); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocCrossBrandNilDBConstructor(t *testing.T) {
	svc := NewTecDocCrossBrand(nil)
	if svc == nil {
		t.Fatalf("NewTecDocCrossBrand(nil) must not return nil")
	}
	if _, err := svc.FindCrossBrandEquivalents("X-1", "Hyundai", 20); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocCrossBrandLimitClamp(t *testing.T) {
	repo := &stubCrossBrandRepo{}
	svc := &TecDocCrossBrand{repo: repo}
	_, _ = svc.FindCrossBrandEquivalents("X-1", "Hyundai", 0)
	if repo.lastLimit != 20 {
		t.Fatalf("expected zero limit to clamp to 20, got %d", repo.lastLimit)
	}
	_, _ = svc.FindCrossBrandEquivalents("X-1", "Hyundai", 999)
	if repo.lastLimit != 20 {
		t.Fatalf("expected oversized limit to clamp to 20, got %d", repo.lastLimit)
	}
	_, _ = svc.FindCrossBrandEquivalents("X-1", "Hyundai", 8)
	if repo.lastLimit != 8 {
		t.Fatalf("expected in-range limit to pass through, got %d", repo.lastLimit)
	}
}

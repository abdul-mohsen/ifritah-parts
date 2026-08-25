package service

import (
	"context"
	"errors"
	"testing"
)

type stubSpecificationRepo struct {
	rows      []specificationRow
	err       error
	lastId    int
	callCount int
}

func (s *stubSpecificationRepo) QuerySpecifications(_ context.Context, legacyArticleId int) ([]specificationRow, error) {
	s.callCount++
	s.lastId = legacyArticleId
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func (s *stubSpecificationRepo) QuerySpecificationsBatch(_ context.Context, ids []int) (map[int][]specificationRow, error) {
	s.callCount++
	if s.err != nil {
		return nil, s.err
	}
	out := make(map[int][]specificationRow, len(ids))
	for _, id := range ids {
		s.lastId = id
		// Stamp each row with the id so the caller can distribute.
		rowsForId := make([]specificationRow, len(s.rows))
		for i, r := range s.rows {
			r.LegacyArticleId = id
			rowsForId[i] = r
		}
		out[id] = rowsForId
	}
	return out, nil
}

func TestTecDocSpecificationsFindSpecifications(t *testing.T) {
	repo := &stubSpecificationRepo{
		rows: []specificationRow{
			{CriteriaDescription: "Length", RawValue: "120", UnitDescription: "mm", CriteriaType: "N"},
			{CriteriaDescription: "Thread", RawValue: "M20x1.5", UnitDescription: "", CriteriaType: "K"},
			{CriteriaDescription: "  ", RawValue: "ignored", UnitDescription: "", CriteriaType: ""}, // empty name skipped
			{CriteriaDescription: "Weight", RawValue: "", UnitDescription: "g", CriteriaType: "N"},  // empty value skipped
		},
	}
	svc := &TecDocSpecifications{repo: repo}
	specs, err := svc.FindSpecifications(100001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs after filtering empties, got %d: %+v", len(specs), specs)
	}
	if specs[0].Name != "Length" || specs[0].Value != "120" || specs[0].Unit != "mm" {
		t.Fatalf("first spec not mapped correctly: %+v", specs[0])
	}
	if specs[0].Source != "tecdoc:articlecriteria" {
		t.Fatalf("source provenance not stamped: %q", specs[0].Source)
	}
	if repo.lastId != 100001 {
		t.Fatalf("legacyArticleId not passed through, got %d", repo.lastId)
	}
}

func TestTecDocSpecificationsInvalidId(t *testing.T) {
	svc := &TecDocSpecifications{repo: &stubSpecificationRepo{}}
	if _, err := svc.FindSpecifications(0); err == nil {
		t.Fatalf("expected error for zero id")
	}
	if _, err := svc.FindSpecifications(-5); err == nil {
		t.Fatalf("expected error for negative id")
	}
}

func TestTecDocSpecificationsRepoError(t *testing.T) {
	svc := &TecDocSpecifications{repo: &stubSpecificationRepo{err: errors.New("boom")}}
	if _, err := svc.FindSpecifications(1); err == nil {
		t.Fatalf("expected repo error to surface")
	}
}

func TestTecDocSpecificationsNilRepo(t *testing.T) {
	svc := &TecDocSpecifications{}
	if _, err := svc.FindSpecifications(1); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocSpecificationsNilDBConstructor(t *testing.T) {
	svc := NewTecDocSpecifications(nil)
	if svc == nil {
		t.Fatalf("NewTecDocSpecifications(nil) must not return nil")
	}
	if _, err := svc.FindSpecifications(1); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocSpecificationsTrimsWhitespace(t *testing.T) {
	repo := &stubSpecificationRepo{
		rows: []specificationRow{
			{CriteriaDescription: "  Torque  ", RawValue: "  25  ", UnitDescription: " Nm ", CriteriaType: " N "},
		},
	}
	svc := &TecDocSpecifications{repo: repo}
	specs, _ := svc.FindSpecifications(1)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Name != "Torque" || specs[0].Value != "25" || specs[0].Unit != "Nm" || specs[0].CriteriaType != "N" {
		t.Fatalf("whitespace not trimmed: %+v", specs[0])
	}
}

// TestTecDocSpecifications_FindSpecificationsBatch - M3.S1.T2 batch API
// returns per-id spec lists in one call, dedupes zero + duplicate ids.
func TestTecDocSpecifications_FindSpecificationsBatch(t *testing.T) {
	repo := &stubSpecificationRepo{
		rows: []specificationRow{
			{CriteriaDescription: "Length", RawValue: "100", UnitDescription: "mm", CriteriaType: "N"},
			{CriteriaDescription: "Thread", RawValue: "M12", UnitDescription: "", CriteriaType: "K"},
		},
	}
	svc := &TecDocSpecifications{repo: repo}

	byId, err := svc.FindSpecificationsBatch([]int{100, 200, 200, 0, -1, 300})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 3 unique valid ids -> 3 map entries
	if len(byId) != 3 {
		t.Errorf("len(byId) = %d, want 3 (dedup 200 + drop 0/-1)", len(byId))
	}
	for id, specs := range byId {
		if len(specs) != 2 {
			t.Errorf("byId[%d] len = %d, want 2", id, len(specs))
		}
		for _, s := range specs {
			if s.Source != "tecdoc:articlecriteria" {
				t.Errorf("Source = %q, want tecdoc:articlecriteria", s.Source)
			}
		}
	}
}

// TestTecDocSpecifications_FindSpecificationsBatch_EmptyInput
func TestTecDocSpecifications_FindSpecificationsBatch_EmptyInput(t *testing.T) {
	repo := &stubSpecificationRepo{}
	svc := &TecDocSpecifications{repo: repo}

	byId, err := svc.FindSpecificationsBatch(nil)
	if err != nil {
		t.Fatalf("unexpected err on nil: %v", err)
	}
	if len(byId) != 0 {
		t.Errorf("len(byId) = %d, want 0", len(byId))
	}

	byId, err = svc.FindSpecificationsBatch([]int{0, -1})
	if err != nil {
		t.Fatalf("unexpected err on invalid ids: %v", err)
	}
	if len(byId) != 0 {
		t.Errorf("len(byId) = %d, want 0 (all invalid ids)", len(byId))
	}
}

// TestTecDocSpecifications_FindSpecificationsBatch_RepoError - repo err propagates
func TestTecDocSpecifications_FindSpecificationsBatch_RepoError(t *testing.T) {
	repo := &stubSpecificationRepo{err: errors.New("boom")}
	svc := &TecDocSpecifications{repo: repo}

	_, err := svc.FindSpecificationsBatch([]int{100})
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

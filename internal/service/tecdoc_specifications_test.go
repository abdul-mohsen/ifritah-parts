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

package service

import (
	"context"
	"errors"
	"testing"
)

type stubDocumentRepo struct {
	rows      []documentRow
	err       error
	lastId    int
	callCount int
}

func (s *stubDocumentRepo) QueryDocuments(_ context.Context, legacyArticleId int) ([]documentRow, error) {
	s.callCount++
	s.lastId = legacyArticleId
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}

func TestTecDocDocumentsFindDocuments(t *testing.T) {
	repo := &stubDocumentRepo{
		rows: []documentRow{
			{URL: "https://tecdoc.cdn/doc1.pdf", FileName: "mount.pdf", DocType: "MOUNTING_INSTRUCTIONS", Language: "en"},
			{URL: "https://tecdoc.cdn/doc2.pdf", FileName: "safety.pdf", DocType: "SAFETY_DATASHEET", Language: "de"},
			{URL: "", FileName: "orphan.pdf", DocType: "GENERIC", Language: "en"}, // empty URL skipped
		},
	}
	svc := &TecDocDocuments{repo: repo}
	docs, err := svc.FindDocuments(100001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs after filtering empty URLs, got %d", len(docs))
	}
	if docs[0].URL != "https://tecdoc.cdn/doc1.pdf" || docs[0].DocType != "MOUNTING_INSTRUCTIONS" {
		t.Fatalf("first doc not mapped: %+v", docs[0])
	}
	if docs[0].LicensedSource != "tecdoc:articledocs" {
		t.Fatalf("expected licensedSource stamp, got %q", docs[0].LicensedSource)
	}
	if docs[0].LegacyArticleId != 100001 {
		t.Fatalf("expected LegacyArticleId set from query arg, got %d", docs[0].LegacyArticleId)
	}
}

func TestTecDocDocumentsInvalidId(t *testing.T) {
	svc := &TecDocDocuments{repo: &stubDocumentRepo{}}
	if _, err := svc.FindDocuments(0); err == nil {
		t.Fatalf("expected error for zero id")
	}
	if _, err := svc.FindDocuments(-1); err == nil {
		t.Fatalf("expected error for negative id")
	}
}

func TestTecDocDocumentsRepoError(t *testing.T) {
	svc := &TecDocDocuments{repo: &stubDocumentRepo{err: errors.New("db down")}}
	if _, err := svc.FindDocuments(1); err == nil {
		t.Fatalf("expected repo error to surface")
	}
}

func TestTecDocDocumentsNilRepo(t *testing.T) {
	svc := &TecDocDocuments{}
	if _, err := svc.FindDocuments(1); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocDocumentsNilDBConstructor(t *testing.T) {
	svc := NewTecDocDocuments(nil)
	if svc == nil {
		t.Fatalf("NewTecDocDocuments(nil) must not return nil")
	}
	if _, err := svc.FindDocuments(1); err == nil {
		t.Fatalf("expected 'database not connected' error")
	}
}

func TestTecDocDocumentsEmptyResult(t *testing.T) {
	svc := &TecDocDocuments{repo: &stubDocumentRepo{rows: nil}}
	docs, err := svc.FindDocuments(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 0 {
		t.Fatalf("expected 0 docs for empty result, got %d", len(docs))
	}
}

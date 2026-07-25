package service

import (
	"os"
	"path/filepath"
	"testing"

	"parts-engine/internal/model"
)

func TestWorkerStoreSubmissionsStayPendingUntilReviewed(t *testing.T) {
	dir := t.TempDir()
	store, err := NewWorkerStore(dir)
	if err != nil {
		t.Fatalf("NewWorkerStore: %v", err)
	}
	defer store.Close()

	submission, err := store.SubmitReplacement(model.WorkerReplacementSubmissionInput{
		PartArticleNumber:      "97133-D3000",
		CandidateArticleNumber: "CUK 26 013",
		CandidateBrandName:     "MANN-FILTER",
		RelationType:           "equivalent",
		EvidenceText:           "Shared OEM reference on owned cross-reference index.",
		EvidenceSource:         "owned_catalog_crossref",
		Submitter:              "qa-agent",
	})
	if err != nil {
		t.Fatalf("SubmitReplacement: %v", err)
	}
	if submission.Status != "pending" {
		t.Fatalf("expected pending status, got %s", submission.Status)
	}

	pending, err := store.ListReplacementSubmissions("pending", 10)
	if err != nil {
		t.Fatalf("ListReplacementSubmissions: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending submission, got %d", len(pending))
	}

	reviewed, err := store.ReviewReplacement(submission.ID, "approved", "strict-qa", "evidence accepted")
	if err != nil {
		t.Fatalf("ReviewReplacement: %v", err)
	}
	if reviewed.Status != "approved" {
		t.Fatalf("expected approved status, got %s", reviewed.Status)
	}
}

func TestWorkerStoreUsesSeparateDatabaseFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewWorkerStore(dir)
	if err != nil {
		t.Fatalf("NewWorkerStore: %v", err)
	}
	defer store.Close()

	dbPath := filepath.Join(dir, "worker_contributions.db")
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected separate worker db file at %s: %v", dbPath, err)
	}
}

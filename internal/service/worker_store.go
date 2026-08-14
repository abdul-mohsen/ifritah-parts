package service

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"parts-engine/internal/model"
)

type WorkerStore struct {
	db *sql.DB
}

func NewWorkerStore(dataDir string) (*WorkerStore, error) {
	path := filepath.Join(dataDir, "worker_contributions.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open worker db: %w", err)
	}
	store := &WorkerStore{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *WorkerStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *WorkerStore) init() error {
	if s == nil || s.db == nil {
		return fmt.Errorf("worker db not configured")
	}
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS worker_replacement_submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			part_article_number TEXT NOT NULL,
			candidate_article_number TEXT NOT NULL,
			candidate_brand_name TEXT,
			relation_type TEXT NOT NULL CHECK (relation_type IN ('replacement', 'equivalent')),
			evidence_text TEXT NOT NULL,
			evidence_source TEXT NOT NULL,
			submitter TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
			review_notes TEXT,
			reviewed_by TEXT,
			reviewed_at TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS worker_replacement_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			submission_id INTEGER NOT NULL,
			event_type TEXT NOT NULL,
			actor TEXT NOT NULL,
			note TEXT,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (submission_id) REFERENCES worker_replacement_submissions(id)
		);`,
	}
	for _, stmt := range ddl {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("init worker db: %w", err)
		}
	}
	return nil
}

func (s *WorkerStore) SubmitReplacement(input model.WorkerReplacementSubmissionInput) (*model.WorkerReplacementSubmission, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("worker db not configured")
	}
	input.PartArticleNumber = strings.TrimSpace(input.PartArticleNumber)
	input.CandidateArticleNumber = strings.TrimSpace(input.CandidateArticleNumber)
	input.RelationType = strings.TrimSpace(strings.ToLower(input.RelationType))
	input.EvidenceText = strings.TrimSpace(input.EvidenceText)
	input.EvidenceSource = strings.TrimSpace(input.EvidenceSource)
	input.Submitter = strings.TrimSpace(input.Submitter)
	if input.Submitter == "" {
		input.Submitter = "unknown"
	}
	if input.PartArticleNumber == "" || input.CandidateArticleNumber == "" || input.EvidenceText == "" || input.EvidenceSource == "" {
		return nil, fmt.Errorf("part, candidate, evidence text, and evidence source are required")
	}
	if input.RelationType != "replacement" && input.RelationType != "equivalent" {
		return nil, fmt.Errorf("relationType must be replacement or equivalent")
	}

	result, err := s.db.Exec(
		`INSERT INTO worker_replacement_submissions
		(part_article_number, candidate_article_number, candidate_brand_name, relation_type, evidence_text, evidence_source, submitter, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')`,
		input.PartArticleNumber,
		input.CandidateArticleNumber,
		input.CandidateBrandName,
		input.RelationType,
		input.EvidenceText,
		input.EvidenceSource,
		input.Submitter,
	)
	if err != nil {
		return nil, fmt.Errorf("insert worker submission: %w", err)
	}
	id64, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("read worker submission id: %w", err)
	}
	id := int(id64)
	if err := s.recordEvent(id, "submitted", input.Submitter, input.EvidenceSource); err != nil {
		return nil, err
	}
	return s.GetSubmission(id)
}

func (s *WorkerStore) ListReplacementSubmissions(status string, limit int) ([]model.WorkerReplacementSubmission, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("worker db not configured")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	status = strings.TrimSpace(strings.ToLower(status))
	query := `SELECT id, part_article_number, candidate_article_number, candidate_brand_name, relation_type, evidence_text, evidence_source, submitter, status, review_notes, reviewed_by, reviewed_at, created_at
		FROM worker_replacement_submissions`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list worker submissions: %w", err)
	}
	defer rows.Close()

	var submissions []model.WorkerReplacementSubmission
	for rows.Next() {
		submission, err := scanWorkerSubmission(rows)
		if err != nil {
			return nil, err
		}
		submissions = append(submissions, submission)
	}
	return submissions, rows.Err()
}

func (s *WorkerStore) ReviewReplacement(id int, action, reviewer, notes string) (*model.WorkerReplacementSubmission, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("worker db not configured")
	}
	action = strings.TrimSpace(strings.ToLower(action))
	reviewer = strings.TrimSpace(reviewer)
	notes = strings.TrimSpace(notes)
	if action != "approved" && action != "rejected" {
		return nil, fmt.Errorf("action must be approved or rejected")
	}
	if reviewer == "" {
		return nil, fmt.Errorf("reviewer is required")
	}

	result, err := s.db.Exec(
		`UPDATE worker_replacement_submissions
		 SET status = ?, review_notes = ?, reviewed_by = ?, reviewed_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status = 'pending'`,
		action, notes, reviewer, id,
	)
	if err != nil {
		return nil, fmt.Errorf("review worker submission: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("review worker submission rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("submission not found or already reviewed")
	}
	if err := s.recordEvent(id, action, reviewer, notes); err != nil {
		return nil, err
	}
	return s.GetSubmission(id)
}

func (s *WorkerStore) GetSubmission(id int) (*model.WorkerReplacementSubmission, error) {
	row := s.db.QueryRow(
		`SELECT id, part_article_number, candidate_article_number, candidate_brand_name, relation_type, evidence_text, evidence_source, submitter, status, review_notes, reviewed_by, reviewed_at, created_at
		 FROM worker_replacement_submissions WHERE id = ?`, id,
	)
	submission, err := scanWorkerSubmission(row)
	if err != nil {
		return nil, err
	}
	return &submission, nil
}

func (s *WorkerStore) recordEvent(submissionID int, eventType, actor, note string) error {
	if _, err := s.db.Exec(
		`INSERT INTO worker_replacement_events (submission_id, event_type, actor, note) VALUES (?, ?, ?, ?)`,
		submissionID, eventType, actor, note,
	); err != nil {
		return fmt.Errorf("record worker event: %w", err)
	}
	return nil
}

type workerSubmissionScanner interface {
	Scan(dest ...any) error
}

func scanWorkerSubmission(scanner workerSubmissionScanner) (model.WorkerReplacementSubmission, error) {
	var submission model.WorkerReplacementSubmission
	var candidateBrandName, reviewNotes, reviewedBy, reviewedAt sql.NullString
	if err := scanner.Scan(
		&submission.ID,
		&submission.PartArticleNumber,
		&submission.CandidateArticleNumber,
		&candidateBrandName,
		&submission.RelationType,
		&submission.EvidenceText,
		&submission.EvidenceSource,
		&submission.Submitter,
		&submission.Status,
		&reviewNotes,
		&reviewedBy,
		&reviewedAt,
		&submission.CreatedAt,
	); err != nil {
		return submission, fmt.Errorf("scan worker submission: %w", err)
	}
	submission.CandidateBrandName = candidateBrandName.String
	submission.ReviewNotes = reviewNotes.String
	submission.ReviewedBy = reviewedBy.String
	submission.ReviewedAt = reviewedAt.String
	return submission, nil
}

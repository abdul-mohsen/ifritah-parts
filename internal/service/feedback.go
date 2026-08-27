// Package service — feedback storage layer (M6.S2.T1).
//
// Persists thumbs-up / thumbs-down / skip verdicts from the frontend
// search-result feedback widget. Aggregates weekly for the report
// pipeline (docs/reports/user-feedback-{week}.md) and surfaces the
// top-disputed OEM list for the M7 ranker's negative-signal set.
//
// Privacy invariant enforced by this file: NO raw IP address and NO
// raw session cookie EVER leave this process's memory. Callers pass
// pre-hashed SHA256 fingerprints; INSERT statements only reference
// those hash columns. See internal/handler/feedback.go for the hash
// derivation and the test that asserts hash-shape on stored rows.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// FeedbackVerdict is the union type accepted by search_feedback.verdict.
// The CHECK constraint on the table matches this list one-for-one.
type FeedbackVerdict string

const (
	FeedbackThumbsUp   FeedbackVerdict = "thumbs_up"
	FeedbackThumbsDown FeedbackVerdict = "thumbs_down"
	FeedbackSkip       FeedbackVerdict = "skip"
)

// Valid returns true iff v is one of the three canonical verdict strings.
func (v FeedbackVerdict) Valid() bool {
	switch v {
	case FeedbackThumbsUp, FeedbackThumbsDown, FeedbackSkip:
		return true
	}
	return false
}

// FeedbackEvent is a single vote written to search_feedback. The Hash
// fields are the ALREADY-HASHED SHA256 fingerprints of the raw session
// cookie / client IP — this struct never carries plaintext PII.
type FeedbackEvent struct {
	SearchID        string          `json:"searchId"`
	QueryOEM        string          `json:"queryOem"`
	ResultArticleID int32           `json:"resultArticleId"`
	ResultBrand     string          `json:"resultBrand"`
	ResultPartNum   string          `json:"resultPartNum"`
	Verdict         FeedbackVerdict `json:"verdict"`
	Reason          string          `json:"reason,omitempty"`
	UserHash        string          `json:"userHash,omitempty"`
	ClientIPHash    string          `json:"clientIpHash,omitempty"`
}

// WeeklyBucket is one row of the /api/feedback/weekly aggregation.
type WeeklyBucket struct {
	WeekStart time.Time       `json:"weekStart"`
	Verdict   FeedbackVerdict `json:"verdict"`
	Votes     int64           `json:"votes"`
}

// DisputedOEM is one row of the /api/feedback/disputed report.
type DisputedOEM struct {
	QueryOEM   string `json:"queryOem"`
	DownVotes  int64  `json:"downVotes"`
	UpVotes    int64  `json:"upVotes"`
	TotalVotes int64  `json:"totalVotes"`
}

// FeedbackStore is the storage interface the handler talks to. Splitting
// the interface out lets the handler tests inject an in-memory fake and
// exercise validation / hashing / rate-limiting without needing a live
// Postgres instance in CI.
type FeedbackStore interface {
	Insert(ctx context.Context, ev FeedbackEvent) (id int64, createdAt time.Time, err error)
	AggregateWeekly(ctx context.Context) ([]WeeklyBucket, error)
	TopDisputedOEMs(ctx context.Context) ([]DisputedOEM, error)
}

// ErrFeedbackDBNotConnected is returned when the underlying *sql.DB is
// nil. The handler translates this to a 503 so callers can distinguish
// "server misconfigured" from "bad request".
var ErrFeedbackDBNotConnected = errors.New("feedback: database not connected")

// FeedbackService is the Postgres-backed FeedbackStore. Uses raw
// db.QueryContext / db.QueryRowContext (rather than sqlc-generated
// code) so that the queries can evolve without regenerating the store
// package — see db/queries/search_feedback.sql for the source-of-truth
// SQL that this file mirrors.
type FeedbackService struct {
	db *sql.DB
}

// NewFeedbackService returns a FeedbackService. Nil db is legal — every
// method returns ErrFeedbackDBNotConnected. This matches the pattern
// used by NewOEMLookup / NewPartsLookup and keeps the handler wiring
// in main.go free of nil-checks.
func NewFeedbackService(db *sql.DB) *FeedbackService {
	return &FeedbackService{db: db}
}

// truncate defensively caps free-text fields so a malicious payload
// can't bloat the row. Chosen limits match Postgres text-column
// operational advice: 500 for reason, 200 for brand / part number,
// 128 for hash outputs (SHA256 hex = 64 chars but we keep headroom
// for algorithm changes). Package-scoped because community_contrib.go
// re-uses the same helper — one clip function, one contract.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// Insert writes a single feedback event. Validates the verdict against
// the CHECK constraint enum before hitting the DB so the error message
// is clear (Postgres would return a fairly opaque constraint-violation
// error otherwise).
func (f *FeedbackService) Insert(ctx context.Context, ev FeedbackEvent) (int64, time.Time, error) {
	if f == nil || f.db == nil {
		return 0, time.Time{}, ErrFeedbackDBNotConnected
	}
	if !ev.Verdict.Valid() {
		return 0, time.Time{}, fmt.Errorf("feedback: invalid verdict %q — want thumbs_up | thumbs_down | skip", ev.Verdict)
	}
	if strings.TrimSpace(ev.QueryOEM) == "" {
		return 0, time.Time{}, fmt.Errorf("feedback: queryOem required")
	}
	if strings.TrimSpace(ev.SearchID) == "" {
		return 0, time.Time{}, fmt.Errorf("feedback: searchId required")
	}

	// Trim + truncate every free-text column so a malicious payload
	// can't bloat the table.
	ev.SearchID = truncate(strings.TrimSpace(ev.SearchID), 100)
	ev.QueryOEM = truncate(strings.TrimSpace(ev.QueryOEM), 100)
	ev.ResultBrand = truncate(strings.TrimSpace(ev.ResultBrand), 200)
	ev.ResultPartNum = truncate(strings.TrimSpace(ev.ResultPartNum), 200)
	ev.Reason = truncate(ev.Reason, 500)
	ev.UserHash = truncate(ev.UserHash, 128)
	ev.ClientIPHash = truncate(ev.ClientIPHash, 128)

	const q = `
		INSERT INTO search_feedback (
			search_id,
			query_oem,
			result_article_id,
			result_brand,
			result_part_num,
			verdict,
			reason,
			user_hash,
			client_ip_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`

	var id int64
	var createdAt time.Time
	err := f.db.QueryRowContext(ctx, q,
		ev.SearchID,
		ev.QueryOEM,
		ev.ResultArticleID,
		nullableString(ev.ResultBrand),
		nullableString(ev.ResultPartNum),
		string(ev.Verdict),
		nullableString(ev.Reason),
		nullableString(ev.UserHash),
		nullableString(ev.ClientIPHash),
	).Scan(&id, &createdAt)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("feedback: insert: %w", err)
	}
	return id, createdAt, nil
}

// AggregateWeekly returns per-(week, verdict) vote counts for the last
// 90 days. Empty slice on empty table — never nil — so the handler can
// json-encode `[]` without a special case.
func (f *FeedbackService) AggregateWeekly(ctx context.Context) ([]WeeklyBucket, error) {
	if f == nil || f.db == nil {
		return nil, ErrFeedbackDBNotConnected
	}
	const q = `
		SELECT
			date_trunc('week', created_at)::date AS week_start,
			verdict,
			COUNT(*) AS votes
		FROM search_feedback
		WHERE created_at >= NOW() - INTERVAL '90 days'
		GROUP BY 1, 2
		ORDER BY 1 DESC, 2`

	rows, err := f.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("feedback: aggregate weekly: %w", err)
	}
	defer rows.Close()

	out := make([]WeeklyBucket, 0)
	for rows.Next() {
		var b WeeklyBucket
		var v string
		if err := rows.Scan(&b.WeekStart, &v, &b.Votes); err != nil {
			return nil, fmt.Errorf("feedback: scan weekly row: %w", err)
		}
		b.Verdict = FeedbackVerdict(v)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("feedback: iterate weekly rows: %w", err)
	}
	return out, nil
}

// TopDisputedOEMs returns OEMs with ≥ 3 thumbs-down votes in the last
// 30 days, ordered by downvote count descending, capped at 50.
func (f *FeedbackService) TopDisputedOEMs(ctx context.Context) ([]DisputedOEM, error) {
	if f == nil || f.db == nil {
		return nil, ErrFeedbackDBNotConnected
	}
	const q = `
		SELECT
			query_oem,
			COUNT(*) FILTER (WHERE verdict = 'thumbs_down') AS down_votes,
			COUNT(*) FILTER (WHERE verdict = 'thumbs_up')   AS up_votes,
			COUNT(*)                                         AS total_votes
		FROM search_feedback
		WHERE created_at >= NOW() - INTERVAL '30 days'
		GROUP BY query_oem
		HAVING COUNT(*) FILTER (WHERE verdict = 'thumbs_down') >= 3
		ORDER BY down_votes DESC
		LIMIT 50`

	rows, err := f.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("feedback: top disputed: %w", err)
	}
	defer rows.Close()

	out := make([]DisputedOEM, 0)
	for rows.Next() {
		var d DisputedOEM
		if err := rows.Scan(&d.QueryOEM, &d.DownVotes, &d.UpVotes, &d.TotalVotes); err != nil {
			return nil, fmt.Errorf("feedback: scan disputed row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("feedback: iterate disputed rows: %w", err)
	}
	return out, nil
}

// nullableString returns sql.NullString{Valid:false} for an empty input.
// Keeps the table free of empty-string values in nullable columns —
// makes IS NULL / IS NOT NULL filters correct for downstream queries.
func nullableString(s string) interface{} {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return s
}

// Compile-time assertion that *FeedbackService satisfies FeedbackStore.
// Catches any accidental interface drift the moment it happens.
var _ FeedbackStore = (*FeedbackService)(nil)

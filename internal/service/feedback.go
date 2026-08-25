package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// FeedbackVerdict is either "up" or "down". Any other value is an error.
type FeedbackVerdict string

const (
	FeedbackUp   FeedbackVerdict = "up"
	FeedbackDown FeedbackVerdict = "down"
)

// SearchFeedback is one thumbs-up/down record submitted by a user against
// a search result. Powers M6.S2.T1 and the weekly-report pipeline.
type SearchFeedback struct {
	ID          int64           `json:"id"`
	QueryOEM    string          `json:"queryOem"`
	ResultOEM   string          `json:"resultOem"`
	ResultDesc  string          `json:"resultDesc,omitempty"`
	ResultBrand string          `json:"resultBrand,omitempty"`
	Verdict     FeedbackVerdict `json:"verdict"`
	Reason      string          `json:"reason,omitempty"`
	SessionID   string          `json:"sessionId,omitempty"`
	SubmittedAt time.Time       `json:"submittedAt"`
}

// FeedbackService writes/reads search_feedback rows.
type FeedbackService struct {
	db *sql.DB
}

func NewFeedbackService(db *sql.DB) *FeedbackService {
	if db == nil {
		return &FeedbackService{}
	}
	return &FeedbackService{db: db}
}

// Submit records a single feedback event. Validates verdict; returns
// error on bad input.
func (f *FeedbackService) Submit(ctx context.Context, sf SearchFeedback) (int64, error) {
	if f.db == nil {
		return 0, fmt.Errorf("feedback service not connected")
	}
	if sf.QueryOEM == "" {
		return 0, fmt.Errorf("queryOem required")
	}
	if sf.ResultOEM == "" {
		return 0, fmt.Errorf("resultOem required")
	}
	if sf.Verdict != FeedbackUp && sf.Verdict != FeedbackDown {
		return 0, fmt.Errorf("verdict must be 'up' or 'down', got %q", sf.Verdict)
	}
	// Truncate long free-text fields so a malicious payload can't bloat the row.
	sf.ResultDesc = truncate(sf.ResultDesc, 500)
	sf.ResultBrand = truncate(sf.ResultBrand, 100)
	sf.Reason = truncate(sf.Reason, 500)
	sf.SessionID = truncate(sf.SessionID, 100)

	const q = `
		INSERT INTO search_feedback
			(query_oem, result_oem, result_desc, result_brand, verdict, reason, session_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`

	var id int64
	err := f.db.QueryRowContext(ctx, q,
		strings.TrimSpace(sf.QueryOEM),
		strings.TrimSpace(sf.ResultOEM),
		sf.ResultDesc,
		sf.ResultBrand,
		string(sf.Verdict),
		sf.Reason,
		sf.SessionID,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("submit feedback: %w", err)
	}
	return id, nil
}

// AggregateWeekly returns thumbs-up rate per category for the last N days.
// Used by the M6.S2.T1 weekly-report generator.
type AggregatedCategory struct {
	Category  string
	Total     int
	UpCount   int
	DownCount int
	UpRate    float64
}

// AggregateSince returns per-QueryOEM aggregate for feedback newer than
// `since`. The caller (a report script) groups OEMs by category via
// DecodeOEMPrefix.
func (f *FeedbackService) AggregateSince(ctx context.Context, since time.Time) (map[string]struct{ Up, Down int }, error) {
	if f.db == nil {
		return nil, nil
	}
	const q = `
		SELECT query_oem, verdict, COUNT(*)
		FROM search_feedback
		WHERE submitted_at >= $1
		GROUP BY query_oem, verdict`

	rows, err := f.db.QueryContext(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("aggregate feedback: %w", err)
	}
	defer rows.Close()

	out := make(map[string]struct{ Up, Down int })
	for rows.Next() {
		var oem, verdict string
		var count int
		if err := rows.Scan(&oem, &verdict, &count); err != nil {
			continue
		}
		v := out[oem]
		if verdict == "up" {
			v.Up = count
		} else if verdict == "down" {
			v.Down = count
		}
		out[oem] = v
	}
	return out, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

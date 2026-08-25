package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CommunityContribution is one user-submitted OEM<->aftermarket cross-ref
// record. Powers M4.S4 (community contribution flow).
type CommunityContribution struct {
	ID            int64      `json:"id"`
	OEMNormalized string     `json:"oemNormalized"`
	Brand         string     `json:"brand"`
	PartNumber    string     `json:"partNumber"`
	Description   string     `json:"description,omitempty"`
	SourceURL     string     `json:"sourceUrl,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	Contributor   string     `json:"contributor,omitempty"`
	ContributorIP string     `json:"contributorIp,omitempty"`
	Status        string     `json:"status"`
	SubmittedAt   time.Time  `json:"submittedAt"`
	ReviewedAt    *time.Time `json:"reviewedAt,omitempty"`
	ReviewedBy    string     `json:"reviewedBy,omitempty"`
	ReviewNote    string     `json:"reviewNote,omitempty"`
}

// ContribStatus is the enum stored in aftermarket_community.status.
type ContribStatus string

const (
	ContribPending  ContribStatus = "pending"
	ContribApproved ContribStatus = "approved"
	ContribRejected ContribStatus = "rejected"
)

// CommunityContribService handles submissions + moderation for
// aftermarket_community. Reads/writes only — no rate-limiting here;
// that's the handler's job (per-IP daily quota).
type CommunityContribService struct {
	db *sql.DB
}

func NewCommunityContribService(db *sql.DB) *CommunityContribService {
	if db == nil {
		return &CommunityContribService{}
	}
	return &CommunityContribService{db: db}
}

// Submit records a new pending contribution. Normalises OEM + brand,
// truncates long free-text, validates required fields. Returns id or
// error.
func (s *CommunityContribService) Submit(ctx context.Context, c CommunityContribution) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("community-contrib service not connected")
	}
	oem := NormalizeOEM(c.OEMNormalized)
	brand := NormalizeBrand(c.Brand)
	partNumber := strings.TrimSpace(c.PartNumber)
	if oem == "" {
		return 0, fmt.Errorf("oem is required")
	}
	if brand == "" {
		return 0, fmt.Errorf("brand is required")
	}
	if partNumber == "" {
		return 0, fmt.Errorf("partNumber is required")
	}

	desc := truncate(c.Description, 500)
	url := truncate(c.SourceURL, 500)
	notes := truncate(c.Notes, 1000)
	contributor := truncate(c.Contributor, 200)

	// contributor_ip is inet — pass NULL when empty so bad input doesn't
	// corrupt the column.
	var ipArg any
	if c.ContributorIP != "" {
		ipArg = c.ContributorIP
	} else {
		ipArg = nil
	}

	const q = `
		INSERT INTO aftermarket_community
			(oem_normalized, brand, part_number, description, source_url, notes, contributor, contributor_ip, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')
		RETURNING id`

	var id int64
	err := s.db.QueryRowContext(ctx, q,
		oem, brand, partNumber, desc, url, notes, contributor, ipArg,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("submit contribution: %w", err)
	}
	return id, nil
}

// CountRecentByIP returns how many contributions the given IP has
// submitted in the last 24 hours. Used by the rate-limit check in the
// handler.
func (s *CommunityContribService) CountRecentByIP(ctx context.Context, ip string) (int, error) {
	if s.db == nil || ip == "" {
		return 0, nil
	}
	const q = `
		SELECT COUNT(*) FROM aftermarket_community
		WHERE contributor_ip = $1 AND submitted_at > NOW() - INTERVAL '24 hours'`
	var n int
	if err := s.db.QueryRowContext(ctx, q, ip).Scan(&n); err != nil {
		return 0, fmt.Errorf("count recent by ip: %w", err)
	}
	return n, nil
}

// ListPending returns unreviewed contributions, oldest first. limit
// clamped [1, 100].
func (s *CommunityContribService) ListPending(ctx context.Context, limit int) ([]CommunityContribution, error) {
	if s.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	const q = `
		SELECT id, oem_normalized, brand, part_number, description, source_url, notes,
		       COALESCE(contributor, ''), COALESCE(host(contributor_ip), ''), status, submitted_at
		FROM aftermarket_community
		WHERE status = 'pending'
		ORDER BY submitted_at ASC
		LIMIT $1`

	rows, err := s.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	defer rows.Close()

	var out []CommunityContribution
	for rows.Next() {
		var c CommunityContribution
		if err := rows.Scan(&c.ID, &c.OEMNormalized, &c.Brand, &c.PartNumber,
			&c.Description, &c.SourceURL, &c.Notes,
			&c.Contributor, &c.ContributorIP, &c.Status, &c.SubmittedAt); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// Review sets the status of a pending contribution to approved or rejected,
// stamps reviewedAt + reviewedBy + reviewNote.
func (s *CommunityContribService) Review(ctx context.Context, id int64, decision ContribStatus, reviewer, note string) error {
	if s.db == nil {
		return fmt.Errorf("community-contrib service not connected")
	}
	if decision != ContribApproved && decision != ContribRejected {
		return fmt.Errorf("decision must be 'approved' or 'rejected', got %q", decision)
	}
	const q = `
		UPDATE aftermarket_community
		SET status = $2, reviewed_at = NOW(), reviewed_by = $3, review_note = $4
		WHERE id = $1 AND status = 'pending'`

	res, err := s.db.ExecContext(ctx, q, id, string(decision), reviewer, truncate(note, 500))
	if err != nil {
		return fmt.Errorf("review contribution: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("contribution %d not found or already reviewed", id)
	}
	return nil
}

// FindApprovedByOEM returns approved community entries for an OEM. Used by
// FindAftermarketForOEM_MultiPath as its N-th union path.
func (s *CommunityContribService) FindApprovedByOEM(ctx context.Context, oem string) ([]CommunityContribution, error) {
	if s.db == nil {
		return nil, nil
	}
	clean := NormalizeOEM(oem)
	if clean == "" {
		return nil, nil
	}
	const q = `
		SELECT id, oem_normalized, brand, part_number, description, source_url, notes, status, submitted_at
		FROM aftermarket_community
		WHERE oem_normalized = $1 AND status = 'approved'
		ORDER BY submitted_at DESC
		LIMIT 20`

	rows, err := s.db.QueryContext(ctx, q, clean)
	if err != nil {
		return nil, fmt.Errorf("find approved by oem: %w", err)
	}
	defer rows.Close()

	var out []CommunityContribution
	for rows.Next() {
		var c CommunityContribution
		if err := rows.Scan(&c.ID, &c.OEMNormalized, &c.Brand, &c.PartNumber,
			&c.Description, &c.SourceURL, &c.Notes, &c.Status, &c.SubmittedAt); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

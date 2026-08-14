package service

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

type CommonsMediaStore struct {
	queries *store.Queries
}

func NewCommonsMediaStore(db *sql.DB) *CommonsMediaStore {
	if db == nil {
		return nil
	}
	return &CommonsMediaStore{queries: store.New(db)}
}

func (s *CommonsMediaStore) Submit(input model.CommonsMediaSubmission) (store.CommonsMediaReview, error) {
	if s == nil || s.queries == nil {
		return store.CommonsMediaReview{}, fmt.Errorf("Commons media review store is not available")
	}
	if err := validateCommonsMediaSubmission(input); err != nil {
		return store.CommonsMediaReview{}, err
	}
	return s.queries.CreateCommonsMediaReview(context.Background(), store.CreateCommonsMediaReviewParams{
		Title: strings.TrimSpace(input.Title), CategoryNorm: normalizeMediaCategory(input.Category),
		MediaUrl: strings.TrimSpace(input.MediaURL), ThumbnailUrl: strings.TrimSpace(input.ThumbnailURL),
		FilePageUrl: strings.TrimSpace(input.FilePageURL), LicenseName: strings.TrimSpace(input.LicenseName),
		LicenseUrl: strings.TrimSpace(input.LicenseURL), Attribution: strings.TrimSpace(input.Attribution),
		SourceRevision: strings.TrimSpace(input.SourceRevision),
	})
}

func (s *CommonsMediaStore) Review(id int64, input model.CommonsMediaReviewInput) (store.CommonsMediaReview, error) {
	if s == nil || s.queries == nil {
		return store.CommonsMediaReview{}, fmt.Errorf("Commons media review store is not available")
	}
	action, reviewer := strings.ToLower(strings.TrimSpace(input.Action)), strings.TrimSpace(input.Reviewer)
	if id <= 0 || (action != "approved" && action != "rejected") || reviewer == "" {
		return store.CommonsMediaReview{}, fmt.Errorf("valid id, approved or rejected action, and reviewer are required")
	}
	return s.queries.ReviewCommonsMedia(context.Background(), store.ReviewCommonsMediaParams{
		ID: id, ReviewStatus: action, ReviewNotes: strings.TrimSpace(input.ReviewNotes), ReviewedBy: reviewer,
	})
}

func (s *CommonsMediaStore) List() ([]store.CommonsMediaReview, error) {
	if s == nil || s.queries == nil {
		return nil, fmt.Errorf("Commons media review store is not available")
	}
	return s.queries.ListCommonsMediaReviews(context.Background())
}

func validateCommonsMediaSubmission(input model.CommonsMediaSubmission) error {
	if strings.TrimSpace(input.Title) == "" || normalizeMediaCategory(input.Category) == "" || strings.TrimSpace(input.Attribution) == "" {
		return fmt.Errorf("title, category, and attribution are required")
	}
	if input.LicenseName != "CC0" && input.LicenseName != "CC BY 4.0" {
		return fmt.Errorf("only CC0 and CC BY 4.0 Commons media may be submitted")
	}
	if !isAllowedCommonsURL(input.MediaURL, "upload.wikimedia.org") ||
		!isAllowedCommonsURL(input.FilePageURL, "commons.wikimedia.org") ||
		!isAllowedCommonsURL(input.LicenseURL, "creativecommons.org") {
		return fmt.Errorf("media, file page, and license URLs must be HTTPS Commons/Creative Commons URLs")
	}
	if input.ThumbnailURL != "" && !isAllowedCommonsURL(input.ThumbnailURL, "upload.wikimedia.org") {
		return fmt.Errorf("thumbnail URL must be an HTTPS Wikimedia upload URL")
	}
	return nil
}

func isAllowedCommonsURL(raw, host string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Host, host)
}

func normalizeMediaCategory(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

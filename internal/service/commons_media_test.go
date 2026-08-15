package service

import (
	"testing"

	"parts-engine/internal/model"
)

func TestValidateCommonsMediaSubmissionRequiresApprovedLicenseAndOrigins(t *testing.T) {
	valid := model.CommonsMediaSubmission{
		Title: "Hyundai engine illustration", Category: "engine oil filters",
		MediaURL:    "https://upload.wikimedia.org/example.jpg",
		FilePageURL: "https://commons.wikimedia.org/wiki/File:Example.jpg",
		LicenseName: "CC BY 4.0", LicenseURL: "https://creativecommons.org/licenses/by/4.0/",
		Attribution: "Example Author",
	}
	if err := validateCommonsMediaSubmission(valid); err != nil {
		t.Fatalf("valid Commons submission rejected: %v", err)
	}

	valid.LicenseName = "CC BY-SA 4.0"
	if err := validateCommonsMediaSubmission(valid); err == nil {
		t.Fatal("expected unsupported share-alike license to be rejected pending policy support")
	}
}

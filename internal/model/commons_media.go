package model

type CommonsMediaSubmission struct {
	Title          string `json:"title"`
	Category       string `json:"category"`
	MediaURL       string `json:"mediaUrl"`
	ThumbnailURL   string `json:"thumbnailUrl"`
	FilePageURL    string `json:"filePageUrl"`
	LicenseName    string `json:"licenseName"`
	LicenseURL     string `json:"licenseUrl"`
	Attribution    string `json:"attribution"`
	SourceRevision string `json:"sourceRevision"`
}

type CommonsMediaReviewInput struct {
	Action      string `json:"action"`
	Reviewer    string `json:"reviewer"`
	ReviewNotes string `json:"reviewNotes"`
}

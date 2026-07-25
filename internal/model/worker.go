package model

type WorkerReplacementSubmission struct {
	ID                     int    `json:"id"`
	PartArticleNumber      string `json:"partArticleNumber"`
	CandidateArticleNumber string `json:"candidateArticleNumber"`
	CandidateBrandName     string `json:"candidateBrandName,omitempty"`
	RelationType           string `json:"relationType"`
	EvidenceText           string `json:"evidenceText"`
	EvidenceSource         string `json:"evidenceSource"`
	Submitter              string `json:"submitter"`
	Status                 string `json:"status"`
	ReviewNotes            string `json:"reviewNotes,omitempty"`
	ReviewedBy             string `json:"reviewedBy,omitempty"`
	ReviewedAt             string `json:"reviewedAt,omitempty"`
	CreatedAt              string `json:"createdAt"`
}

type WorkerReplacementSubmissionInput struct {
	PartArticleNumber      string `json:"partArticleNumber"`
	CandidateArticleNumber string `json:"candidateArticleNumber"`
	CandidateBrandName     string `json:"candidateBrandName,omitempty"`
	RelationType           string `json:"relationType"`
	EvidenceText           string `json:"evidenceText"`
	EvidenceSource         string `json:"evidenceSource"`
	Submitter              string `json:"submitter"`
}

type WorkerReplacementReviewInput struct {
	Action      string `json:"action"`
	Reviewer    string `json:"reviewer"`
	ReviewNotes string `json:"reviewNotes"`
}

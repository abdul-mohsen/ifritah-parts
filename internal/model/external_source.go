package model

type ExternalSourceRecommendation string

const (
	ExternalSourceBackendEnrichment ExternalSourceRecommendation = "backend_enrichment"
	ExternalSourceResearchOnly      ExternalSourceRecommendation = "research_only"
	ExternalSourceRejected          ExternalSourceRecommendation = "rejected"
)

type ExternalSourceRecord struct {
	SourceKey            string
	DisplayName          string
	WebsiteURL           string
	DataType             string
	AccessMethod         string
	LicenseRisk          string
	HyundaiKiaUsefulness string
	MultiMakeUsefulness  string
	FalsePositiveRisk    string
	Recommendation       ExternalSourceRecommendation
	UserFacingEligible   bool
	FreshnessNotes       string
	RateLimitNotes       string
	Notes                string
}

type ExternalSourceAssessment struct {
	SourceKey           string
	SampleScope         string
	EvidenceScore       int
	PrecisionScore      int
	DuplicateNoiseScore int
	FalsePositiveScore  int
	QADecision          string
	Rationale           string
}

type ExternalPartReference struct {
	SourceKey      string
	PartNumberNorm string
	BrandNorm      string
	MakeNorm       string
	ModelNorm      string
	VehicleHint    string
	ExactMatch     bool
	Confidence     float64
	ProvenanceURL  string
}

type ExternalArtifact struct {
	SourceKey     string
	ArtifactType  string
	Title         string
	MediaURL      string
	ThumbnailURL  string
	MimeType      string
	Exactness     string
	Confidence    float64
	LicenseNote   string
	ProvenanceURL string
}

type ExternalInstallHint struct {
	SourceKey      string
	PartNumberNorm string
	MakeNorm       string
	ModelNorm      string
	HintType       string
	HintText       string
	Exactness      string
	Confidence     float64
	ProvenanceURL  string
}

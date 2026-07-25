package model

// OEMReference maps an OEM part number to aftermarket alternatives.
type OEMReference struct {
	RawNumber       string `json:"rawNumber"`
	Normalized      string `json:"normalized,omitempty"`
	Manufacturer    string `json:"manufacturer,omitempty"`
	BrandName       string `json:"brandName,omitempty"`
	ArticleNumber   string `json:"articleNumber,omitempty"`
	Description     string `json:"description,omitempty"`
	LegacyArticleId int    `json:"legacyArticleId"`
}

// OEMSearchResult is the response from the OEM lookup endpoint.
type OEMSearchResult struct {
	Query      string         `json:"query"`
	Normalized string         `json:"normalized"`
	Results    []OEMReference `json:"results"`
	Total      int            `json:"total"`
}

// Recall represents an NHTSA safety recall.
type Recall struct {
	NHTSACampaignNumber string `json:"nhtsaCampaignNumber"`
	Component           string `json:"component"`
	Summary             string `json:"summary"`
	Consequence         string `json:"consequence,omitempty"`
	Remedy              string `json:"remedy,omitempty"`
	ReportDate          string `json:"reportDate,omitempty"`
	SourceLabel         string `json:"sourceLabel"`
	SourceURL           string `json:"sourceUrl"`
	Warning             string `json:"warning,omitempty"`
}

// SearchResult is a unified search response (parts, OEM, text).
type SearchResult struct {
	Query   string `json:"query"`
	Type    string `json:"type"` // "part", "oem", "text"
	Results []any  `json:"results"`
	Total   int    `json:"total"`
}

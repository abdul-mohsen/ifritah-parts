package model

// OnlinePartResult represents a part looked up from an online source (e.g. PartsOuq).
type OnlinePartResult struct {
	PartNumber    string             `json:"partNumber"`
	Description   string             `json:"description"`
	Make          string             `json:"make"`
	Category      string             `json:"category,omitempty"`
	Substitutions []SubstitutionPart `json:"substitutions,omitempty"`
	Aftermarket   []AftermarketPart  `json:"aftermarket,omitempty"`
	Compatibility []string           `json:"compatibility,omitempty"`
	Source        string             `json:"source"` // e.g. "partsouq"
}

// SubstitutionPart is a replacement/supersession from the same OEM.
type SubstitutionPart struct {
	PartNumber  string `json:"partNumber"`
	Description string `json:"description"`
	Make        string `json:"make,omitempty"`
}

// AftermarketPart is a non-OEM alternative from a third-party brand.
type AftermarketPart struct {
	PartNumber  string `json:"partNumber"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

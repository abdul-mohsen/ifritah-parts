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
//
// M8 additions — the Source/SourceURL/PriceCents/Currency/Condition/ImageURL
// fields are populated only for online-sourced results (eBay Motors API,
// schema.org JSON-LD from public reference pages, etc.). Existing paths
// (articlecrosses, oem_number, oem_search_index, community, regional)
// leave these fields at their zero value and JSON `omitempty` hides them —
// so existing API consumers see no change to their contract.
type AftermarketPart struct {
	PartNumber  string `json:"partNumber"`
	Description string `json:"description"`
	Brand       string `json:"brand"`

	// M8: online-source provenance (optional; empty for TecDoc-sourced results)
	Source     string `json:"source,omitempty"`     // "online:ebay" / "online:hyundaipartsdeal" / etc.
	SourceURL  string `json:"sourceUrl,omitempty"`  // click-through URL for attribution
	PriceCents int64  `json:"priceCents,omitempty"` // 0 = unknown
	Currency   string `json:"currency,omitempty"`   // ISO 4217, e.g. "USD" / "EUR"
	Condition  string `json:"condition,omitempty"`  // "new" / "used" / "reman" / "unknown"
	ImageURL   string `json:"imageUrl,omitempty"`
}

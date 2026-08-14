package model

// Part represents an aftermarket part from the parts cache.
type Part struct {
	LegacyArticleId int    `json:"legacyArticleId"`
	ArticleNumber   string `json:"articleNumber"`
	Description     string `json:"description"`
	BrandName       string `json:"brandName,omitempty"`
	Category        string `json:"category,omitempty"`
	AssemblyGroupId int    `json:"assemblyGroupId,omitempty"`
}

// PartDetail includes OEM cross-references and supersession info.
type PartDetail struct {
	Part
	OEMNumbers   []OEMReference     `json:"oemNumbers,omitempty"`
	Supersession []SupersessionLink `json:"supersession,omitempty"`
	FitsVehicles []Vehicle          `json:"fitsVehicles,omitempty"`
}

// SupersessionLink represents one link in a replacement chain.
type SupersessionLink struct {
	LegacyArticleId int    `json:"legacyArticleId"`
	ArticleNumber   string `json:"articleNumber"`
	BrandName       string `json:"brandName,omitempty"`
	Direction       string `json:"direction"` // "replaced_by" or "replaces"
}

// CategoryInfo describes a part category with count and fitment driver.
type CategoryInfo struct {
	Name           string `json:"name"`
	PartCount      int    `json:"partCount"`
	FitmentDriver  string `json:"fitmentDriver"`
	DependencyType string `json:"dependencyType,omitempty"`
}

// EnrichedPart extends Part with OEM cross-references for genuine+aftermarket display.
type EnrichedPart struct {
	Part
	OEMNumbers   []string          `json:"oemNumbers,omitempty"`
	Genuine      bool              `json:"genuine"`
	Alternatives []AftermarketPart `json:"alternatives,omitempty"`
}

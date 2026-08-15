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

type PartPlacement struct {
	Kind          string          `json:"kind"`
	PlacementType string          `json:"placementType"`
	Title         string          `json:"title"`
	Summary       string          `json:"summary"`
	LocationArea  string          `json:"locationArea,omitempty"`
	ImageURL      string          `json:"imageUrl,omitempty"`
	ThumbnailURL  string          `json:"thumbnailUrl,omitempty"`
	Hints         []string        `json:"hints,omitempty"`
	Warnings      []string        `json:"warnings,omitempty"`
	Confidence    float64         `json:"confidence"`
	Source        PlacementSource `json:"source"`
}

type PlacementSource struct {
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

type ReplacementCandidate struct {
	Part
	CandidateType string            `json:"candidateType"`
	Explanation   string            `json:"explanation"`
	OEMReference  string            `json:"oemReference,omitempty"`
	Confidence    float64           `json:"confidence"`
	Source        ReplacementSource `json:"source"`
	Warnings      []string          `json:"warnings,omitempty"`
}

type ReplacementSource struct {
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

// SupersessionLink represents an explicit source link. A reported link is not
// presented as OEM-confirmed supersession unless its source says so.
type SupersessionLink struct {
	LegacyArticleId int               `json:"legacyArticleId"`
	ArticleNumber   string            `json:"articleNumber"`
	BrandName       string            `json:"brandName,omitempty"`
	Description     string            `json:"description,omitempty"`
	Direction       string            `json:"direction"` // "replaced_by", "replaces", "reported_replacement", "reported_predecessor", or "reported_related"
	Confidence      float64           `json:"confidence"`
	Source          ReplacementSource `json:"source"`
	Warnings        []string          `json:"warnings,omitempty"`
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

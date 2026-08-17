package model

// Document is a TecDoc articledocs row: an installation manual, a mounting
// diagram, a technical sheet, or a certification PDF that TecDoc has
// licensed for redistribution.
//
// URL is the fully qualified location (TecDoc CDN); FileName is the
// original filename for download UX. DocType is the TecDoc document
// classification (e.g. "MOUNTING_INSTRUCTIONS", "SAFETY_DATASHEET").
// Language is the ISO 639-1 code the document was translated to.
//
// IMPORTANT: BUGS.md ("Rejected for ingestion: Hyundai/Kia dealer pages,
// retail diagrams, official marketing/service portal media without a
// license") means callers MUST NOT synthesize Document rows from arbitrary
// web pages — only rows sourced directly from the TecDoc articledocs
// table may populate this type. LicensedSource captures the origin so
// the media-review workflow can audit provenance.
type Document struct {
	LegacyArticleId int    `json:"legacyArticleId"`
	URL             string `json:"url"`
	FileName        string `json:"fileName,omitempty"`
	DocType         string `json:"docType,omitempty"`
	Language        string `json:"language,omitempty"`
	LicensedSource  string `json:"licensedSource,omitempty"`
}

package model

// Specification is a single TecDoc articlecriteria row for a part
// (dimensions, torque, material, connector, etc.).
//
// Fields map directly to articlecriteria columns in the TecDoc MySQL schema:
//   - Name         <- criteriaDescription
//   - Value        <- rawValue
//   - Unit         <- criteriaUnitDescription (may be empty)
//   - CriteriaType <- criteriaId or criteriaType label (numeric / boolean / text)
//
// Provenance rule: the Source field records who supplied the criterion.
// Only TecDoc-sourced rows may claim manufacturer-confirmed specification
// status; anything else must be surfaced with a warning per BUGS.md.
type Specification struct {
	Name         string `json:"name"`
	Value        string `json:"value"`
	Unit         string `json:"unit,omitempty"`
	CriteriaType string `json:"criteriaType,omitempty"`
	Source       string `json:"source,omitempty"`
	Warning      string `json:"warning,omitempty"`
}

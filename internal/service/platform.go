package service

import (
	"database/sql"
	"strings"

	"parts-engine/internal/model"
)

// Platform provides Hyundai↔Kia cross-brand suggestions.
type Platform struct {
	db *sql.DB
}

func NewPlatform(db *sql.DB) *Platform {
	return &Platform{db: db}
}

// Known platform pairs (hardcoded fallback when DB table is empty)
var knownPlatforms = []struct {
	hyundai  string
	kia      string
	platform string
}{
	{"TUCSON", "SPORTAGE", "NX4/NQ5"},
	{"ELANTRA", "FORTE", "CN7/BD"},
	{"ELANTRA", "CERATO", "CN7/BD"},
	{"SONATA", "K5", "DN8/DL3"},
	{"SONATA", "OPTIMA", "DN8/DL3"},
	{"SANTA FE", "SORENTO", "TM/MQ4"},
	{"VENUE", "SONET", "QX"},
	{"CRETA", "SELTOS", "SU2/SP2"},
	{"I20", "RIO", "BC3/YB"},
	{"KONA", "SELTOS", "OS/SP2"},
	{"ACCENT", "RIO", "HC/YB"},
	{"PALISADE", "TELLURIDE", "LX2/ON"},
	{"I30", "CEED", "PD/CD"},
	{"IONIQ 5", "EV6", "E-GMP"},
	{"IONIQ 6", "EV6", "E-GMP"},
	{"STARIA", "CARNIVAL", "US4/KA4"},
	{"SANTA CRUZ", "SPORTAGE", "NX4"},
	{"VELOSTER", "FORTE", "JS/BD"},
	{"GENESIS GV70", "SPORTAGE", "JK1/NQ5"},
	{"NEXO", "NIRO", "FE/DE"},
}

// FindSiblings returns cross-brand vehicle matches for a given model.
func (s *Platform) FindSiblings(make, modelName string) ([]model.CrossBrandHit, error) {
	// Try DB first
	if s.db != nil {
		hits, err := s.findSiblingsDB(make, modelName)
		if err == nil && len(hits) > 0 {
			return hits, nil
		}
	}

	// Fallback to hardcoded pairs
	return s.findSiblingsFallback(make, modelName), nil
}

func (s *Platform) findSiblingsDB(make, modelName string) ([]model.CrossBrandHit, error) {
	var query string
	var arg string

	if make == "HYUNDAI" {
		query = `SELECT 'KIA' AS sibling_make, kia_model AS sibling_model, platform_code
		         FROM hk_platform_map WHERE UPPER(hyundai_model) = ?`
		arg = modelName
	} else if make == "KIA" {
		query = `SELECT 'HYUNDAI' AS sibling_make, hyundai_model AS sibling_model, platform_code
		         FROM hk_platform_map WHERE UPPER(kia_model) = ?`
		arg = modelName
	} else {
		return nil, nil
	}

	rows, err := s.db.Query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []model.CrossBrandHit
	for rows.Next() {
		var h model.CrossBrandHit
		if err := rows.Scan(&h.SiblingMake, &h.SiblingModel, &h.Platform); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, nil
}

func (s *Platform) findSiblingsFallback(make, modelName string) []model.CrossBrandHit {
	upper := strings.ToUpper(modelName)
	var hits []model.CrossBrandHit

	for _, p := range knownPlatforms {
		if make == "HYUNDAI" && strings.EqualFold(p.hyundai, upper) {
			hits = append(hits, model.CrossBrandHit{
				SiblingMake:  "KIA",
				SiblingModel: p.kia,
				Platform:     p.platform,
			})
		} else if make == "KIA" && strings.EqualFold(p.kia, upper) {
			hits = append(hits, model.CrossBrandHit{
				SiblingMake:  "HYUNDAI",
				SiblingModel: p.hyundai,
				Platform:     p.platform,
			})
		}
	}
	return hits
}

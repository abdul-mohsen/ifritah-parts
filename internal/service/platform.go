package service

import (
	"context"
	"database/sql"
	"strings"

	"parts-engine/internal/model"
	"parts-engine/internal/store"
)

type Platform struct {
	db      *sql.DB
	queries *store.Queries
}

func NewPlatform(db *sql.DB) *Platform {
	if db == nil {
		return &Platform{}
	}
	return &Platform{db: db, queries: store.New(db)}
}

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
	{"IONIQ 5", "EV6", "E-GMP"},
	{"IONIQ 6", "EV6", "E-GMP"},
}

func (s *Platform) FindSiblings(make, modelName string) ([]model.CrossBrandHit, error) {
	if s.queries != nil {
		hits, err := s.findSiblingsDB(make, modelName)
		if err == nil && len(hits) > 0 {
			return hits, nil
		}
	}
	return s.findSiblingsFallback(make, modelName), nil
}

func (s *Platform) findSiblingsDB(make, modelName string) ([]model.CrossBrandHit, error) {
	ctx := context.Background()
	var hits []model.CrossBrandHit
	switch strings.ToUpper(make) {
	case "HYUNDAI":
		rows, err := s.queries.FindPlatformSiblingsForHyundai(ctx, modelName)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			hits = append(hits, model.CrossBrandHit{SiblingMake: row.SiblingMake, SiblingModel: row.SiblingModel, Platform: row.PlatformCode})
		}
	case "KIA":
		rows, err := s.queries.FindPlatformSiblingsForKia(ctx, modelName)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			hits = append(hits, model.CrossBrandHit{SiblingMake: row.SiblingMake, SiblingModel: row.SiblingModel, Platform: row.PlatformCode})
		}
	}
	return hits, nil
}

func (s *Platform) findSiblingsFallback(make, modelName string) []model.CrossBrandHit {
	upper := strings.ToUpper(modelName)
	var hits []model.CrossBrandHit
	for _, p := range knownPlatforms {
		if strings.EqualFold(make, "HYUNDAI") && strings.EqualFold(p.hyundai, upper) {
			hits = append(hits, model.CrossBrandHit{SiblingMake: "KIA", SiblingModel: p.kia, Platform: p.platform})
		}
		if strings.EqualFold(make, "KIA") && strings.EqualFold(p.kia, upper) {
			hits = append(hits, model.CrossBrandHit{SiblingMake: "HYUNDAI", SiblingModel: p.hyundai, Platform: p.platform})
		}
	}
	return hits
}

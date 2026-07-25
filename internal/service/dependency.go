package service

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
)

// DependencyType classifies how strictly a part category depends on engine specs.
type DependencyType string

const (
	DepEngineStrict   DependencyType = "ENGINE_STRICT"   // CC spread < 500 — must match engine code
	DepEngineModerate DependencyType = "ENGINE_MODERATE" // CC spread 500-1500 — match CC range + fuel
	DepPlatform       DependencyType = "PLATFORM"        // CC spread 1500-3000 — model + body style
	DepUniversal      DependencyType = "UNIVERSAL"       // CC spread > 3000 — make/model/year only
)

// DependencyClassifier maps assembly group nodes and part descriptions to dependency types
// based on CC spread analysis of the parts cache.
type DependencyClassifier struct {
	db *sql.DB

	mu      sync.RWMutex
	byGroup map[int]DependencyType    // assemblyGroupNodeId → type
	byDesc  map[string]DependencyType // genericArticleDesc → type
	loaded  bool
}

func NewDependencyClassifier(db *sql.DB) *DependencyClassifier {
	return &DependencyClassifier{
		db:      db,
		byGroup: make(map[int]DependencyType),
		byDesc:  make(map[string]DependencyType),
	}
}

// Load queries hk_parts_cache once to build both classification maps.
// Call at server startup.
func (d *DependencyClassifier) Load() error {
	if d.db == nil {
		return fmt.Errorf("database not connected")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Build by assemblyGroupNodeId
	groupQuery := `
		SELECT assemblyGroupNodeId,
		       COUNT(DISTINCT legacyArticleId) AS parts,
		       MAX(capacityCC) - MIN(capacityCC) AS spread
		FROM hk_parts_cache
		WHERE capacityCC > 0
		GROUP BY assemblyGroupNodeId
		HAVING parts >= 3`

	rows, err := d.db.Query(groupQuery)
	if err != nil {
		return fmt.Errorf("load group deps: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var groupId, parts, spread int
		if err := rows.Scan(&groupId, &parts, &spread); err != nil {
			return fmt.Errorf("scan group dep: %w", err)
		}
		d.byGroup[groupId] = classifySpread(spread)
	}

	// Build by genericArticleDesc
	descQuery := `
		SELECT genericArticleDesc,
		       COUNT(DISTINCT legacyArticleId) AS parts,
		       MAX(capacityCC) - MIN(capacityCC) AS spread
		FROM hk_parts_cache
		WHERE capacityCC > 0 AND genericArticleDesc IS NOT NULL AND genericArticleDesc != ''
		GROUP BY genericArticleDesc
		HAVING parts >= 3`

	rows2, err := d.db.Query(descQuery)
	if err != nil {
		return fmt.Errorf("load desc deps: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var desc string
		var parts, spread int
		if err := rows2.Scan(&desc, &parts, &spread); err != nil {
			return fmt.Errorf("scan desc dep: %w", err)
		}
		d.byDesc[desc] = classifySpread(spread)
	}

	d.loaded = true
	log.Printf("✓ Dependency classifier loaded: %d groups, %d descriptions", len(d.byGroup), len(d.byDesc))
	return nil
}

func classifySpread(spread int) DependencyType {
	switch {
	case spread < 500:
		return DepEngineStrict
	case spread < 1500:
		return DepEngineModerate
	case spread < 3000:
		return DepPlatform
	default:
		return DepUniversal
	}
}

// ClassifyGroup returns the dependency type for an assemblyGroupNodeId.
func (d *DependencyClassifier) ClassifyGroup(assemblyGroupNodeId int) DependencyType {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if dt, ok := d.byGroup[assemblyGroupNodeId]; ok {
		return dt
	}
	return DepUniversal
}

// ClassifyDesc returns the dependency type for a genericArticleDesc.
func (d *DependencyClassifier) ClassifyDesc(genericArticleDesc string) DependencyType {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if dt, ok := d.byDesc[genericArticleDesc]; ok {
		return dt
	}
	// Fallback: use existing category.go keyword matching
	rule := ClassifyCategory(genericArticleDesc)
	switch rule.Driver {
	case FitEngine:
		if rule.Strict {
			return DepEngineModerate
		}
		return DepPlatform
	case FitBrake:
		return DepPlatform
	case FitBody, FitDrivetrain:
		return DepPlatform
	default:
		return DepUniversal
	}
}

// IsEngineDependent returns true if the category requires engine-code filtering.
func (d *DependencyClassifier) IsEngineDependent(assemblyGroupNodeId int, genericArticleDesc string) bool {
	dt := d.ClassifyGroup(assemblyGroupNodeId)
	if dt == DepEngineStrict || dt == DepEngineModerate {
		return true
	}
	dt = d.ClassifyDesc(genericArticleDesc)
	return dt == DepEngineStrict || dt == DepEngineModerate
}

// IsStrict returns true if the category requires exact engine code match.
func (d *DependencyClassifier) IsStrict(assemblyGroupNodeId int, genericArticleDesc string) bool {
	if dt := d.ClassifyGroup(assemblyGroupNodeId); dt == DepEngineStrict {
		return true
	}
	return d.ClassifyDesc(genericArticleDesc) == DepEngineStrict
}

// Loaded returns true if classification data has been loaded.
func (d *DependencyClassifier) Loaded() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.loaded
}

package service

// CategoryTree builds a 2-level hierarchical grouping of the flat assembly-group
// categories that already exist in hk_parts_cache. The parent groups are inferred
// from the category names using a static mapping. When connected to MySQL with
// assemblygroupnodenames available, the real tree is used instead.

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
)

// CategoryGroup is a top-level group in the parts catalog tree.
type CategoryGroup struct {
	Name       string         `json:"name"`
	Icon       string         `json:"icon,omitempty"`
	Categories []CategoryLeaf `json:"categories"`
	TotalParts int            `json:"totalParts"`
}

// CategoryLeaf is a leaf category that contains actual parts.
type CategoryLeaf struct {
	Name          string `json:"name"`
	AssemblyGroup int    `json:"assemblyGroupId,omitempty"`
	PartCount     int    `json:"partCount"`
}

// CategoryTree manages the hierarchical category structure.
type CategoryTree struct {
	db      *sql.DB
	offline bool

	mu       sync.RWMutex
	tree     []CategoryGroup   // cached tree (vehicle-independent structure)
	groupMap map[string]string // categoryName → parent group name
}

// parentMapping defines the 2-level hierarchy.
// Key = parent group name, Value = list of category name substrings to match.
var parentMapping = map[string][]string{
	"Engine & Drivetrain": {
		"Air Intake", "Cooling System", "Engine Mounts", "Engine Oil",
		"Exhaust System", "Fuel System", "Ignition System", "Timing",
	},
	"Brakes": {
		"Front Brake", "Rear Brake", "Brake Hydraulic", "ABS", "Wheel Speed",
	},
	"Suspension & Steering": {
		"Front Suspension", "Rear Suspension", "Steering",
	},
	"Body & Exterior": {
		"Body Panel", "Headlight", "Rear Light", "Mirror", "Glass", "Wiper",
	},
	"Interior & Climate": {
		"Cabin Filter", "Blower", "HVAC", "Climate",
	},
	"Electrical & Sensors": {
		"Electrical", "Sensor",
	},
	"Transmission & Clutch": {
		"Clutch", "Drive Shaft", "Transmission",
	},
}

// parentIcons maps each parent group to an icon hint for the frontend.
var parentIcons = map[string]string{
	"Engine & Drivetrain":   "engine",
	"Brakes":                "brake",
	"Suspension & Steering": "suspension",
	"Body & Exterior":       "body",
	"Interior & Climate":    "climate",
	"Electrical & Sensors":  "electrical",
	"Transmission & Clutch": "transmission",
}

// parentOrder defines display ordering.
var parentOrder = []string{
	"Engine & Drivetrain",
	"Brakes",
	"Suspension & Steering",
	"Body & Exterior",
	"Interior & Climate",
	"Electrical & Sensors",
	"Transmission & Clutch",
}

func NewCategoryTree(db *sql.DB, offline bool) *CategoryTree {
	ct := &CategoryTree{
		db:       db,
		offline:  offline,
		groupMap: make(map[string]string),
	}
	// Build the lookup map: category substring → parent group
	for parent, keywords := range parentMapping {
		for _, kw := range keywords {
			ct.groupMap[strings.ToLower(kw)] = parent
		}
	}
	return ct
}

// resolveParent finds the parent group for a category name.
func (ct *CategoryTree) resolveParent(categoryName string) string {
	lower := strings.ToLower(categoryName)
	for kw, parent := range ct.groupMap {
		if strings.Contains(lower, kw) {
			return parent
		}
	}
	return "Other"
}

// GetTreeForVehicle returns the hierarchical category tree for a specific vehicle,
// with accurate part counts per leaf category.
func (ct *CategoryTree) GetTreeForVehicle(linkageTargetId int) ([]CategoryGroup, error) {
	if ct.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	// First try MySQL assemblygroupnodenames if available
	if !ct.offline {
		tree, err := ct.getFromAssemblyGroupNames(linkageTargetId)
		if err == nil && len(tree) > 0 {
			return tree, nil
		}
		// Fall through to hk_parts_cache
	}

	return ct.getFromPartsCache(linkageTargetId)
}

// getFromPartsCache builds the tree from hk_parts_cache flat categories.
func (ct *CategoryTree) getFromPartsCache(linkageTargetId int) ([]CategoryGroup, error) {
	query := `
		SELECT categoryName, assemblyGroupNodeId, COUNT(DISTINCT legacyArticleId) AS partCount
		FROM hk_parts_cache
		WHERE linkingTargetId = ?
		  AND categoryName IS NOT NULL AND categoryName != ''
		GROUP BY categoryName, assemblyGroupNodeId
		ORDER BY categoryName`

	rows, err := ct.db.Query(query, linkageTargetId)
	if err != nil {
		return nil, fmt.Errorf("category tree: %w", err)
	}
	defer rows.Close()

	// Collect leaves and assign to parent groups
	groupLeaves := make(map[string][]CategoryLeaf)
	for rows.Next() {
		var name string
		var groupId, count int
		if err := rows.Scan(&name, &groupId, &count); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}

		parent := ct.resolveParent(name)
		groupLeaves[parent] = append(groupLeaves[parent], CategoryLeaf{
			Name:          name,
			AssemblyGroup: groupId,
			PartCount:     count,
		})
	}

	// Build ordered tree
	var tree []CategoryGroup
	for _, parentName := range parentOrder {
		leaves, ok := groupLeaves[parentName]
		if !ok || len(leaves) == 0 {
			continue
		}
		sort.Slice(leaves, func(i, j int) bool {
			return leaves[i].Name < leaves[j].Name
		})
		total := 0
		for _, l := range leaves {
			total += l.PartCount
		}
		tree = append(tree, CategoryGroup{
			Name:       parentName,
			Icon:       parentIcons[parentName],
			Categories: leaves,
			TotalParts: total,
		})
	}

	// Add "Other" bucket if any categories didn't match
	if others, ok := groupLeaves["Other"]; ok && len(others) > 0 {
		sort.Slice(others, func(i, j int) bool {
			return others[i].Name < others[j].Name
		})
		total := 0
		for _, l := range others {
			total += l.PartCount
		}
		tree = append(tree, CategoryGroup{
			Name:       "Other",
			Icon:       "other",
			Categories: others,
			TotalParts: total,
		})
	}

	return tree, nil
}

// getFromAssemblyGroupNames tries the real TecDoc assembly group hierarchy.
// Returns empty if the table is unavailable or has no English names.
func (ct *CategoryTree) getFromAssemblyGroupNames(linkageTargetId int) ([]CategoryGroup, error) {
	// Check if assemblygroupnodenames table exists and has data
	var cnt int
	err := ct.db.QueryRow(`SELECT COUNT(*) FROM assemblygroupnodenames WHERE lang IN ('en','EN') LIMIT 1`).Scan(&cnt)
	if err != nil || cnt == 0 {
		return nil, fmt.Errorf("no assembly group names available")
	}

	// Query parts for this vehicle joined to real assembly group hierarchy
	query := `
		SELECT agn.assemblyGroupName, agn.parentNodeId, avt.assemblyGroupNodeId,
		       COUNT(DISTINCT avt.legacyArticleId) AS partCount
		FROM articlesvehicletrees avt
		JOIN assemblygroupnodenames agn
		  ON agn.assemblyGroupNodeId = avt.assemblyGroupNodeId
		  AND agn.lang = 'en'
		WHERE avt.linkingTargetId = ?
		  AND avt.linkingTargetType = 'P'
		GROUP BY agn.assemblyGroupName, agn.parentNodeId, avt.assemblyGroupNodeId
		ORDER BY agn.assemblyGroupName`

	rows, err := ct.db.Query(query, linkageTargetId)
	if err != nil {
		return nil, fmt.Errorf("assembly group tree: %w", err)
	}
	defer rows.Close()

	type rawGroup struct {
		name      string
		parentId  int
		groupId   int
		partCount int
	}

	var raw []rawGroup
	for rows.Next() {
		var r rawGroup
		if err := rows.Scan(&r.name, &r.parentId, &r.groupId, &r.partCount); err != nil {
			continue
		}
		raw = append(raw, r)
	}

	if len(raw) == 0 {
		return nil, fmt.Errorf("no groups found")
	}

	// For now, use our local hierarchy resolver on the TecDoc names too
	groupLeaves := make(map[string][]CategoryLeaf)
	for _, r := range raw {
		parent := ct.resolveParent(r.name)
		groupLeaves[parent] = append(groupLeaves[parent], CategoryLeaf{
			Name:          r.name,
			AssemblyGroup: r.groupId,
			PartCount:     r.partCount,
		})
	}

	var tree []CategoryGroup
	for _, parentName := range parentOrder {
		leaves, ok := groupLeaves[parentName]
		if !ok || len(leaves) == 0 {
			continue
		}
		sort.Slice(leaves, func(i, j int) bool {
			return leaves[i].Name < leaves[j].Name
		})
		total := 0
		for _, l := range leaves {
			total += l.PartCount
		}
		tree = append(tree, CategoryGroup{
			Name:       parentName,
			Icon:       parentIcons[parentName],
			Categories: leaves,
			TotalParts: total,
		})
	}

	if others, ok := groupLeaves["Other"]; ok && len(others) > 0 {
		total := 0
		for _, l := range others {
			total += l.PartCount
		}
		tree = append(tree, CategoryGroup{
			Name:       "Other",
			Icon:       "other",
			Categories: others,
			TotalParts: total,
		})
	}

	log.Printf("✓ Category tree built from assemblygroupnodenames: %d groups", len(tree))
	return tree, nil
}

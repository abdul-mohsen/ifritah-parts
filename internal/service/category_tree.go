package service

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"

	"parts-engine/internal/store"
)

type CategoryGroup struct {
	Name       string         `json:"name"`
	Icon       string         `json:"icon,omitempty"`
	Categories []CategoryLeaf `json:"categories"`
	TotalParts int            `json:"totalParts"`
}

type CategoryLeaf struct {
	Name          string `json:"name"`
	AssemblyGroup int    `json:"assemblyGroupId,omitempty"`
	PartCount     int    `json:"partCount"`
}

type CategoryTree struct {
	db       *sql.DB
	queries  *store.Queries
	mu       sync.RWMutex
	groupMap map[string]string
}

var parentMapping = map[string][]string{
	"Engine & Drivetrain":   {"Air Intake", "Cooling System", "Engine Mounts", "Engine Oil", "Exhaust System", "Fuel System", "Ignition System", "Timing"},
	"Brakes":                {"Front Brake", "Rear Brake", "Brake Hydraulic", "ABS", "Wheel Speed"},
	"Suspension & Steering": {"Front Suspension", "Rear Suspension", "Steering"},
	"Body & Exterior":       {"Body Panel", "Headlight", "Rear Light", "Mirror", "Glass", "Wiper"},
	"Interior & Climate":    {"Cabin Filter", "Blower", "HVAC", "Climate"},
	"Electrical & Sensors":  {"Electrical", "Sensor"},
	"Transmission & Clutch": {"Clutch", "Drive Shaft", "Transmission"},
}

var parentIcons = map[string]string{
	"Engine & Drivetrain":   "engine",
	"Brakes":                "brake",
	"Suspension & Steering": "suspension",
	"Body & Exterior":       "body",
	"Interior & Climate":    "climate",
	"Electrical & Sensors":  "electrical",
	"Transmission & Clutch": "transmission",
}

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
	_ = offline
	ct := &CategoryTree{
		db:       db,
		groupMap: make(map[string]string),
	}
	if db != nil {
		ct.queries = store.New(db)
	}
	for parent, keywords := range parentMapping {
		for _, kw := range keywords {
			ct.groupMap[strings.ToLower(kw)] = parent
		}
	}
	return ct
}

func (ct *CategoryTree) resolveParent(categoryName string) string {
	lower := strings.ToLower(categoryName)
	for kw, parent := range ct.groupMap {
		if strings.Contains(lower, kw) {
			return parent
		}
	}
	return "Other"
}

func (ct *CategoryTree) GetTreeForVehicle(linkageTargetId int) ([]CategoryGroup, error) {
	if ct.queries == nil {
		return nil, fmt.Errorf("database not connected")
	}

	rows, err := ct.queries.CategoryTreeLeavesByVehicle(context.Background(), int32(linkageTargetId))
	if err != nil {
		return nil, fmt.Errorf("category tree: %w", err)
	}

	groupLeaves := make(map[string][]CategoryLeaf)
	for _, row := range rows {
		parent := ct.resolveParent(row.CategoryName)
		groupLeaves[parent] = append(groupLeaves[parent], CategoryLeaf{
			Name:          row.CategoryName,
			AssemblyGroup: int(row.AssemblyGroupNodeID),
			PartCount:     int(row.PartCount),
		})
	}

	var tree []CategoryGroup
	for _, parentName := range parentOrder {
		leaves := groupLeaves[parentName]
		if len(leaves) == 0 {
			continue
		}
		sort.Slice(leaves, func(i, j int) bool { return leaves[i].Name < leaves[j].Name })
		total := 0
		for _, l := range leaves {
			total += l.PartCount
		}
		tree = append(tree, CategoryGroup{Name: parentName, Icon: parentIcons[parentName], Categories: leaves, TotalParts: total})
	}
	if others := groupLeaves["Other"]; len(others) > 0 {
		sort.Slice(others, func(i, j int) bool { return others[i].Name < others[j].Name })
		total := 0
		for _, l := range others {
			total += l.PartCount
		}
		tree = append(tree, CategoryGroup{Name: "Other", Icon: "other", Categories: others, TotalParts: total})
	}
	return tree, nil
}

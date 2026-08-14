package service

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
)

// EngineInfo describes one motor code and its specs.
type EngineInfo struct {
	MotorCode  string `json:"motorCode"`
	CC         int    `json:"cc"`
	FuelType   string `json:"fuelType,omitempty"`
	Cylinders  int    `json:"cylinders,omitempty"`
	PowerHP    int    `json:"powerHP,omitempty"`
	PowerKW    int    `json:"powerKW,omitempty"`
	EngineType string `json:"engineType,omitempty"`
}

// EngineResolver resolves vehicles to their TecDoc motor codes.
type EngineResolver struct {
	db *sql.DB

	mu    sync.RWMutex
	cache map[int][]EngineInfo // linkageTargetId → engines
}

func NewEngineResolver(db *sql.DB) *EngineResolver {
	return &EngineResolver{
		db:    db,
		cache: make(map[int][]EngineInfo),
	}
}

// ResolveByLinkageTarget returns motor codes for a linkageTargetId.
// Path: linkagetargets → car_link → cars → vehiclemotorcodes → motordetails
func (r *EngineResolver) ResolveByLinkageTarget(linkageTargetId int) ([]EngineInfo, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	r.mu.RLock()
	if cached, ok := r.cache[linkageTargetId]; ok {
		r.mu.RUnlock()
		return cached, nil
	}
	r.mu.RUnlock()

	query := `
		SELECT DISTINCT vmc.motorCode, md.cylinderCapacity, md.fuelType,
		       md.cylinderCount, md.powerHP, md.powerKW, md.motorType
		FROM car_link cl
		JOIN vehiclemotorcodes vmc ON vmc.carId = cl.carId
		JOIN motordetails md ON md.motorCode = vmc.motorCode
		WHERE cl.linkageTargetId = ?`

	rows, err := r.db.Query(query, linkageTargetId)
	if err != nil {
		return nil, fmt.Errorf("resolve engine: %w", err)
	}
	defer rows.Close()

	var engines []EngineInfo
	for rows.Next() {
		var e EngineInfo
		var fuel, mtype sql.NullString
		var cyl, hp, kw sql.NullInt32
		if err := rows.Scan(&e.MotorCode, &e.CC, &fuel, &cyl, &hp, &kw, &mtype); err != nil {
			return nil, fmt.Errorf("scan engine: %w", err)
		}
		e.FuelType = fuel.String
		if cyl.Valid {
			e.Cylinders = int(cyl.Int32)
		}
		if hp.Valid {
			e.PowerHP = int(hp.Int32)
		}
		if kw.Valid {
			e.PowerKW = int(kw.Int32)
		}
		e.EngineType = mtype.String
		engines = append(engines, e)
	}

	r.mu.Lock()
	r.cache[linkageTargetId] = engines
	r.mu.Unlock()

	return engines, nil
}

// ResolveBySpecs finds motor codes matching CC and fuel type via motordetails directly.
// Used when linkageTargetId bridge is unavailable.
func (r *EngineResolver) ResolveBySpecs(cc int, fuelType string) ([]EngineInfo, error) {
	if r.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	// Allow ±150cc tolerance for fingerprint matching
	margin := 150
	query := `
		SELECT DISTINCT motorCode, cylinderCapacity, fuelType,
		       cylinderCount, powerHP, powerKW, motorType
		FROM motordetails
		WHERE cylinderCapacity BETWEEN ? AND ?`
	args := []any{cc - margin, cc + margin}

	if fuelType != "" {
		query += " AND fuelType LIKE ?"
		args = append(args, "%"+fuelType+"%")
	}
	query += " ORDER BY ABS(cylinderCapacity - ?) LIMIT 20"
	args = append(args, cc)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("resolve by specs: %w", err)
	}
	defer rows.Close()

	var engines []EngineInfo
	for rows.Next() {
		var e EngineInfo
		var fuel, mtype sql.NullString
		var cyl, hp, kw sql.NullInt32
		if err := rows.Scan(&e.MotorCode, &e.CC, &fuel, &cyl, &hp, &kw, &mtype); err != nil {
			return nil, fmt.Errorf("scan engine spec: %w", err)
		}
		e.FuelType = fuel.String
		if cyl.Valid {
			e.Cylinders = int(cyl.Int32)
		}
		if hp.Valid {
			e.PowerHP = int(hp.Int32)
		}
		if kw.Valid {
			e.PowerKW = int(kw.Int32)
		}
		e.EngineType = mtype.String
		engines = append(engines, e)
	}
	return engines, nil
}

// ResolveForVehicle resolves engines using the best available method:
// 1. Try linkageTargetId bridge (most precise)
// 2. Fall back to CC + fuelType fingerprint (less precise)
func (r *EngineResolver) ResolveForVehicle(linkageTargetId int, cc int, fuelType string) ([]EngineInfo, error) {
	// Try precise bridge first
	engines, err := r.ResolveByLinkageTarget(linkageTargetId)
	if err == nil && len(engines) > 0 {
		return engines, nil
	}
	if err != nil {
		log.Printf("engine bridge failed for lt=%d: %v (falling back to specs)", linkageTargetId, err)
	}

	// Fallback to CC + fuel fingerprint
	if cc > 0 {
		return r.ResolveBySpecs(cc, fuelType)
	}

	return nil, nil
}

// MotorCodes returns just the motor code strings from EngineInfo slice.
func MotorCodes(engines []EngineInfo) []string {
	codes := make([]string, 0, len(engines))
	seen := make(map[string]bool)
	for _, e := range engines {
		if e.MotorCode != "" && !seen[e.MotorCode] {
			codes = append(codes, e.MotorCode)
			seen[e.MotorCode] = true
		}
	}
	return codes
}

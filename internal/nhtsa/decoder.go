package nhtsa

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

// DecodeResult holds the decoded VIN fields from the NHTSA vPIC database.
type DecodeResult struct {
	Make         string
	Model        string
	ModelYear    int
	BodyClass    string
	DriveType    string
	FuelType     string
	EngineCC     string
	EngineCyl    string
	PlantCountry string
	Trim         string
	Series       string
	VehicleType  string
}

// Decoder queries the NHTSA vPIC SQLite database to decode VINs.
type Decoder struct {
	db *sql.DB
	mu sync.RWMutex // protects db

	// cached element ID → code mapping
	elemCodes map[int]string
	// cached lookup table queries
	lookupCache map[string]map[int]string
}

// NewDecoder opens the NHTSA vPIC SQLite database.
func NewDecoder(dbPath string) (*Decoder, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("nhtsa: open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("nhtsa: ping db: %w", err)
	}
	db.SetMaxOpenConns(4)

	d := &Decoder{
		db:          db,
		lookupCache: make(map[string]map[int]string),
	}

	if err := d.loadElementCodes(); err != nil {
		db.Close()
		return nil, err
	}
	if err := d.preloadLookups(); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

func (d *Decoder) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// loadElementCodes caches Element.Id → Element.Code mapping.
func (d *Decoder) loadElementCodes() error {
	rows, err := d.db.Query("SELECT Id, Code FROM Element")
	if err != nil {
		return fmt.Errorf("nhtsa: load elements: %w", err)
	}
	defer rows.Close()

	d.elemCodes = make(map[int]string)
	for rows.Next() {
		var id int
		var code string
		if err := rows.Scan(&id, &code); err != nil {
			return err
		}
		d.elemCodes[id] = code
	}
	return rows.Err()
}

// preloadLookups caches small lookup tables (Make, Model, Country, etc.)
func (d *Decoder) preloadLookups() error {
	tables := []string{"Make", "Model", "Country", "VehicleType", "BodyStyle",
		"DriveType", "FuelType", "EngineConfiguration"}
	for _, table := range tables {
		m := make(map[int]string)
		rows, err := d.db.Query(fmt.Sprintf("SELECT Id, Name FROM %s", table))
		if err != nil {
			return fmt.Errorf("nhtsa: preload %s: %w", table, err)
		}
		for rows.Next() {
			var id int
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				rows.Close()
				return err
			}
			m[id] = name
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		d.lookupCache[table] = m
	}
	return nil
}

var yearCodes = map[byte]int{
	'A': 2010, 'B': 2011, 'C': 2012, 'D': 2013, 'E': 2014,
	'F': 2015, 'G': 2016, 'H': 2017, 'J': 2018, 'K': 2019,
	'L': 2020, 'M': 2021, 'N': 2022, 'P': 2023, 'R': 2024,
	'S': 2025, 'T': 2026, 'V': 2027, 'W': 2028, 'X': 2029,
	'Y': 2030,
	'1': 2001, '2': 2002, '3': 2003, '4': 2004, '5': 2005,
	'6': 2006, '7': 2007, '8': 2008, '9': 2009,
}

// Decode decodes a 17-char VIN using the NHTSA vPIC SQLite database.
// Returns nil if the WMI is not found in the database.
func (d *Decoder) Decode(vin string) (*DecodeResult, error) {
	vin = strings.ToUpper(strings.TrimSpace(vin))
	if len(vin) != 17 {
		return nil, fmt.Errorf("nhtsa: VIN must be 17 characters")
	}

	wmi := vin[0:3]
	year := 0
	if y, ok := yearCodes[vin[9]]; ok {
		year = y
	}

	// Step 1: Find WMI ID and Make
	var wmiID int
	var makeID sql.NullInt64
	err := d.db.QueryRow("SELECT Id, MakeId FROM Wmi WHERE Wmi = ?", wmi).Scan(&wmiID, &makeID)
	if err == sql.ErrNoRows {
		return nil, nil // WMI not in NHTSA DB
	}
	if err != nil {
		return nil, fmt.Errorf("nhtsa: wmi lookup: %w", err)
	}

	// Resolve Make from WMI (try MakeId first, then Wmi_Make table)
	wmiMake := ""
	if makeID.Valid {
		if name, ok := d.lookupCache["Make"][int(makeID.Int64)]; ok {
			wmiMake = name
		}
	}
	if wmiMake == "" {
		var wmMakeID int
		err = d.db.QueryRow("SELECT MakeId FROM Wmi_Make WHERE WmiId = ? LIMIT 1", wmiID).Scan(&wmMakeID)
		if err == nil {
			if name, ok := d.lookupCache["Make"][wmMakeID]; ok {
				wmiMake = name
			}
		}
	}

	// Step 2: Find VinSchemaIds for this WMI + year
	schemaIDs, err := d.findSchemas(wmiID, year)
	if err != nil {
		return nil, err
	}
	if len(schemaIDs) == 0 {
		return nil, nil
	}

	// Step 3: Get patterns for matching schemas and decode
	result := &DecodeResult{ModelYear: year, Make: wmiMake}
	err = d.matchPatterns(vin, schemaIDs, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (d *Decoder) findSchemas(wmiID, year int) ([]int, error) {
	rows, err := d.db.Query(`
		SELECT DISTINCT VinSchemaId
		FROM Wmi_VinSchema
		WHERE WmiId = ?
		AND (YearFrom IS NULL OR YearFrom <= ?)
		AND (YearTo IS NULL OR YearTo >= ?)
	`, wmiID, year, year)
	if err != nil {
		return nil, fmt.Errorf("nhtsa: find schemas: %w", err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (d *Decoder) matchPatterns(vin string, schemaIDs []int, result *DecodeResult) error {
	// Build placeholders for IN clause
	placeholders := make([]string, len(schemaIDs))
	args := make([]interface{}, len(schemaIDs))
	for i, id := range schemaIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT p.Keys, p.ElementId, p.AttributeId, e.LookupTable
		FROM Pattern p
		JOIN Element e ON p.ElementId = e.Id
		WHERE p.VinSchemaId IN (%s)
		AND e.Code IN ('Make','Model','BodyClass','DriveType','FuelTypePrimary',
			'PlantCountry','Trim','Series','VehicleType',
			'DisplacementCC','DisplacementL','EngineCylinders')
	`, strings.Join(placeholders, ","))

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("nhtsa: query patterns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var keys string
		var elemID int
		var attrID string
		var lookupTable sql.NullString
		if err := rows.Scan(&keys, &elemID, &attrID, &lookupTable); err != nil {
			return err
		}

		if !matchKeys(vin, keys) {
			continue
		}

		code := d.elemCodes[elemID]
		value := d.resolveValue(attrID, lookupTable.String)
		d.applyField(code, value, result)
	}
	return rows.Err()
}

// matchKeys checks if VIN positions 4-N match the pattern Keys.
// Keys can contain [ABC] character classes and * wildcards.
func matchKeys(vin string, keys string) bool {
	// Keys match against VIN positions 4 onward (0-indexed: vin[3:])
	vds := vin[3:]

	// Convert Keys pattern to regex
	var b strings.Builder
	b.WriteByte('^')
	i := 0
	for i < len(keys) {
		ch := keys[i]
		if ch == '[' {
			// Copy character class as-is
			end := strings.IndexByte(keys[i:], ']')
			if end < 0 {
				return false
			}
			b.WriteString(keys[i : i+end+1])
			i += end + 1
		} else if ch == '*' {
			b.WriteString("[A-HJ-NPR-Z0-9]")
			i++
		} else {
			b.WriteByte(ch)
			i++
		}
	}
	b.WriteByte('$')

	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}

	// Effective length of the pattern (count actual character positions)
	effLen := patternLen(keys)
	if effLen > len(vds) {
		return false
	}

	segment := vds[:effLen]
	return re.MatchString(segment)
}

// patternLen returns the number of character positions a Keys pattern matches.
func patternLen(keys string) int {
	n := 0
	i := 0
	for i < len(keys) {
		if keys[i] == '[' {
			end := strings.IndexByte(keys[i:], ']')
			if end < 0 {
				n++
				i++
			} else {
				n++
				i += end + 1
			}
		} else {
			n++
			i++
		}
	}
	return n
}

func (d *Decoder) resolveValue(attrID, lookupTable string) string {
	if lookupTable == "" {
		return attrID
	}
	id, err := strconv.Atoi(attrID)
	if err != nil {
		return attrID
	}
	if cache, ok := d.lookupCache[lookupTable]; ok {
		if name, ok := cache[id]; ok {
			return name
		}
	}
	return attrID
}

func (d *Decoder) applyField(code, value string, r *DecodeResult) {
	switch code {
	case "Make":
		if r.Make == "" {
			r.Make = value
		}
	case "Model":
		if r.Model == "" {
			r.Model = value
		}
	case "BodyClass":
		if r.BodyClass == "" {
			r.BodyClass = value
		}
	case "DriveType":
		if r.DriveType == "" {
			r.DriveType = value
		}
	case "FuelTypePrimary":
		if r.FuelType == "" {
			r.FuelType = value
		}
	case "PlantCountry":
		if r.PlantCountry == "" {
			r.PlantCountry = value
		}
	case "Trim":
		if r.Trim == "" {
			r.Trim = value
		}
	case "Series":
		if r.Series == "" {
			r.Series = value
		}
	case "VehicleType":
		if r.VehicleType == "" {
			r.VehicleType = value
		}
	case "DisplacementCC":
		if r.EngineCC == "" {
			r.EngineCC = value
		}
	case "DisplacementL":
		if r.EngineCC == "" {
			// Convert liters to CC
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				r.EngineCC = strconv.Itoa(int(f * 1000))
			}
		}
	case "EngineCylinders":
		if r.EngineCyl == "" {
			r.EngineCyl = value
		}
	}
}

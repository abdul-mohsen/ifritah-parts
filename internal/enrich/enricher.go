package enrich

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Enricher adds extra vehicle data (MPG, transmission, trim, etc.) after VIN decode.
type Enricher struct {
	epa      *sql.DB
	ovdb     map[string][]ovdbModel      // make → models
	arthur   map[string]map[string][]int // make → model → years
	carquery *http.Client
	fuelAPI  *http.Client
}

type ovdbModel struct {
	Name        string `json:"model_name"`
	VehicleType string `json:"vehicle_type"`
	Years       []int  `json:"years"`
}

// New creates an Enricher with all available local databases and API clients.
// Any source that fails to load is skipped (non-fatal).
func New(dataDir string) *Enricher {
	e := &Enricher{
		carquery: &http.Client{Timeout: 5 * time.Second},
		fuelAPI:  &http.Client{Timeout: 5 * time.Second},
	}

	// 1. EPA FuelEconomy SQLite
	epaPath := dataDir + "/epa_vehicles.db"
	if _, err := os.Stat(epaPath); err == nil {
		db, err := sql.Open("sqlite", epaPath)
		if err == nil {
			e.epa = db
			log.Printf("✓ EPA FuelEconomy DB loaded (%s)", epaPath)
		} else {
			log.Printf("⚠ EPA DB open error: %v", err)
		}
	}

	// 2. open-vehicle-db
	ovdbPath := dataDir + "/open-vehicle-db/makes_and_models.json"
	if data, err := os.ReadFile(ovdbPath); err == nil {
		var makes []struct {
			MakeName string               `json:"make_name"`
			Models   map[string]ovdbModel `json:"models"`
		}
		if err := json.Unmarshal(data, &makes); err == nil {
			e.ovdb = make(map[string][]ovdbModel, len(makes))
			for _, m := range makes {
				key := strings.ToUpper(m.MakeName)
				models := make([]ovdbModel, 0, len(m.Models))
				for _, mdl := range m.Models {
					models = append(models, mdl)
				}
				e.ovdb[key] = models
			}
			log.Printf("✓ OpenVehicleDB loaded (%d makes)", len(e.ovdb))
		}
	}

	// 3. arthurkao vehicle-make-model-data
	arthurPath := dataDir + "/vehicle_make_model.json"
	if data, err := os.ReadFile(arthurPath); err == nil {
		var entries []struct {
			Year  int    `json:"year"`
			Make  string `json:"make"`
			Model string `json:"model"`
		}
		if err := json.Unmarshal(data, &entries); err == nil {
			e.arthur = make(map[string]map[string][]int)
			for _, ent := range entries {
				mk := strings.ToUpper(ent.Make)
				if e.arthur[mk] == nil {
					e.arthur[mk] = make(map[string][]int)
				}
				e.arthur[mk][strings.ToUpper(ent.Model)] = append(
					e.arthur[mk][strings.ToUpper(ent.Model)], ent.Year)
			}
			log.Printf("✓ Arthurkao vehicle data loaded (%d makes)", len(e.arthur))
		}
	}

	return e
}

// Close releases resources.
func (e *Enricher) Close() {
	if e.epa != nil {
		e.epa.Close()
	}
}

// EnrichResult holds enrichment data from all sources.
type EnrichResult struct {
	Transmission string
	VehicleClass string
	CityMPG      int
	HighwayMPG   int
	CombinedMPG  int
	CO2Gpm       float64
	EngineDescr  string
	VehicleType  string
	Trim         string
	Sources      []string
}

// Enrich queries all data sources in priority order:
// 1. EPA SQLite (local) → MPG, transmission, vehicle class, engine
// 2. OpenVehicleDB (local) → vehicle type
// 3. Arthurkao (local) → model validation
// 4. CarQuery API (online) → trim, specs
// 5. FuelEconomy.gov API (online) → MPG
func (e *Enricher) Enrich(mk, model string, year int) *EnrichResult {
	r := &EnrichResult{}

	// 1. EPA FuelEconomy (local SQLite)
	if e.epa != nil {
		e.enrichFromEPA(r, mk, model, year)
	}

	// 2. OpenVehicleDB (local JSON)
	if e.ovdb != nil {
		e.enrichFromOVDB(r, mk, model, year)
	}

	// 3. Arthurkao (local JSON) — validates make+model+year
	if e.arthur != nil {
		e.enrichFromArthur(r, mk, model, year)
	}

	// 4. CarQuery API (online) — only if we're still missing data
	if r.Trim == "" {
		e.enrichFromCarQuery(r, mk, model, year)
	}

	// 5. FuelEconomy.gov API (online) — only if we're still missing MPG
	if r.CombinedMPG == 0 {
		e.enrichFromFuelAPI(r, mk, model, year)
	}

	if len(r.Sources) == 0 {
		return nil
	}
	return r
}

func (e *Enricher) enrichFromEPA(r *EnrichResult, mk, model string, year int) {
	makeUpper := strings.ToUpper(mk)

	// Try exact match first, then fuzzy model match
	row := e.epa.QueryRow(`
		SELECT trany, vclass, city_mpg, highway_mpg, comb_mpg, co2_gpm, eng_descr, drive, fuel_type
		FROM vehicles
		WHERE make = ? AND model = ? AND year = ?
		LIMIT 1`, makeUpper, model, year)

	var trany, vclass, engDescr, drive, fuelType sql.NullString
	var city, hwy, comb sql.NullInt64
	var co2 sql.NullFloat64
	err := row.Scan(&trany, &vclass, &city, &hwy, &comb, &co2, &engDescr, &drive, &fuelType)
	if err != nil {
		// Try case-insensitive model match
		row = e.epa.QueryRow(`
			SELECT trany, vclass, city_mpg, highway_mpg, comb_mpg, co2_gpm, eng_descr, drive, fuel_type
			FROM vehicles
			WHERE make = ? AND UPPER(model) = UPPER(?) AND year = ?
			LIMIT 1`, makeUpper, model, year)
		err = row.Scan(&trany, &vclass, &city, &hwy, &comb, &co2, &engDescr, &drive, &fuelType)
	}
	if err != nil {
		// Try partial model match (e.g., "Camry" matches "Camry XSE")
		row = e.epa.QueryRow(`
			SELECT trany, vclass, city_mpg, highway_mpg, comb_mpg, co2_gpm, eng_descr, drive, fuel_type
			FROM vehicles
			WHERE make = ? AND UPPER(model) LIKE '%' || UPPER(?) || '%' AND year = ?
			ORDER BY model
			LIMIT 1`, makeUpper, model, year)
		err = row.Scan(&trany, &vclass, &city, &hwy, &comb, &co2, &engDescr, &drive, &fuelType)
	}
	if err != nil {
		return
	}

	if trany.Valid && r.Transmission == "" {
		r.Transmission = trany.String
	}
	if vclass.Valid && r.VehicleClass == "" {
		r.VehicleClass = vclass.String
	}
	if city.Valid && r.CityMPG == 0 {
		r.CityMPG = int(city.Int64)
	}
	if hwy.Valid && r.HighwayMPG == 0 {
		r.HighwayMPG = int(hwy.Int64)
	}
	if comb.Valid && r.CombinedMPG == 0 {
		r.CombinedMPG = int(comb.Int64)
	}
	if co2.Valid && r.CO2Gpm == 0 {
		r.CO2Gpm = co2.Float64
	}
	if engDescr.Valid && r.EngineDescr == "" {
		r.EngineDescr = engDescr.String
	}
	r.Sources = append(r.Sources, "epa_fueleconomy")
}

func (e *Enricher) enrichFromOVDB(r *EnrichResult, mk, model string, year int) {
	makeUpper := strings.ToUpper(mk)
	models, ok := e.ovdb[makeUpper]
	if !ok {
		return
	}

	modelUpper := strings.ToUpper(model)
	for _, m := range models {
		if strings.ToUpper(m.Name) == modelUpper || strings.Contains(modelUpper, strings.ToUpper(m.Name)) {
			// Verify year exists
			for _, y := range m.Years {
				if y == year {
					if r.VehicleType == "" && m.VehicleType != "" {
						r.VehicleType = m.VehicleType
					}
					r.Sources = append(r.Sources, "open_vehicle_db")
					return
				}
			}
		}
	}
}

func (e *Enricher) enrichFromArthur(r *EnrichResult, mk, model string, year int) {
	makeUpper := strings.ToUpper(mk)
	modelMap, ok := e.arthur[makeUpper]
	if !ok {
		return
	}

	modelUpper := strings.ToUpper(model)
	if years, ok := modelMap[modelUpper]; ok {
		for _, y := range years {
			if y == year {
				r.Sources = append(r.Sources, "arthurkao_vehicle_db")
				return
			}
		}
	}
}

// CarQuery API (online fallback)
func (e *Enricher) enrichFromCarQuery(r *EnrichResult, mk, model string, year int) {
	u := fmt.Sprintf("https://www.carqueryapi.com/api/0.3/?callback=?&cmd=getTrims&make=%s&model=%s&year=%d",
		url.QueryEscape(strings.ToLower(mk)),
		url.QueryEscape(strings.ToLower(model)),
		year)

	resp, err := e.carquery.Get(u)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	// CarQuery wraps response in JSONP callback: ?({...})
	buf := make([]byte, 32*1024)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	// Strip JSONP wrapper
	if idx := strings.Index(body, "("); idx >= 0 {
		body = body[idx+1:]
	}
	if idx := strings.LastIndex(body, ")"); idx >= 0 {
		body = body[:idx]
	}

	var data struct {
		Trims []struct {
			Trim  string `json:"model_trim"`
			Body  string `json:"model_body"`
			Drive string `json:"model_drive"`
			Seats string `json:"model_seats"`
			Doors string `json:"model_doors"`
			HP    string `json:"model_engine_power_ps"`
			Fuel  string `json:"model_engine_fuel"`
			CC    string `json:"model_engine_cc"`
		} `json:"Trims"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil || len(data.Trims) == 0 {
		return
	}

	t := data.Trims[0]
	if t.Trim != "" && r.Trim == "" {
		r.Trim = t.Trim
	}
	if t.Body != "" && r.VehicleType == "" {
		r.VehicleType = t.Body
	}
	r.Sources = append(r.Sources, "carquery_api")
}

// FuelEconomy.gov REST API (online fallback for MPG)
func (e *Enricher) enrichFromFuelAPI(r *EnrichResult, mk, model string, year int) {
	// Step 1: Get vehicle options for make+model+year
	u := fmt.Sprintf("https://www.fueleconomy.gov/ws/rest/vehicle/menu/options?year=%d&make=%s&model=%s",
		year,
		url.QueryEscape(mk),
		url.QueryEscape(model))

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/json")

	resp, err := e.fuelAPI.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var optResult struct {
		MenuItem []struct {
			Value string `json:"value"`
		} `json:"menuItem"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&optResult); err != nil || len(optResult.MenuItem) == 0 {
		return
	}

	// Step 2: Get vehicle data for first option
	vid := optResult.MenuItem[0].Value
	u2 := fmt.Sprintf("https://www.fueleconomy.gov/ws/rest/vehicle/%s", vid)
	req2, err := http.NewRequest("GET", u2, nil)
	if err != nil {
		return
	}
	req2.Header.Set("Accept", "application/json")

	resp2, err := e.fuelAPI.Do(req2)
	if err != nil {
		return
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		return
	}

	var veh struct {
		City08    int     `json:"city08"`
		Highway08 int     `json:"highway08"`
		Comb08    int     `json:"comb08"`
		CO2       float64 `json:"co2TailpipeGpm"`
		Trany     string  `json:"trany"`
		VClass    string  `json:"VClass"`
		Displ     string  `json:"displ"`
		Cylinders string  `json:"cylinders"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&veh); err != nil {
		return
	}

	if veh.Comb08 > 0 && r.CombinedMPG == 0 {
		r.CityMPG = veh.City08
		r.HighwayMPG = veh.Highway08
		r.CombinedMPG = veh.Comb08
	}
	if veh.CO2 > 0 && r.CO2Gpm == 0 {
		r.CO2Gpm = veh.CO2
	}
	if veh.Trany != "" && r.Transmission == "" {
		r.Transmission = veh.Trany
	}
	if veh.VClass != "" && r.VehicleClass == "" {
		r.VehicleClass = veh.VClass
	}
	r.Sources = append(r.Sources, "fueleconomy_api")
}

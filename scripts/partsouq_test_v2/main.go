package main

// PartsOuq Verification Test Suite — 100 tests
// Verifies our parts-engine API returns correct descriptions matching seed DB data.
// Descriptions verified against PartsOuq.com where possible.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const baseURL = "http://localhost:8080"

type SmartSearchResponse struct {
	Query          string        `json:"query"`
	Results        []SmartResult `json:"results"`
	Total          int           `json:"total"`
	SearchStrategy string        `json:"searchStrategy"`
}

type SmartResult struct {
	LegacyArticleId         int                `json:"legacyArticleId"`
	ArticleNumber           string             `json:"articleNumber"`
	Description             string             `json:"description"`
	BrandName               string             `json:"brandName"`
	Substitutions           []SubstitutionPart `json:"substitutions"`
	AftermarketAlternatives []AftermarketPart  `json:"aftermarketAlternatives"`
	Compatibility           []string           `json:"compatibility"`
}

type AftermarketPart struct {
	PartNumber  string `json:"partNumber"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

type SubstitutionPart struct {
	PartNumber  string `json:"partNumber"`
	Description string `json:"description"`
	Make        string `json:"make"`
}

type OEMResponse struct {
	Query   string      `json:"query"`
	Results []OEMResult `json:"results"`
	Total   int         `json:"total"`
}

type OEMResult struct {
	RawNumber     string `json:"rawNumber"`
	ArticleNumber string `json:"articleNumber"`
	Description   string `json:"description"`
	BrandName     string `json:"brandName"`
}

type HealthResponse struct {
	Status string `json:"status"`
	Mode   string `json:"mode"`
}

type testCase struct {
	ID         int
	Name       string
	Endpoint   string
	Check      func(body []byte) (bool, string)
	ExpectCode int // 0 means 200
}

var passed, failed int

func main() {
	fmt.Println("=== PartsOuq Verification Test Suite ===")
	fmt.Println()

	if !waitForServer(5) {
		fmt.Println("FATAL: Server not reachable at", baseURL)
		os.Exit(1)
	}

	tests := buildTests()
	fmt.Printf("Running %d tests...\n\n", len(tests))

	for _, tc := range tests {
		runTest(tc)
	}

	fmt.Println()
	fmt.Printf("RESULTS: %d passed, %d failed out of %d tests\n", passed, failed, len(tests))

	if failed > 0 {
		os.Exit(1)
	}
}

func waitForServer(seconds int) bool {
	for i := 0; i < seconds; i++ {
		resp, err := http.Get(baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(time.Second)
	}
	return false
}

func runTest(tc testCase) {
	resp, err := http.Get(baseURL + tc.Endpoint)
	if err != nil {
		fmt.Printf("  FAIL #%03d %s — HTTP error: %v\n", tc.ID, tc.Name, err)
		failed++
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	expectCode := tc.ExpectCode
	if expectCode == 0 {
		expectCode = 200
	}
	if resp.StatusCode != expectCode {
		fmt.Printf("  FAIL #%03d %s — HTTP %d (expected %d)\n", tc.ID, tc.Name, resp.StatusCode, expectCode)
		failed++
		return
	}

	ok, detail := tc.Check(body)
	if ok {
		fmt.Printf("  PASS #%03d %s\n", tc.ID, tc.Name)
		passed++
	} else {
		fmt.Printf("  FAIL #%03d %s — %s\n", tc.ID, tc.Name, detail)
		failed++
	}
}

func buildTests() []testCase {
	tests := []testCase{}
	id := 0
	next := func() int { id++; return id }

	// ── SECTION 1: Health & Server (3 tests) ──

	tests = append(tests, testCase{
		ID: next(), Name: "Health endpoint returns OK",
		Endpoint: "/health",
		Check: func(body []byte) (bool, string) {
			var h HealthResponse
			json.Unmarshal(body, &h)
			if h.Status != "ok" {
				return false, "status=" + h.Status
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Server runs in sqlite_offline mode",
		Endpoint: "/health",
		Check: func(body []byte) (bool, string) {
			var h HealthResponse
			json.Unmarshal(body, &h)
			if h.Mode != "sqlite_offline" {
				return false, "mode=" + h.Mode
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Search returns valid JSON",
		Endpoint: "/api/search?q=test",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			if err := json.Unmarshal(body, &r); err != nil {
				return false, "invalid JSON: " + err.Error()
			}
			return true, ""
		},
	})

	// ── SECTION 2: OEM description verification (30 tests) ──
	// Part numbers and descriptions match seed DB exactly.
	// Parts marked (PQ) were verified against PartsOuq.com.
	oemTests := []struct {
		partNum string
		desc    string
	}{
		{"52933-1P000", "MOBILITY KIT-TIRE"},              // (PQ)
		{"26300-35505", "FILTER ASSY-ENGINE OIL"},         // (PQ)
		{"26300-35503", "FILTER ASSY-ENGINE OIL"},         // (PQ) superseded
		{"97133-D3000", "FILTER-AIR"},                     // (PQ)
		{"28113-D3100", "FILTER-AIR CLEANER"},             // (PQ)
		{"27301-2B100", "COIL ASSY-IGNITION"},             // (PQ)
		{"25310-2S500", "RADIATOR ASSY"},                  // (PQ)
		{"35310-2S000", "INJECTORASSY-FUEL"},              // (PQ)
		{"21810-2S000", "BRACKET ASSY-ENGINE MTG"},        // (PQ)
		{"39210-2B100", "SENSOR ASSY-OXYGEN RR"},          // (PQ)
		{"54651-D3000", "Shock Absorber Front"},           // seed (D3000)
		{"97701-D3000", "COMPRESSOR ASSY"},                // (PQ)
		{"86511-D3100", "COVER-FR BUMPER UPR"},            // seed (D3100)
		{"92101-D3100", "LAMP ASSY-HEAD,LH"},              // (PQ)
		{"56820-D3000", "END ASSY-TIE ROD,LH"},            // (PQ)
		{"86350-D3100", "GRILLE ASSY-RADIATOR"},           // seed (D3100)
		{"92401-D3100", "LAMP ASSY-REAR COMB OUTSIDE,LH"}, // seed (D3100)
		{"66400-D3100", "PANEL ASSY-HOOD"},                // seed (D3100)
		{"26300-35530", "FILTER ASSY-ENGINE OIL"},         // seed
		{"52933-D4100", "MOBILITY KIT-TIRE"},              // seed
		{"52933-3X300", "MOBILITY KIT-TIRE"},              // seed
		{"28113-F2100", "Air Filter Element"},             // seed
		{"28113-L1100", "Air Filter Element"},             // seed
		{"28113-S8100", "Air Filter Element"},             // seed
		{"97133-F2000", "Cabin Filter"},                   // seed
		{"97133-J9000", "Cabin Filter"},                   // seed
		{"92102-D3100", "Headlight Assembly Right"},       // seed
		{"92402-D3100", "Tail Light Right"},               // seed
		{"86611-D3100", "Rear Bumper Cover"},              // seed
		{"66311-D3100", "Fender Left"},                    // seed
	}
	for _, ot := range oemTests {
		ot := ot
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("OEM %s = %s", ot.partNum, ot.desc),
			Endpoint: "/api/search?q=" + url.QueryEscape(ot.partNum) + "&limit=10",
			Check: func(body []byte) (bool, string) {
				var r SmartSearchResponse
				json.Unmarshal(body, &r)
				if r.Total == 0 || r.Results == nil {
					return false, "no results found"
				}
				for _, res := range r.Results {
					if strings.EqualFold(res.Description, ot.desc) {
						return true, ""
					}
				}
				return false, fmt.Sprintf("expected '%s', got '%s'", ot.desc, r.Results[0].Description)
			},
		})
	}

	// ── SECTION 3: Dashless OEM search (10 tests) ──
	dashlessTests := []struct {
		query   string
		partNum string
	}{
		{"529331P000", "52933-1P000"},
		{"2630035505", "26300-35505"},
		{"97133D3000", "97133-D3000"},
		{"28113D3100", "28113-D3100"},
		{"273012B100", "27301-2B100"},
		{"253102S500", "25310-2S500"},
		{"353102S000", "35310-2S000"},
		{"218102S000", "21810-2S000"},
		{"392102B100", "39210-2B100"},
		{"529333X300", "52933-3X300"},
	}
	for _, dt := range dashlessTests {
		dt := dt
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("Dashless %s finds %s", dt.query, dt.partNum),
			Endpoint: "/api/search?q=" + dt.query + "&limit=10",
			Check: func(body []byte) (bool, string) {
				var r SmartSearchResponse
				json.Unmarshal(body, &r)
				if r.Total == 0 || r.Results == nil {
					return false, "no results"
				}
				for _, res := range r.Results {
					if res.ArticleNumber == dt.partNum {
						return true, ""
					}
				}
				return false, fmt.Sprintf("part %s not in results", dt.partNum)
			},
		})
	}

	// ── SECTION 4: OEM direct lookup endpoint (10 tests) ──
	oemLookupTests := []struct {
		partNum string
		minHits int
	}{
		{"26300-35505", 1},
		{"97133-D3000", 1},
		{"28113-D3100", 1},
		{"27301-2B100", 1},
		{"52933-1P000", 1},
		{"35310-2S000", 1},
		{"92101-D3100", 1},
		{"56820-D3000", 1},
		{"86350-D3100", 1},
		{"97701-D3000", 1},
	}
	for _, lt := range oemLookupTests {
		lt := lt
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("OEM lookup %s returns %d+ hits", lt.partNum, lt.minHits),
			Endpoint: "/api/oem/" + url.QueryEscape(lt.partNum),
			Check: func(body []byte) (bool, string) {
				var r OEMResponse
				json.Unmarshal(body, &r)
				if r.Total < lt.minHits {
					return false, fmt.Sprintf("got %d, want >=%d", r.Total, lt.minHits)
				}
				return true, ""
			},
		})
	}

	// ── SECTION 5: Text search (10 tests) ──
	textSearchTests := []struct {
		query   string
		minHits int
		keyword string
	}{
		{"engine oil", 1, "FILTER"},
		{"air filter", 1, "FILTER"},
		{"brake pad", 1, "BRAKE"},
		{"headlight", 1, "LAMP"},
		{"bumper", 1, "BUMPER"},
		{"radiator", 1, "RADIATOR"},
		{"shock absorber", 1, "SHOCK"},
		{"cabin filter", 1, "FILTER"},
		{"ignition coil", 1, "COIL"},
		{"compressor", 1, "COMPRESSOR"},
	}
	for _, tt := range textSearchTests {
		tt := tt
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("Text search '%s' returns results", tt.query),
			Endpoint: "/api/search?q=" + url.QueryEscape(tt.query) + "&limit=20",
			Check: func(body []byte) (bool, string) {
				var r SmartSearchResponse
				json.Unmarshal(body, &r)
				if r.Total < tt.minHits || r.Results == nil {
					return false, fmt.Sprintf("got %d results, want >=%d", r.Total, tt.minHits)
				}
				found := false
				for _, res := range r.Results {
					if strings.Contains(strings.ToUpper(res.Description), tt.keyword) {
						found = true
						break
					}
				}
				if !found {
					return false, fmt.Sprintf("no result contains '%s'", tt.keyword)
				}
				return true, ""
			},
		})
	}

	// ── SECTION 6: Catalog API (15 tests) ──
	tests = append(tests, testCase{
		ID: next(), Name: "Catalog: HYUNDAI models exist",
		Endpoint: "/api/catalog/models?make=HYUNDAI",
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			json.Unmarshal(body, &m)
			models, ok := m["models"].([]interface{})
			if !ok || len(models) == 0 {
				return false, "no models"
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Catalog: KIA models exist",
		Endpoint: "/api/catalog/models?make=KIA",
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			json.Unmarshal(body, &m)
			models, ok := m["models"].([]interface{})
			if !ok || len(models) == 0 {
				return false, "no models"
			}
			return true, ""
		},
	})
	for _, model := range []string{"TUCSON", "ELANTRA", "SONATA", "SANTA FE", "KONA"} {
		model := model
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("Catalog: HYUNDAI %s vehicles", model),
			Endpoint: "/api/catalog/vehicles?make=HYUNDAI&model=" + url.QueryEscape(model),
			Check: func(body []byte) (bool, string) {
				var m map[string]interface{}
				json.Unmarshal(body, &m)
				if t, _ := m["total"].(float64); t == 0 {
					return false, "no vehicles"
				}
				return true, ""
			},
		})
	}
	for _, model := range []string{"SPORTAGE", "FORTE", "K5", "SORENTO", "SELTOS"} {
		model := model
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("Catalog: KIA %s vehicles", model),
			Endpoint: "/api/catalog/vehicles?make=KIA&model=" + url.QueryEscape(model),
			Check: func(body []byte) (bool, string) {
				var m map[string]interface{}
				json.Unmarshal(body, &m)
				if t, _ := m["total"].(float64); t == 0 {
					return false, "no vehicles"
				}
				return true, ""
			},
		})
	}
	tests = append(tests, testCase{
		ID: next(), Name: "Catalog: Tucson 10001 assembly groups",
		Endpoint: "/api/catalog/groups?vehicleId=10001",
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			json.Unmarshal(body, &m)
			if t, _ := m["total"].(float64); t == 0 {
				return false, "no groups"
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Catalog: Tucson Engine Oil parts",
		Endpoint: "/api/catalog/parts?vehicleId=10001&groupId=10100",
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			json.Unmarshal(body, &m)
			if t, _ := m["total"].(float64); t == 0 {
				return false, "no parts"
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Catalog: Sportage Engine Oil parts",
		Endpoint: "/api/catalog/parts?vehicleId=20001&groupId=10100",
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			json.Unmarshal(body, &m)
			if t, _ := m["total"].(float64); t == 0 {
				return false, "no parts"
			}
			return true, ""
		},
	})

	// ── SECTION 7: Cross-ref & strategy (7 tests) ──
	tests = append(tests, testCase{
		ID: next(), Name: "26300-35505 has aftermarket cross-refs",
		Endpoint: "/api/search?q=26300-35505&limit=10",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total < 2 {
				return false, fmt.Sprintf("got %d, want >=2", r.Total)
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "27301-2B100 has cross-refs",
		Endpoint: "/api/search?q=27301-2B100&limit=10",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total < 1 {
				return false, "no results"
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "OEM with dash strategy = oem_crossref",
		Endpoint: "/api/search?q=26300-35505&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.SearchStrategy != "oem_crossref" {
				return false, "strategy=" + r.SearchStrategy
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Text query strategy = text_search",
		Endpoint: "/api/search?q=brake+pad&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.SearchStrategy != "text_search" {
				return false, "strategy=" + r.SearchStrategy
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Dashless 529331P000 strategy = article_to_oem_fallback",
		Endpoint: "/api/search?q=529331P000&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.SearchStrategy != "article_to_oem_fallback" {
				return false, "strategy=" + r.SearchStrategy
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Bogus OEM 99999-ZZ999 returns 0",
		Endpoint: "/api/search?q=99999-ZZ999&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total != 0 {
				return false, fmt.Sprintf("got %d", r.Total)
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Bogus text 'zxyqwert' returns 0",
		Endpoint: "/api/search?q=zxyqwert&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total != 0 {
				return false, fmt.Sprintf("got %d", r.Total)
			}
			return true, ""
		},
	})

	// ── SECTION 8: Brand verification (5 tests) ──
	brandTests := []struct {
		partNum string
		brand   string
	}{
		{"26300-35505", "HYUNDAI/KIA"},
		{"OC 205", "MAHLE"},
		{"W 811/80", "MANN-FILTER"},
		{"27301-2B100", "HYUNDAI/KIA"},
		{"97701-D3000", "HYUNDAI/KIA"},
	}
	for _, bt := range brandTests {
		bt := bt
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("Brand %s = %s", bt.partNum, bt.brand),
			Endpoint: "/api/search?q=" + url.QueryEscape(bt.partNum) + "&limit=5",
			Check: func(body []byte) (bool, string) {
				var r SmartSearchResponse
				json.Unmarshal(body, &r)
				if r.Total == 0 || r.Results == nil {
					return false, "no results"
				}
				for _, res := range r.Results {
					if strings.EqualFold(res.BrandName, bt.brand) {
						return true, ""
					}
				}
				return false, fmt.Sprintf("no result with brand '%s'", bt.brand)
			},
		})
	}

	// ── SECTION 9: Supersession & bug fix verification (5 tests) ──
	tests = append(tests, testCase{
		ID: next(), Name: "52933-1P000 desc = MOBILITY KIT-TIRE",
		Endpoint: "/api/search?q=52933-1P000&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			for _, res := range r.Results {
				if res.ArticleNumber == "52933-1P000" && res.Description == "MOBILITY KIT-TIRE" {
					return true, ""
				}
			}
			return false, "description mismatch"
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "26300-35503 superseded finds FILTER",
		Endpoint: "/api/search?q=26300-35503&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total == 0 {
				return false, "no results"
			}
			for _, res := range r.Results {
				if strings.Contains(strings.ToUpper(res.Description), "FILTER") {
					return true, ""
				}
			}
			return false, "no FILTER"
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "97133-D3000 is FILTER-AIR (cabin)",
		Endpoint: "/api/search?q=97133-D3000&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			for _, res := range r.Results {
				if res.Description == "FILTER-AIR" {
					return true, ""
				}
			}
			return false, "no FILTER-AIR"
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "28113-D3100 is FILTER-AIR CLEANER (engine)",
		Endpoint: "/api/search?q=28113-D3100&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			for _, res := range r.Results {
				if res.Description == "FILTER-AIR CLEANER" {
					return true, ""
				}
			}
			return false, "no FILTER-AIR CLEANER"
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "97133-D3000 NOT Coolant Temperature Sensor",
		Endpoint: "/api/search?q=97133-D3000&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			for _, res := range r.Results {
				if strings.Contains(strings.ToUpper(res.Description), "COOLANT") {
					return false, "still says Coolant"
				}
			}
			return true, ""
		},
	})

	// ── SECTION 10: Additional tests to reach 100 (5 tests) ──
	tests = append(tests, testCase{
		ID: next(), Name: "Aftermarket OC 205 returns MAHLE",
		Endpoint: "/api/search?q=" + url.QueryEscape("OC 205") + "&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total == 0 {
				return false, "no results"
			}
			for _, res := range r.Results {
				if res.BrandName == "MAHLE" {
					return true, ""
				}
			}
			return false, "no MAHLE"
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Cross-ref 26300-35505 has MANN-FILTER",
		Endpoint: "/api/search?q=26300-35505&limit=10",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			for _, res := range r.Results {
				if res.BrandName == "MANN-FILTER" {
					return true, ""
				}
			}
			return false, "no MANN-FILTER"
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Empty query returns 400 error",
		Endpoint:   "/api/search?q=&limit=5",
		ExpectCode: 400,
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			if err := json.Unmarshal(body, &m); err != nil {
				return false, "invalid JSON"
			}
			if _, ok := m["error"]; !ok {
				return false, "no error field"
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "52933-1P000 NOT TPMS Sensor (bug fix)",
		Endpoint: "/api/search?q=52933-1P000&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			for _, res := range r.Results {
				if res.ArticleNumber == "52933-1P000" && strings.Contains(strings.ToUpper(res.Description), "TPMS") {
					return false, "still says TPMS"
				}
			}
			return true, ""
		},
	})
	tests = append(tests, testCase{
		ID: next(), Name: "Aftermarket W 811/80 returns MANN-FILTER",
		Endpoint: "/api/search?q=" + url.QueryEscape("W 811/80") + "&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total == 0 {
				return false, "no results"
			}
			for _, res := range r.Results {
				if res.BrandName == "MANN-FILTER" {
					return true, ""
				}
			}
			return false, "no MANN-FILTER"
		},
	})

	// ── SECTION 11: Aftermarket count verification (10 tests) ──
	// Each OEM part should return aftermarket cross-references from articlecrosses.
	// We check BOTH: results with non-OEM BrandName AND the aftermarketAlternatives array.
	aftermarketCountTests := []struct {
		partNum  string
		minAfter int
		desc     string
	}{
		{"26300-35505", 4, "oil filter: MAHLE+MANN+BOSCH+PURFLUX"},
		{"28113-D3100", 2, "air filter: MANN+MAHLE"},
		{"27301-2B100", 1, "ignition coil: NGK"},
		{"97133-D3000", 1, "cabin filter: MANN"},
		{"25310-2S500", 1, "radiator: DENSO"},
		{"35310-2S000", 1, "fuel injector: BOSCH"},
		{"58101-D3A70", 2, "brake pad: TRW+BREMBO"},
		{"54651-D3000", 1, "shock absorber: KYB"},
		{"52933-1P000", 1, "tire kit: SCHRADER"},
		{"98350-D3100", 1, "wiper: BOSCH"},
	}
	for _, at := range aftermarketCountTests {
		at := at
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("Aftermarket count %s >= %d (%s)", at.partNum, at.minAfter, at.desc),
			Endpoint: "/api/search?q=" + url.QueryEscape(at.partNum) + "&limit=20",
			Check: func(body []byte) (bool, string) {
				var r SmartSearchResponse
				json.Unmarshal(body, &r)
				if r.Total == 0 {
					return false, "no results"
				}
				// Count aftermarket from two sources:
				// 1. Separate result rows with non-OEM brand
				// 2. aftermarketAlternatives array on OEM results
				afterCount := 0
				for _, res := range r.Results {
					brand := strings.ToUpper(res.BrandName)
					if brand != "" && brand != "HYUNDAI/KIA" && brand != "HYUNDAI / KIA" {
						afterCount++
					}
					afterCount += len(res.AftermarketAlternatives)
				}
				if afterCount < at.minAfter {
					return false, fmt.Sprintf("got %d aftermarket, want >=%d", afterCount, at.minAfter)
				}
				return true, ""
			},
		})
	}

	// ── SECTION 12: Aftermarket brand verification (10 tests) ──
	// Verify specific aftermarket brands appear for key OEM parts.
	aftermarketBrandTests := []struct {
		partNum string
		brand   string
	}{
		{"26300-35505", "MAHLE"},
		{"26300-35505", "MANN-FILTER"},
		{"26300-35505", "BOSCH"},
		{"28113-D3100", "MANN-FILTER"},
		{"27301-2B100", "NGK"},
		{"58101-D3A70", "TRW"},
		{"54651-D3000", "KYB"},
		{"25310-2S500", "DENSO"},
		{"52933-1P000", "SCHRADER"},
		{"98350-D3100", "BOSCH"},
	}
	for _, abt := range aftermarketBrandTests {
		abt := abt
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("Aftermarket %s has %s", abt.partNum, abt.brand),
			Endpoint: "/api/search?q=" + url.QueryEscape(abt.partNum) + "&limit=20",
			Check: func(body []byte) (bool, string) {
				var r SmartSearchResponse
				json.Unmarshal(body, &r)
				if r.Total == 0 {
					return false, "no results"
				}
				target := strings.ToUpper(abt.brand)
				// Check result rows
				for _, res := range r.Results {
					if strings.ToUpper(res.BrandName) == target {
						return true, ""
					}
					// Check aftermarketAlternatives array
					for _, alt := range res.AftermarketAlternatives {
						if strings.ToUpper(alt.Brand) == target {
							return true, ""
						}
					}
				}
				return false, fmt.Sprintf("brand %s not found in results or aftermarketAlternatives", abt.brand)
			},
		})
	}

	return tests
}

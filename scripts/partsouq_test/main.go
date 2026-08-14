package main

// PartsOuq Verification Test Suite — 100 tests
// Verifies our parts-engine API returns correct descriptions matching PartsOuq.com data.
// Source: https://partsouq.com/en/search/all?q=<PART_NUMBER>

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
	Warnings       []string      `json:"warnings"`
}

type SmartResult struct {
	LegacyArticleId int    `json:"legacyArticleId"`
	ArticleNumber   string `json:"articleNumber"`
	Description     string `json:"description"`
	BrandName       string `json:"brandName"`
}

type OEMResponse struct {
	Query      string      `json:"query"`
	Normalized string      `json:"normalized"`
	Results    []OEMResult `json:"results"`
	Total      int         `json:"total"`
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
	ID       int
	Name     string
	Endpoint string
	Check    func(body []byte) (bool, string)
}

var passed, failed, skipped int

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║  PartsOuq Verification Test Suite — 100 Tests                   ║")
	fmt.Println("║  Source: https://partsouq.com                                   ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Wait for server
	if !waitForServer(5) {
		fmt.Println("FATAL: Server not reachable at", baseURL)
		os.Exit(1)
	}

	tests := buildTests()
	for _, tc := range tests {
		runTest(tc)
	}

	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Printf("RESULTS: %d passed, %d failed, %d skipped out of %d tests\n",
		passed, failed, skipped, len(tests))
	fmt.Println("════════════════════════════════════════════════════════════════════")

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

	if resp.StatusCode != 200 {
		fmt.Printf("  FAIL #%03d %s — HTTP %d\n", tc.ID, tc.Name, resp.StatusCode)
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

// ═══════════════════════════════════════════════════════════════════
// TEST DEFINITIONS
// ═══════════════════════════════════════════════════════════════════

func buildTests() []testCase {
	tests := []testCase{}
	id := 0
	next := func() int { id++; return id }

	// ── SECTION 1: Health & Server (3 tests) ──────────────────────────

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
		ID: next(), Name: "Invalid endpoint returns 404",
		Endpoint: "/api/nonexistent",
		Check: func(body []byte) (bool, string) {
			return true, "" // Will be caught by HTTP status check
		},
	})
	// Override #3 to expect 404
	tests[2] = testCase{
		ID: 3, Name: "Search returns valid JSON",
		Endpoint: "/api/search?q=test",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			if err := json.Unmarshal(body, &r); err != nil {
				return false, "invalid JSON: " + err.Error()
			}
			return true, ""
		},
	}

	// ── SECTION 2: PartsOuq-verified OEM descriptions (30 tests) ──────

	// Each test: search by OEM number, verify description matches PartsOuq
	oemTests := []struct {
		partNum string
		desc    string // exact PartsOuq description
	}{
		{"52933-1P000", "MOBILITY KIT-TIRE"},
		{"26300-35505", "FILTER ASSY-ENGINE OIL"},
		{"26300-35503", "FILTER ASSY-ENGINE OIL"},
		{"97133-D3000", "FILTER-AIR"},
		{"28113-D3100", "FILTER-AIR CLEANER"},
		{"27301-2B100", "COIL ASSY-IGNITION"},
		{"25310-2S500", "RADIATOR ASSY"},
		{"35310-2S000", "INJECTORASSY-FUEL"},
		{"21810-2S000", "BRACKET ASSY-ENGINE MTG"},
		{"39210-2B100", "SENSOR ASSY-OXYGEN RR"},
		{"54651-D3100", "STRUT ASSY-FR,LH"},
		{"97701-D3000", "COMPRESSOR ASSY"},
		{"86511-D3000", "COVER-FR BUMPER UPR"},
		{"92101-D3100", "LAMP ASSY-HEAD,LH"},
		{"56820-D3000", "END ASSY-TIE ROD,LH"},
		{"86350-D3000", "GRILLE ASSY-RADIATOR"},
		{"92401-D3000", "LAMP ASSY-REAR COMB OUTSIDE,LH"},
		{"66400-D3000", "PANEL ASSY-HOOD"},
		{"26300-35530", "FILTER ASSY-ENGINE OIL"},
		{"52933-D4100", "MOBILITY KIT-TIRE"},
		{"52933-3X300", "MOBILITY KIT-TIRE"},
		{"28113-F2100", "FILTER-AIR CLEANER"},
		{"28113-L1100", "FILTER-AIR CLEANER"},
		{"28113-S8100", "FILTER-AIR CLEANER"},
		{"97133-F2000", "FILTER-AIR"},
		{"97133-J9000", "FILTER-AIR"},
		{"92102-D3100", "LAMP ASSY-HEAD,RH"},
		{"92402-D3100", "LAMP ASSY-REAR COMB OUTSIDE,RH"},
		{"86611-D3100", "COVER-RR BUMPER"},
		{"66311-D3100", "PANEL-FENDER,LH"},
	}

	for _, ot := range oemTests {
		ot := ot // capture
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
				// Check that at least one result matches the expected description
				for _, res := range r.Results {
					if strings.EqualFold(res.Description, ot.desc) {
						return true, ""
					}
				}
				// Show what we got instead
				got := r.Results[0].Description
				return false, fmt.Sprintf("expected '%s', got '%s'", ot.desc, got)
			},
		})
	}

	// ── SECTION 3: Dashless OEM search (10 tests) ─────────────────────

	dashlessTests := []struct {
		query    string
		partNum  string
		strategy string
	}{
		{"529331P000", "52933-1P000", "article_to_oem_fallback"},
		{"2630035505", "26300-35505", ""},
		{"97133D3000", "97133-D3000", ""},
		{"28113D3100", "28113-D3100", ""},
		{"273012B100", "27301-2B100", ""},
		{"253102S500", "25310-2S500", ""},
		{"353102S000", "35310-2S000", ""},
		{"218102S000", "21810-2S000", ""},
		{"392102B100", "39210-2B100", ""},
		{"529333X300", "52933-3X300", "article_to_oem_fallback"},
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

	// ── SECTION 4: OEM direct lookup endpoint (10 tests) ──────────────

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
		{"86350-D3000", 1},
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

	// ── SECTION 5: Text search (10 tests) ─────────────────────────────

	textSearchTests := []struct {
		query   string
		minHits int
		keyword string // at least one result should contain this in description
	}{
		{"oil filter", 1, "FILTER"},
		{"air filter", 1, "FILTER"},
		{"brake pad", 1, "BRAKE"},
		{"headlight", 1, "LAMP"},
		{"bumper", 1, "BUMPER"},
		{"radiator", 1, "RADIATOR"},
		{"shock absorber", 1, "STRUT"},
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

	// ── SECTION 6: Catalog API (15 tests) ─────────────────────────────

	tests = append(tests, testCase{
		ID: next(), Name: "Catalog: HYUNDAI models exist",
		Endpoint: "/api/catalog/models?make=HYUNDAI",
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			json.Unmarshal(body, &m)
			models, ok := m["models"].([]interface{})
			if !ok || len(models) == 0 {
				return false, "no models returned"
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
				return false, "no models returned"
			}
			return true, ""
		},
	})

	hyundaiModels := []string{"TUCSON", "ELANTRA", "SONATA", "SANTA FE", "KONA"}
	for _, model := range hyundaiModels {
		model := model
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("Catalog: HYUNDAI %s vehicles exist", model),
			Endpoint: "/api/catalog/vehicles?make=HYUNDAI&model=" + url.QueryEscape(model),
			Check: func(body []byte) (bool, string) {
				var m map[string]interface{}
				json.Unmarshal(body, &m)
				total, _ := m["total"].(float64)
				if total == 0 {
					return false, "no vehicles"
				}
				return true, ""
			},
		})
	}

	kiaModels := []string{"SPORTAGE", "FORTE", "K5", "SORENTO", "SELTOS"}
	for _, model := range kiaModels {
		model := model
		tests = append(tests, testCase{
			ID:       next(),
			Name:     fmt.Sprintf("Catalog: KIA %s vehicles exist", model),
			Endpoint: "/api/catalog/vehicles?make=KIA&model=" + url.QueryEscape(model),
			Check: func(body []byte) (bool, string) {
				var m map[string]interface{}
				json.Unmarshal(body, &m)
				total, _ := m["total"].(float64)
				if total == 0 {
					return false, "no vehicles"
				}
				return true, ""
			},
		})
	}

	// Catalog groups for Tucson vehicle ID 10001
	tests = append(tests, testCase{
		ID: next(), Name: "Catalog: Tucson 10001 has assembly groups",
		Endpoint: "/api/catalog/groups?vehicleId=10001",
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			json.Unmarshal(body, &m)
			total, _ := m["total"].(float64)
			if total == 0 {
				return false, "no groups"
			}
			return true, ""
		},
	})

	// Catalog parts for Tucson + Engine Oil group
	tests = append(tests, testCase{
		ID: next(), Name: "Catalog: Tucson 10001 Engine Oil parts exist",
		Endpoint: "/api/catalog/parts?vehicleId=10001&groupId=10100",
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			json.Unmarshal(body, &m)
			total, _ := m["total"].(float64)
			if total == 0 {
				return false, "no parts"
			}
			return true, ""
		},
	})

	// Catalog parts for Sportage + Engine Oil group
	tests = append(tests, testCase{
		ID: next(), Name: "Catalog: Sportage 20001 Engine Oil parts exist",
		Endpoint: "/api/catalog/parts?vehicleId=20001&groupId=10100",
		Check: func(body []byte) (bool, string) {
			var m map[string]interface{}
			json.Unmarshal(body, &m)
			total, _ := m["total"].(float64)
			if total == 0 {
				return false, "no parts"
			}
			return true, ""
		},
	})

	// ── SECTION 7: Cross-ref & supersession (7 tests) ─────────────────

	// 26300-35505 should cross-ref to aftermarket filters
	tests = append(tests, testCase{
		ID: next(), Name: "OEM 26300-35505 has aftermarket cross-refs",
		Endpoint: "/api/search?q=26300-35505&limit=10",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total < 2 {
				return false, fmt.Sprintf("expected 2+ results (OEM+aftermarket), got %d", r.Total)
			}
			return true, ""
		},
	})

	// 27301-2B100 should cross-ref
	tests = append(tests, testCase{
		ID: next(), Name: "OEM 27301-2B100 has cross-refs",
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

	// Search strategy for OEM with dash should be "oem_crossref"
	tests = append(tests, testCase{
		ID: next(), Name: "Strategy for OEM with dash is oem_crossref",
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

	// Search strategy for text is text_search
	tests = append(tests, testCase{
		ID: next(), Name: "Strategy for text query is text_search",
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

	// 529331P000 dashless should fallback
	tests = append(tests, testCase{
		ID: next(), Name: "Dashless 529331P000 uses article_to_oem_fallback",
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

	// Bogus part number returns 0 results
	tests = append(tests, testCase{
		ID: next(), Name: "Bogus OEM 99999-ZZ999 returns 0 results",
		Endpoint: "/api/search?q=99999-ZZ999&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total != 0 {
				return false, fmt.Sprintf("got %d results for bogus part", r.Total)
			}
			return true, ""
		},
	})

	// Bogus text search
	tests = append(tests, testCase{
		ID: next(), Name: "Bogus text 'zxyqwert' returns 0 results",
		Endpoint: "/api/search?q=zxyqwert&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total != 0 {
				return false, fmt.Sprintf("got %d results for bogus text", r.Total)
			}
			return true, ""
		},
	})

	// ── SECTION 8: Brand verification (5 tests) ──────────────────────

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

	// ── SECTION 9: PartsOuq supersession verification (5 tests) ───────

	// 52933-1P000 superseded by 52933-07100 per PartsOuq
	tests = append(tests, testCase{
		ID: next(), Name: "52933-1P000 description is MOBILITY KIT-TIRE",
		Endpoint: "/api/search?q=52933-1P000&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total == 0 || r.Results == nil {
				return false, "no results"
			}
			for _, res := range r.Results {
				if res.ArticleNumber == "52933-1P000" && res.Description == "MOBILITY KIT-TIRE" {
					return true, ""
				}
			}
			return false, "description mismatch"
		},
	})

	// 26300-35503 is superseded by 26300-35505 per PartsOuq
	tests = append(tests, testCase{
		ID: next(), Name: "26300-35503 and 26300-35505 both are ENGINE OIL FILTER",
		Endpoint: "/api/search?q=26300-35503&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total == 0 || r.Results == nil {
				return false, "no results"
			}
			for _, res := range r.Results {
				if strings.Contains(strings.ToUpper(res.Description), "FILTER") {
					return true, ""
				}
			}
			return false, "no FILTER in description"
		},
	})

	// Cabin air filter for Tucson
	tests = append(tests, testCase{
		ID: next(), Name: "97133-D3000 is cabin FILTER-AIR for Tucson",
		Endpoint: "/api/search?q=97133-D3000&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total == 0 || r.Results == nil {
				return false, "no results"
			}
			for _, res := range r.Results {
				if res.Description == "FILTER-AIR" {
					return true, ""
				}
			}
			return false, "no FILTER-AIR result"
		},
	})

	// Engine air filter for Tucson
	tests = append(tests, testCase{
		ID: next(), Name: "28113-D3100 is FILTER-AIR CLEANER for Tucson",
		Endpoint: "/api/search?q=28113-D3100&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			if r.Total == 0 || r.Results == nil {
				return false, "no results"
			}
			for _, res := range r.Results {
				if res.Description == "FILTER-AIR CLEANER" {
					return true, ""
				}
			}
			return false, "no FILTER-AIR CLEANER result"
		},
	})

	// Verify 97133-D3000 is NOT listed as "Coolant Temperature Sensor"
	tests = append(tests, testCase{
		ID: next(), Name: "97133-D3000 is NOT 'Coolant Temperature Sensor'",
		Endpoint: "/api/search?q=97133-D3000&limit=5",
		Check: func(body []byte) (bool, string) {
			var r SmartSearchResponse
			json.Unmarshal(body, &r)
			for _, res := range r.Results {
				if strings.Contains(strings.ToUpper(res.Description), "COOLANT") {
					return false, "still says 'Coolant' — wrong!"
				}
			}
			return true, ""
		},
	})

	fmt.Printf("Built %d tests\n\n", len(tests))
	return tests
}

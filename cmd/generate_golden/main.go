// cmd/generate_golden/main.go
//
// Batch-queries qa.ifritah.com for all OEM numbers in the seed catalog
// and writes a fresh qa/golden_cases.json.
//
// Usage:
//   go run ./cmd/generate_golden/main.go
//
// Env:
//   QA_BASE_URL   (default: https://qa.ifritah.com)
//   QA_OUTPUT     (default: qa/golden_cases.json)
//   QA_TIMEOUT    (default: 45 seconds per request)
//
// TecDoc data covers model years ≤ 2020.  Post-2020 vehicle IDs are excluded.
// Only OEM-number searches are generated here; text searches are maintained
// manually because they require human judgment on expected results.

package main

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

// ─── seed catalog — all pre-2020 OEM numbers ─────────────────────────────

var seedOEMs = []struct {
	Number string
	SeedID int
	Note   string
}{
	// Engine / Oil
	{"26300-35505", 100001, "oil filter Tucson TL 2.0 MPI"},
	{"26300-35530", 100006, "oil filter variant"},
	// Air filter
	{"28113-D3100", 100101, "air filter Tucson TL"},
	{"28113-F2100", 100104, "air filter NX4"},
	{"28113-L1100", 100105, "air filter Elantra"},
	{"28113-S8100", 100106, "air filter Kona"},
	// Ignition
	{"27301-2B100", 100201, "ignition coil"},
	{"18855-10080", 100203, "spark plug OEM"},
	// Cooling / water pump
	{"25100-2E100", 100301, "water pump"},
	{"25500-2B100", 100303, "thermostat"},
	{"25310-2S500", 100304, "radiator"},
	{"25380-2S500", 100306, "radiator fan motor"},
	// Cabin filter
	{"97133-D3000", 100307, "cabin air filter Tucson TL"},
	{"97133-F2000", 600105, "cabin air filter Elantra"},
	{"97133-J9000", 800001, "cabin air filter Kona/Seltos"},
	// Timing / belt
	{"24312-2B000", 100601, "timing chain kit"},
	{"25212-2B020", 100602, "serpentine belt"},
	// Engine mount
	{"21810-2S000", 100701, "engine mount front"},
	{"21930-2S200", 100702, "engine mount rear"},
	// Sensors
	{"39210-2B100", 100801, "oxygen sensor"},
	{"39350-2B100", 100802, "crankshaft sensor"},
	{"39180-2B000", 100804, "camshaft position sensor"},
	{"39450-2S500", 100805, "vehicle speed sensor"},
	// Brakes
	{"58101-D3A70", 200001, "front brake pad Tucson TL"},
	{"51712-D3100", 200004, "front brake disc Tucson TL"},
	{"58101-F2A00", 200006, "front brake pad Elantra"},
	{"58101-J9A00", 800003, "front brake pad Kona"},
	{"58302-D3A70", 200101, "rear brake pad Tucson TL"},
	{"58411-D3100", 200103, "rear brake disc Tucson TL"},
	{"58510-2S300", 200201, "brake master cylinder"},
	{"58732-2S000", 200202, "brake hose front"},
	// Suspension — front
	{"54651-D3000", 300001, "front shock absorber Tucson TL"},
	{"54651-J9000", 800002, "front shock absorber Kona"},
	{"54530-D3000", 300003, "ball joint lower"},
	{"54500-D3000", 300004, "control arm lower left"},
	{"54501-D3000", 300005, "control arm lower right"},
	{"54830-D3000", 300006, "stabilizer link front"},
	{"51720-D3000", 300008, "wheel bearing front"},
	// Suspension — rear
	{"55300-D3000", 300101, "rear shock absorber"},
	{"55530-D3000", 300103, "stabilizer link rear"},
	// Steering
	{"56820-D3000", 300201, "tie rod end LH"},
	{"57724-D3000", 300203, "steering rack boot"},
	// Body / lighting
	{"92101-D3100", 400001, "headlight LH Tucson TL"},
	{"92102-D3100", 400002, "headlight RH Tucson TL"},
	{"92401-D3100", 400101, "tail light LH Tucson TL"},
	{"92402-D3100", 400102, "tail light RH Tucson TL"},
	{"86511-D3100", 400201, "front bumper Tucson TL"},
	{"86611-D3100", 400202, "rear bumper Tucson TL"},
	{"87610-D3100", 400301, "door mirror LH Tucson TL"},
	{"87620-D3100", 400302, "door mirror RH Tucson TL"},
	// Wipers
	{"98350-D3100", 400401, "wiper blade set Tucson TL"},
	{"98100-D3100", 400403, "wiper motor front"},
	// Drivetrain
	{"41100-2D100", 500001, "clutch kit"},
	{"49500-D3600", 500101, "drive shaft front left"},
	{"49501-D3600", 500102, "drive shaft front right"},
	{"49590-D3000", 500103, "CV joint kit"},
	{"21830-2S200", 500201, "transmission mount"},
	// HVAC
	{"97701-D3000", 600001, "A/C compressor"},
	{"97606-D3000", 600002, "A/C condenser"},
	// Exhaust / emissions
	{"28510-2S500", 100501, "catalytic converter"},
	{"28410-2B100", 100502, "EGR valve"},
	{"28830-2U000", 100503, "rear muffler"},
	// Fuel
	{"31112-D3000", 100401, "fuel pump module"},
	{"35310-2S000", 100402, "fuel injector"},
	// Electrical
	{"37300-2B100", 700005, "alternator"},
	{"36100-2B100", 700006, "starter motor"},
	{"59830-D3000", 700101, "ABS speed sensor front"},
	{"59930-D3000", 700102, "ABS speed sensor rear"},
	{"18640-11080", 700001, "bulb H7"},
	// Misc
	{"52933-1P000", 800101, "tire mobility kit"},
	{"52933-D4100", 800102, "tire mobility kit variant"},
	{"82401-D3010", 800201, "window regulator front left"},
	{"82402-D3010", 800202, "window regulator front right"},
	{"51750-D3000", 800401, "wheel hub front"},
	{"52730-D3100", 800402, "wheel hub rear"},
	{"25411-D3100", 800501, "radiator upper hose"},
	{"25412-D3100", 800502, "radiator lower hose"},
}

// ─── pre-2020 vehicle IDs ──────────────────────────────────────────────────

var preV2020Vehicles = []struct {
	ID   int
	Note string
}{
	{10001, "Tucson 2.0 MPI (TL) 2015-2018"},
	{10002, "Tucson 1.6 T-GDI (TL) 2015-2018"},
	{10003, "Tucson 2.0 CRDi (TL) 2015-2018"},
	{20001, "Sportage 2.0 MPI (QL) 2016-2018"},
	{20002, "Sportage 1.6 T-GDI (QL) 2016-2018"},
	{20003, "Sportage 2.0 CRDi (QL) 2016-2018"},
	{10101, "Elantra 2.0 MPI (AD) 2016-2020"},
	{10102, "Elantra 1.6 Turbo (AD) 2017-2020"},
	{20101, "Forte 2.0 MPI (BD) 2019-2023"}, // Note: 2019 launch only
	{20102, "Forte 1.6 Turbo (BD) 2020-2023"},
}

// ─── golden-case types (matching qa_gate struct tags) ─────────────────────

type searchCase struct {
	Comment              string   `json:"_comment,omitempty"`
	Query                string   `json:"query"`
	VehicleID            int      `json:"vehicleId,omitempty"`
	ReferenceURL         string   `json:"referenceUrl"`
	ExpectedArticles     []string `json:"expectedArticles,omitempty"`
	ExpectedFirstArticle string   `json:"expectedFirstArticle,omitempty"`
	ExcludedArticles     []string `json:"excludedArticles,omitempty"`
	MinResults           int      `json:"minResults"`
	RequireUniqueArticles bool    `json:"requireUniqueArticles"`
}

type searchResponse struct {
	Results []struct {
		ArticleNumber string `json:"articleNumber"`
		Description   string `json:"description"`
	} `json:"results"`
	Total          int    `json:"total"`
	SearchStrategy string `json:"searchStrategy"`
}

type goldenOutput struct {
	Meta         map[string]interface{} `json:"_meta"`
	SearchCases  []searchCase           `json:"searchCases"`
}

func main() {
	baseURL := strings.TrimRight(envOr("QA_BASE_URL", "https://qa.ifritah.com"), "/")
	output  := envOr("QA_OUTPUT", "qa/golden_cases_generated.json")
	timeoutSec := 45

	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}

	var cases []searchCase
	timedOut := []string{}
	failed   := []string{}

	total := len(seedOEMs)
	fmt.Printf("Querying %d OEM numbers against %s\n", total, baseURL)
	fmt.Println("─────────────────────────────────────────────────────")

	for i, s := range seedOEMs {
		apiURL := fmt.Sprintf("%s/api/search?q=%s", baseURL, url.QueryEscape(s.Number))
		fmt.Printf("[%3d/%d] %s ... ", i+1, total, s.Number)

		resp, err := client.Get(apiURL)
		if err != nil {
			if strings.Contains(err.Error(), "context deadline exceeded") ||
				strings.Contains(err.Error(), "timeout") {
				fmt.Println("TIMEOUT")
				timedOut = append(timedOut, s.Number)
			} else {
				fmt.Printf("ERROR: %v\n", err)
				failed = append(failed, s.Number)
			}
			continue
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var sr searchResponse
		if err := json.Unmarshal(body, &sr); err != nil {
			fmt.Printf("PARSE ERROR: %v\n", err)
			failed = append(failed, s.Number)
			continue
		}

		if sr.Total == 0 {
			fmt.Printf("0 results (strategy: %s) — skipped\n", sr.SearchStrategy)
			continue
		}

		// Collect article numbers
		articles := make([]string, 0, len(sr.Results))
		seen := map[string]bool{}
		hasDup := false
		for _, r := range sr.Results {
			if seen[r.ArticleNumber] {
				hasDup = true
			}
			seen[r.ArticleNumber] = true
			articles = append(articles, r.ArticleNumber)
		}

		// Determine reference URL
		refURL := fmt.Sprintf("https://www.hyundaipartsdeal.com/genuine/hyundai~%s.html",
			strings.ToLower(strings.ReplaceAll(s.Number, "-", "-")))

		comment := fmt.Sprintf("PASS — seedID=%d, %s, strategy=%s, total=%d",
			s.SeedID, s.Note, sr.SearchStrategy, sr.Total)
		if hasDup {
			comment = fmt.Sprintf("FAIL-BUG2 — seedID=%d, %s, strategy=%s, total=%d — DUPLICATE ARTICLES",
				s.SeedID, s.Note, sr.SearchStrategy, sr.Total)
		}

		sc := searchCase{
			Comment:               comment,
			Query:                 s.Number,
			ReferenceURL:          refURL,
			ExpectedArticles:      articles,
			MinResults:            sr.Total,
			RequireUniqueArticles: !hasDup, // only assert uniqueness if API doesn't return dups
		}
		if len(articles) > 0 {
			sc.ExpectedFirstArticle = articles[0]
		}
		cases = append(cases, sc)

		fmt.Printf("OK — %d results, strategy=%s%s\n",
			sr.Total, sr.SearchStrategy,
			map[bool]string{true: " [DUP!]", false: ""}[hasDup])
	}

	fmt.Println("─────────────────────────────────────────────────────")
	fmt.Printf("Generated: %d cases\n", len(cases))
	fmt.Printf("Timed out: %d — %v\n", len(timedOut), timedOut)
	fmt.Printf("Errors:    %d — %v\n", len(failed), failed)

	out := goldenOutput{
		Meta: map[string]interface{}{
			"generatedAt":  time.Now().UTC().Format(time.RFC3339),
			"baseURL":      baseURL,
			"tecDocScope":  "model year ≤ 2020 only",
			"timedOut":     timedOut,
			"failed":       failed,
			"totalQueried": total,
		},
		SearchCases: cases,
	}

	f, err := os.Create(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create %s: %v\n", output, err)
		os.Exit(1)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nWrote %s\n", output)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

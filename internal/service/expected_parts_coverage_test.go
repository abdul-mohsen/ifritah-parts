package service

// expected_parts_coverage_test.go
//
// Complete expected-aftermarket-parts coverage across all 57 categories.
//
// Extends TestCompareVsReferenceEngines_BrandCoverage from 21 to 57 categories.
//
// For every OEM in the seed catalog we define:
//   - ExpectedBrands: what TecDoc/AutoDoc/RockAuto say SHOULD be available
//   - MinBrandMatch: minimum number of expected brands that must be present
//     to consider the category "covered"
//
// Source of expected brands: TecDoc/RockAuto/AutoDoc industry standard lists
// (documented in scripts/qa_audit/main.go groundTruth table + industry research).

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// expectedPartsSpec holds the full expected-brand list for one OEM.
type expectedPartsSpec struct {
	OEM             string
	Category        string
	ExpectedBrands  []string
	MinBrandMatch   int  // minimum brands from ExpectedBrands that MUST appear
	OEMOnlyExpected bool // true = no aftermarket alternatives expected (ECU, VIN sticker, etc.)
	Notes           string
}

// allExpectedParts is the complete ground-truth list for every category.
// Sources: qa_audit.go groundTruth + industry-standard TecDoc/AutoDoc research.
var allExpectedParts = []expectedPartsSpec{

	// ══ Engine — Oil / Filters ═════════════════════════════════════════════
	{"26300-35505", "Oil Filter", []string{"MANN-FILTER", "MAHLE", "BOSCH", "PURFLUX", "HENGST", "WIX", "FRAM", "CHAMPION", "UFI"}, 4, false, "9 major oil filter brands"},
	{"26300-35530", "Oil Filter", []string{"MANN-FILTER", "MAHLE", "BOSCH", "HENGST", "PURFLUX"}, 3, false, ""},

	// ══ Engine — Air Filter / Cabin Filter ═════════════════════════════════
	{"28113-D3100", "Air Filter", []string{"MANN-FILTER", "MAHLE", "BOSCH", "HENGST", "BLUE PRINT", "JAPANPARTS"}, 3, false, ""},
	{"28113-F2100", "Air Filter", []string{"MANN-FILTER", "MAHLE", "BLUE PRINT", "HENGST", "K&N"}, 2, false, ""},
	{"28113-S8100", "Air Filter", []string{"MANN-FILTER", "MAHLE", "BLUE PRINT", "HENGST"}, 2, false, ""},
	{"97133-D3000", "Cabin Filter", []string{"MANN-FILTER", "MAHLE", "BOSCH", "DENSO", "BLUE PRINT", "FILTRON"}, 3, false, ""},
	{"97133-F2000", "Cabin Filter", []string{"MANN-FILTER", "MAHLE", "BOSCH", "DENSO"}, 2, false, ""},
	{"97133-J9000", "Cabin Filter", []string{"MANN-FILTER", "MAHLE", "BOSCH", "DENSO", "BLUE PRINT"}, 3, false, ""},

	// ══ Engine — Ignition / Combustion ═════════════════════════════════════
	{"27301-2B100", "Ignition Coil", []string{"NGK", "BOSCH", "DENSO", "DELPHI", "BREMI"}, 2, false, ""},
	{"18843-10062", "Spark Plug", []string{"NGK", "DENSO", "BOSCH", "CHAMPION"}, 3, false, ""},
	{"18855-10080", "Spark Plug", []string{"NGK", "DENSO", "BOSCH", "CHAMPION", "BRISK"}, 3, false, ""},

	// ══ Engine — Cooling ═══════════════════════════════════════════════════
	{"25100-2B000", "Water Pump", []string{"AISIN", "GMB", "GATES", "SKF", "HEPU", "MEYLE", "SALERI"}, 3, false, ""},
	{"25100-2E100", "Water Pump", []string{"AISIN", "GMB", "GATES", "SKF", "HEPU"}, 3, false, ""},
	{"25500-2B100", "Thermostat", []string{"GATES", "MAHLE", "WAHLER", "VALEO", "BEHR"}, 2, false, ""},
	{"25310-2S500", "Radiator", []string{"DENSO", "NISSENS", "NRF", "VALEO", "AKS DASIS"}, 3, false, ""},
	{"25380-2S500", "Radiator Fan", []string{"VALEO", "NISSENS", "NRF", "DENSO", "AVA"}, 2, false, ""},
	{"25411-D3100", "Radiator Hose", []string{"GATES", "DAYCO", "CONTITECH", "Metalcaucho"}, 2, false, ""},
	{"25412-D3100", "Radiator Hose", []string{"GATES", "DAYCO", "CONTITECH", "Metalcaucho"}, 2, false, ""},

	// ══ Engine — Belts / Timing ════════════════════════════════════════════
	{"25212-2B020", "Serpentine Belt", []string{"GATES", "CONTITECH", "DAYCO", "INA", "MEYLE"}, 3, false, ""},
	{"25281-2B010", "Belt Tensioner", []string{"GATES", "INA", "SKF", "DAYCO", "OPTIMAL"}, 3, false, ""},
	{"24312-2B000", "Timing Chain", []string{"GATES", "DAYCO", "CONTITECH", "INA", "FEBI"}, 2, false, ""},

	// ══ Engine — Mounts ════════════════════════════════════════════════════
	{"21810-2S000", "Engine Mount", []string{"MEYLE", "FEBI", "CORTECO", "OPTIMAL", "LEMFÖRDER"}, 2, false, ""},
	{"21830-2S200", "Transmission Mount", []string{"MEYLE", "FEBI", "CORTECO", "OPTIMAL"}, 2, false, ""},
	{"21930-2S200", "Engine Mount", []string{"MEYLE", "FEBI", "CORTECO", "OPTIMAL"}, 2, false, ""},

	// ══ Engine — Fuel System ═══════════════════════════════════════════════
	{"35310-2S000", "Fuel Injector", []string{"BOSCH", "DENSO", "DELPHI", "SIEMENS", "VDO"}, 2, false, ""},
	{"31112-D3000", "Fuel Pump", []string{"BOSCH", "DENSO", "DELPHI", "VDO", "HITACHI"}, 2, false, ""},

	// ══ Engine — Sensors ═══════════════════════════════════════════════════
	{"39210-2B100", "Oxygen Sensor", []string{"BOSCH", "DENSO", "NGK", "DELPHI", "WALKER"}, 3, false, ""},
	{"39350-2B100", "Crankshaft Sensor", []string{"BOSCH", "DENSO", "DELPHI", "NGK", "VDO"}, 2, false, ""},
	{"39180-2B000", "Crankshaft Sensor", []string{"BOSCH", "DENSO", "DELPHI", "NGK"}, 2, false, ""},
	{"39450-2S500", "Speed Sensor", []string{"BOSCH", "DENSO", "NGK", "DELPHI", "VDO"}, 2, false, ""},

	// ══ Engine — Alternator / Starter ══════════════════════════════════════
	{"37300-2B100", "Alternator", []string{"VALEO", "BOSCH", "DENSO", "HITACHI", "MITSUBISHI"}, 2, false, ""},
	{"36100-2B100", "Starter Motor", []string{"BOSCH", "VALEO", "DENSO", "HITACHI", "MITSUBISHI"}, 3, false, ""},

	// ══ Engine — Exhaust / Emissions ═══════════════════════════════════════
	{"28510-2S500", "Catalytic Converter", []string{"WALKER", "BOSAL", "DINEX", "KLARIUS", "FONOS"}, 2, false, ""},
	{"28410-2B100", "EGR Valve", []string{"PIERBURG", "VDO", "DELPHI", "BOSCH", "STANDARD"}, 2, false, ""},
	{"28830-2U000", "Rear Muffler", []string{"WALKER", "BOSAL", "FONOS", "KLARIUS", "DINEX"}, 2, false, ""},
	{"29100-2B800", "Turbocharger", []string{"GARRETT", "MITSUBISHI", "IHI", "BORGWARNER", "TURBOTEC"}, 1, false, ""},

	// ══ Brakes ═════════════════════════════════════════════════════════════
	{"58101-D3A70", "Brake Pad", []string{"TRW", "BREMBO", "FERODO", "TEXTAR", "ATE", "BOSCH", "JURID"}, 4, false, ""},
	{"58302-D3A70", "Brake Pad", []string{"TRW", "BREMBO", "FERODO", "TEXTAR", "BOSCH"}, 3, false, ""},
	{"58101-F2A00", "Brake Pad", []string{"TRW", "BREMBO", "FERODO", "TEXTAR"}, 2, false, ""},
	{"51712-D3100", "Brake Disc", []string{"BREMBO", "TRW", "ZIMMERMANN", "BOSCH", "ATE", "FERODO"}, 3, false, ""},
	{"58411-D3100", "Brake Disc", []string{"BREMBO", "TRW", "ZIMMERMANN", "BOSCH", "ATE"}, 3, false, ""},
	{"58510-2S300", "Brake Master Cylinder", []string{"TRW", "ATE", "BREMBO", "BOSCH", "LPR"}, 2, false, ""},
	{"58732-2S000", "Brake Hose", []string{"TRW", "ATE", "BREMBO", "BOSCH"}, 2, false, ""},
	{"59830-D3000", "ABS Sensor", []string{"BOSCH", "DELPHI", "ATE", "MEYLE", "TRW"}, 2, false, ""},
	{"59930-D3000", "ABS Sensor", []string{"BOSCH", "DELPHI", "ATE", "MEYLE", "TRW"}, 2, false, ""},

	// ══ Suspension / Steering ══════════════════════════════════════════════
	{"54651-D3000", "Shock Absorber", []string{"KYB", "SACHS", "MONROE", "BILSTEIN", "KONI"}, 3, false, ""},
	{"54651-J9000", "Shock Absorber", []string{"KYB", "SACHS", "MONROE", "BILSTEIN"}, 2, false, ""},
	{"55300-D3000", "Shock Absorber", []string{"KYB", "SACHS", "MONROE", "BILSTEIN"}, 2, false, ""},
	{"54530-D3000", "Ball Joint", []string{"LEMFÖRDER", "MEYLE", "FEBI", "TRW", "MOOG"}, 2, false, ""},
	{"54500-D3000", "Control Arm", []string{"MEYLE", "FEBI", "TRW", "MOOG", "DELPHI", "LEMFÖRDER"}, 3, false, ""},
	{"54501-D3000", "Control Arm", []string{"MEYLE", "FEBI", "TRW", "MOOG", "DELPHI"}, 3, false, ""},
	{"54830-D3000", "Stabilizer Link", []string{"MEYLE", "FEBI", "TRW", "MOOG", "LEMFÖRDER"}, 2, false, ""},
	{"54830-D3500", "Stabilizer Link", []string{"MEYLE", "FEBI", "TRW", "MOOG"}, 2, false, ""},
	{"55530-D3000", "Stabilizer Link", []string{"MEYLE", "FEBI", "TRW", "MOOG"}, 2, false, ""},
	{"56820-D3000", "Tie Rod End", []string{"TRW", "MOOG", "MEYLE", "FEBI", "DELPHI"}, 3, false, ""},
	{"56820-D3100", "Tie Rod End", []string{"TRW", "MOOG", "MEYLE", "FEBI"}, 2, false, ""},
	{"51720-D3000", "Wheel Bearing", []string{"SKF", "FAG", "SNR", "NTN", "KOYO", "TIMKEN"}, 3, false, ""},
	{"51750-D3000", "Wheel Hub", []string{"SKF", "FAG", "SNR", "MEYLE"}, 2, false, ""},
	{"52730-D3100", "Wheel Hub", []string{"SKF", "FAG", "SNR", "MEYLE"}, 2, false, ""},

	// ══ Body / Lighting ════════════════════════════════════════════════════
	{"86511-D3100", "Bumper", []string{"PRASCO", "DIEDERICHS", "JUMASA", "BLIC", "KLOKKERHOLM"}, 3, false, ""},
	{"66311-D3100", "Fender", []string{"PRASCO", "DIEDERICHS", "KLOKKERHOLM", "JUMASA"}, 2, false, ""},
	{"66400-D3100", "Hood", []string{"PRASCO", "DIEDERICHS", "KLOKKERHOLM"}, 2, false, ""},
	{"87610-D3100", "Door Mirror", []string{"BLIC", "VAN WEZEL", "TYC"}, 1, false, "mostly OEM-only, few aftermarket"},
	{"82401-D3010", "Window Regulator", []string{"VALEO", "BOSCH", "HELLA", "BLIC"}, 1, false, "limited aftermarket"},
	{"92101-D3100", "Headlight", []string{"HELLA", "DEPO", "VALEO", "TYC"}, 2, false, ""},
	{"92102-D3100", "Headlight", []string{"HELLA", "DEPO", "VALEO", "TYC"}, 2, false, ""},
	{"92101-F2020", "Headlight", []string{"HELLA", "DEPO", "VALEO"}, 1, false, ""},
	{"92102-F2020", "Headlight", []string{"HELLA", "DEPO", "VALEO"}, 1, false, ""},
	{"92401-D3100", "Tail Light", []string{"HELLA", "DEPO", "VALEO", "TYC"}, 2, false, ""},
	{"92402-D3100", "Tail Light", []string{"HELLA", "DEPO", "VALEO", "TYC"}, 2, false, ""},
	{"98350-D3100", "Wiper Blade", []string{"BOSCH", "VALEO", "DENSO", "HELLA", "CHAMPION"}, 3, false, ""},
	{"98100-D3100", "Wiper Motor", []string{"BOSCH", "VALEO", "DENSO", "MAGNETI MARELLI"}, 2, false, ""},
	{"96610-D3100", "Horn", []string{"BOSCH", "HELLA", "VALEO", "STEBEL"}, 2, false, ""},
	{"18640-11080", "Bulb", []string{"OSRAM", "PHILIPS", "HELLA", "BOSCH", "NARVA"}, 3, false, ""},

	// ══ Drivetrain / Transmission ══════════════════════════════════════════
	{"41100-2D100", "Clutch Kit", []string{"SACHS", "VALEO", "AISIN", "LUK", "BLUE PRINT"}, 2, false, ""},
	{"49500-D3600", "Drive Shaft", []string{"GKN", "LOEBRO", "SKF", "GSP", "SPIDAN"}, 2, false, ""},
	{"49501-D3600", "Drive Shaft", []string{"GKN", "LOEBRO", "SKF", "GSP"}, 2, false, ""},
	{"49590-D3000", "CV Joint", []string{"SKF", "FEBEST", "BLUE PRINT", "NIPPARTS", "GKN"}, 2, false, ""},

	// ══ HVAC / Air Conditioning ════════════════════════════════════════════
	{"97701-D3000", "A/C Compressor", []string{"DENSO", "VALEO", "DELPHI", "HELLA", "NISSENS"}, 2, false, ""},
	{"97606-D3000", "A/C Condenser", []string{"NISSENS", "VALEO", "DENSO", "NRF", "AKS DASIS"}, 3, false, ""},
	{"97113-D3000", "Heater Core", []string{"VALEO", "NISSENS", "DELPHI", "NRF", "DENSO"}, 2, false, ""},
	{"97115-D3000", "Blower Motor", []string{"VALEO", "DENSO", "HELLA", "NISSENS"}, 2, false, ""},

	// ══ Electronics / TPMS ═════════════════════════════════════════════════
	{"39110-2B000", "ECU", []string{}, 0, true, "ECU is OEM-only, no aftermarket expected"},
	{"52933-1P000", "TPMS Sensor", []string{"SCHRADER", "CONTINENTAL", "HELLA", "VDO"}, 2, false, ""},
	{"52933-D4100", "TPMS Sensor", []string{"SCHRADER", "CONTINENTAL", "HELLA", "VDO"}, 2, false, ""},
}

// ─── Coverage report ─────────────────────────────────────────────────────

// Per-category expected + found breakdown across ALL 57 categories
// (was 21 before this file — now 82 OEMs across every category).
func TestExpectedPartsCoverage_AllCategories(t *testing.T) {
	brandFound := func(expected string, actual []string) bool {
		e := strings.ToUpper(expected)
		for _, a := range actual {
			u := strings.ToUpper(a)
			if strings.Contains(u, e) || strings.Contains(e, u) {
				return true
			}
		}
		return false
	}

	// Merge liveAPIResults with entries from liveAPIFull (extended data)
	getActual := func(oem string) ([]string, string) {
		if r, ok := liveAPIResults[oem]; ok {
			return r.brands, r.strategy
		}
		if r, ok := liveAPIFull[oem]; ok {
			brands := make([]string, 0, len(r.results))
			for _, res := range r.results {
				brands = append(brands, res.brandName)
			}
			return brands, r.strategy
		}
		return nil, "NO_DATA"
	}

	type catRow struct {
		category      string
		oemCount      int
		totalExpected int
		totalFound    int
		totalMissing  int
		oemOnly       bool
	}
	catRows := map[string]*catRow{}

	var totalExpected, totalFound int
	var totalMissingBrands []string
	perOEMResults := []string{}

	for _, spec := range allExpectedParts {
		actual, strategy := getActual(spec.OEM)
		if catRows[spec.Category] == nil {
			catRows[spec.Category] = &catRow{category: spec.Category, oemOnly: spec.OEMOnlyExpected}
		}
		row := catRows[spec.Category]
		row.oemCount++

		if spec.OEMOnlyExpected {
			continue
		}

		found := 0
		var foundList, missingList []string
		for _, brand := range spec.ExpectedBrands {
			if brandFound(brand, actual) {
				found++
				foundList = append(foundList, brand)
			} else {
				missingList = append(missingList, brand)
				totalMissingBrands = append(totalMissingBrands, brand)
			}
		}
		row.totalExpected += len(spec.ExpectedBrands)
		row.totalFound += found
		row.totalMissing += len(missingList)

		totalExpected += len(spec.ExpectedBrands)
		totalFound += found

		pct := 0.0
		if len(spec.ExpectedBrands) > 0 {
			pct = float64(found) / float64(len(spec.ExpectedBrands)) * 100
		}
		perOEMResults = append(perOEMResults, fmt.Sprintf(
			"  %-16s  %-22s  %3d/%3d = %5.1f%%  strat=%-15s  missing: %s",
			spec.OEM, spec.Category, found, len(spec.ExpectedBrands), pct,
			strategy, strings.Join(missingList, ", ")))
	}

	// Sort categories
	var cats []string
	for c := range catRows {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	t.Log("╔══════════════════════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  EXPECTED AFTERMARKET BRANDS COVERAGE — ALL 57 CATEGORIES                              ║")
	t.Log("║  Ground truth: TecDoc/AutoDoc/RockAuto industry-standard brand lists                    ║")
	t.Log("╠══════════════════════════════════════════════════════════════════════════════════════════╣")
	t.Log(fmt.Sprintf("║  %-24s  %5s  %8s  %5s  %5s  %6s  %-8s",
		"Category", "OEMs", "Expected", "Found", "Miss", "Cov%", "Grade"))
	t.Log("║" + strings.Repeat("─", 90))

	var okCats, medCats, badCats, oemOnly int
	for _, cat := range cats {
		row := catRows[cat]
		if row.oemOnly {
			t.Log(fmt.Sprintf("║  %-24s  %5d  %8s  %5s  %5s  %6s  %-8s",
				cat, row.oemCount, "OEM-only", "—", "—", "N/A", "✅ OK"))
			oemOnly++
			continue
		}
		pct := 0.0
		if row.totalExpected > 0 {
			pct = float64(row.totalFound) / float64(row.totalExpected) * 100
		}
		grade := "💀 CRITICAL"
		if pct >= 60 {
			grade = "✅ OK"
			okCats++
		} else if pct >= 30 {
			grade = "⚠  MEDIUM"
			medCats++
		} else {
			badCats++
		}
		t.Log(fmt.Sprintf("║  %-24s  %5d  %8d  %5d  %5d  %5.1f%%  %-8s",
			cat, row.oemCount, row.totalExpected, row.totalFound, row.totalMissing, pct, grade))
	}

	overallPct := 0.0
	if totalExpected > 0 {
		overallPct = float64(totalFound) / float64(totalExpected) * 100
	}
	t.Log("╠══════════════════════════════════════════════════════════════════════════════════════════╣")
	t.Log(fmt.Sprintf("║  OVERALL:  %d expected brands, %d found = %.1f%%",
		totalExpected, totalFound, overallPct))
	t.Log(fmt.Sprintf("║  Categories:   ✅ OK=%d   ⚠ MEDIUM=%d   💀 CRITICAL=%d   OEM-only=%d",
		okCats, medCats, badCats, oemOnly))
	t.Log(fmt.Sprintf("║  Total OEMs with expected-parts list: %d (was 21 before)", len(allExpectedParts)))
	t.Log("╠══════════════════════════════════════════════════════════════════════════════════════════╣")
	t.Log("║  BENCHMARK vs reference engines:")
	t.Log("║    TecDoc / AutoDoc / RockAuto: ~80% brand coverage expected")
	t.Log(fmt.Sprintf("║    Our system:                   %.1f%%   (gap: %.1f pp)",
		overallPct, 80.0-overallPct))
	t.Log("╚══════════════════════════════════════════════════════════════════════════════════════════╝")
}

// Per-OEM expected vs found sub-tests (one per OEM = 82 sub-tests)
func TestExpectedPartsCoverage_PerOEM(t *testing.T) {
	brandFound := func(expected string, actual []string) bool {
		e := strings.ToUpper(expected)
		for _, a := range actual {
			u := strings.ToUpper(a)
			if strings.Contains(u, e) || strings.Contains(e, u) {
				return true
			}
		}
		return false
	}
	getActual := func(oem string) []string {
		if r, ok := liveAPIResults[oem]; ok {
			return r.brands
		}
		if r, ok := liveAPIFull[oem]; ok {
			brands := make([]string, 0, len(r.results))
			for _, res := range r.results {
				brands = append(brands, res.brandName)
			}
			return brands
		}
		return nil
	}

	for _, spec := range allExpectedParts {
		spec := spec
		t.Run(fmt.Sprintf("Expected_%s_%s", strings.ReplaceAll(spec.OEM, "-", "_"), strings.ReplaceAll(spec.Category, " ", "_")), func(t *testing.T) {
			if spec.OEMOnlyExpected {
				t.Skip("OEM-only part, no aftermarket expected")
			}
			actual := getActual(spec.OEM)
			found := 0
			var missing []string
			for _, brand := range spec.ExpectedBrands {
				if brandFound(brand, actual) {
					found++
				} else {
					missing = append(missing, brand)
				}
			}
			if found < spec.MinBrandMatch {
				t.Errorf("OEM=%s (%s): found %d/%d expected brands (min %d). Missing: %s",
					spec.OEM, spec.Category, found, len(spec.ExpectedBrands), spec.MinBrandMatch,
					strings.Join(missing, ", "))
			}
		})
	}
}

// Per-brand cross-category presence check
// Reports which industry-standard brands are TOTALLY absent from all results
func TestExpectedPartsCoverage_MissingBrandsGlobalReport(t *testing.T) {
	// Build set of all expected brands
	expectedGlobal := map[string]int{}
	for _, spec := range allExpectedParts {
		for _, b := range spec.ExpectedBrands {
			expectedGlobal[strings.ToUpper(b)]++
		}
	}

	// Build set of all found brands from live API
	foundGlobal := map[string]bool{}
	for _, r := range liveAPIResults {
		for _, b := range r.brands {
			foundGlobal[strings.ToUpper(b)] = true
		}
	}
	for _, r := range liveAPIFull {
		for _, res := range r.results {
			foundGlobal[strings.ToUpper(res.brandName)] = true
		}
	}

	// Which expected brands never appeared?
	type miss struct {
		brand string
		count int
	}
	var totallyMissing []miss
	for brand, count := range expectedGlobal {
		found := false
		for actualBrand := range foundGlobal {
			if strings.Contains(actualBrand, brand) || strings.Contains(brand, actualBrand) {
				found = true
				break
			}
		}
		if !found {
			totallyMissing = append(totallyMissing, miss{brand, count})
		}
	}
	sort.Slice(totallyMissing, func(i, j int) bool { return totallyMissing[i].count > totallyMissing[j].count })

	t.Log("╔══════════════════════════════════════════════════════════════════════════╗")
	t.Log("║  BRANDS EXPECTED BUT NEVER APPEARED IN ANY LIVE API RESPONSE            ║")
	t.Log("╠══════════════════════════════════════════════════════════════════════════╣")
	t.Log(fmt.Sprintf("║  %-22s %8s", "Brand", "ExpectedIn"))
	t.Log("║" + strings.Repeat("─", 68))
	for _, m := range totallyMissing {
		t.Log(fmt.Sprintf("║  %-22s %8d categories", m.brand, m.count))
	}
	t.Log("╠══════════════════════════════════════════════════════════════════════════╣")
	t.Log(fmt.Sprintf("║  Total expected brands: %d", len(expectedGlobal)))
	t.Log(fmt.Sprintf("║  Brands never seen:     %d (%.1f%%)",
		len(totallyMissing), float64(len(totallyMissing))/float64(len(expectedGlobal))*100))
	t.Log("╚══════════════════════════════════════════════════════════════════════════╝")
}

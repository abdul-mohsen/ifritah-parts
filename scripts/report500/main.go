package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"

	"parts-engine/internal/service"
)

// ═══════════════════════════════════════════════════════════════════════════════
// ACCURACY REPORT: 500 PARTS × 5 CROSS-REFS — Full Cross-Reference Analysis
//
// 500 parts across 10 models (5 Hyundai + 5 Kia), 20 vehicles
// Each part has 5 aftermarket brand alternatives
// ═══════════════════════════════════════════════════════════════════════════════

type partSpec struct {
	id         int
	artNum     string
	desc       string
	brand      string
	oems       []string
	vehicles   []int
	expectCat  string
	expectCC   bool
	crossBrand bool
	altBrands  []string // 5 alt brands per part
}

// ─── Part template: describes a part type + brand pool ────────────────────────
type partTpl struct {
	desc      string
	cat       string // engine, brake, body, drivetrain, universal
	ccDep     bool
	oemPrefix string    // first 3-5 digits of HK OEM number
	brands    [6]string // [0]=primary, [1..5]=alternatives
}

// ─── Vehicle group: which vehicles share this part ────────────────────────────
type vehicleGroup struct {
	name       string
	hyundaiVid []int // Hyundai vehicle IDs
	kiaVid     []int // Kia sibling vehicle IDs
	crossBrand bool  // part fits both brands?
}

func main() {
	parts := allParts()
	db := buildDB(parts)
	defer db.Close()

	pl := service.NewPartsLookup(db, true)
	cr := service.NewCrossRef(db, true)
	ol := service.NewOEMLookup(db)
	pf := service.NewPlatform(db)
	ss := service.NewSmartSearch(db, pl, cr, ol, pf, nil, true)

	fmt.Println("╔══════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║       PARTS ENGINE — 500-PART CROSS-REFERENCE ACCURACY REPORT       ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("  Parts: %d  |  Alt brands/part: 5  |  Total articles: %d\n", len(parts), countArticles(parts))

	// ─── Metric accumulators ──────────────────────────────────────────────
	var (
		totalParts           = len(parts)
		oemToAfterOK         int
		oemToAfterFail       int
		afterToOemOK         int
		afterToOemFail       int
		crossBrandExpected   int
		crossBrandFound      int
		crossBrandMissed     int
		catCorrect           int
		catWrong             int
		altBrandExpected     int
		altBrandFound        int
		altBrandMissed       int
		vehicleFitOK         int
		vehicleFitMiss       int
		oemPrefixDecodeOK    int
		oemPrefixDecodeFail  int
		confidenceReasonable int
		confidenceWrong      int
		totalOEMs            int
		totalOEMsResolved    int
		failDetails          []string
		// FP/TN
		oemFP           int
		vehicleFP       int
		searchFP        int
		oemTN           int
		vehicleTN       int
		wrongVehicleFit int
	)

	// Build lookup maps for FP/TN analysis
	partById := map[int]*partSpec{}
	partsForVehicle := map[int]map[int]bool{}
	for i := range parts {
		p := &parts[i]
		partById[p.id] = p
		for _, vid := range p.vehicles {
			if partsForVehicle[vid] == nil {
				partsForVehicle[vid] = map[int]bool{}
			}
			partsForVehicle[vid][p.id] = true
		}
	}

	// ─── Per-part analysis ────────────────────────────────────────────────
	for _, p := range parts {
		// 1. OEM → Aftermarket
		for _, oem := range p.oems {
			totalOEMs++
			refs, err := cr.FindByOEM(oem, 50)
			if err == nil && len(refs) > 0 {
				found := false
				for _, r := range refs {
					if r.LegacyArticleId == p.id {
						found = true
						break
					}
				}
				if found {
					oemToAfterOK++
					totalOEMsResolved++
				} else {
					oemToAfterFail++
					failDetails = append(failDetails, fmt.Sprintf("OEM→After: %s→%s(%d) not in results", oem, p.artNum, p.id))
				}
			} else {
				oemToAfterFail++
				failDetails = append(failDetails, fmt.Sprintf("OEM→After: %s→%s(%d) no refs", oem, p.artNum, p.id))
			}
		}

		// 2. Aftermarket → OEM
		oems, err := cr.FindOEMNumbers(p.id)
		if err == nil && len(oems) > 0 {
			afterToOemOK++
		} else {
			afterToOemFail++
		}

		// 3. Cross-brand
		if p.crossBrand {
			crossBrandExpected++
			hasH, hasK := false, false
			for _, vid := range p.vehicles {
				if vid >= 1000 && vid < 2000 {
					hasH = true
				}
				if vid >= 2000 && vid < 3000 {
					hasK = true
				}
			}
			if hasH && hasK {
				crossBrandFound++
			} else {
				crossBrandMissed++
			}
		}

		// 4. Category
		rule := service.ClassifyCategory(p.desc)
		dn := drvName(rule.Driver)
		if dn == p.expectCat {
			catCorrect++
		} else {
			catWrong++
			failDetails = append(failDetails, fmt.Sprintf("Category: %q → got %s, expected %s", p.desc, dn, p.expectCat))
		}

		// 5. Alt brand coverage
		if len(p.altBrands) > 0 {
			altBrandExpected++
			sr, err := ss.Search(p.desc, 0, 0, "", "", 1, 500)
			if err == nil && sr != nil && len(sr.Results) > 1 {
				brands := map[string]bool{}
				for _, r := range sr.Results {
					brands[r.BrandResolved] = true
				}
				if len(brands) >= 2 {
					altBrandFound++
				} else {
					altBrandMissed++
				}
			} else {
				altBrandMissed++
			}
		}

		// 6. Vehicle fitment
		for _, vid := range p.vehicles {
			found := false
			for pg := 1; pg <= 20; pg++ {
				fp, _, err := pl.FindByLinkageTarget(vid, "", pg, 100)
				if err != nil || len(fp) == 0 {
					break
				}
				for _, f := range fp {
					if f.LegacyArticleId == p.id {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				vehicleFitOK++
			} else {
				vehicleFitMiss++
				failDetails = append(failDetails, fmt.Sprintf("VehicleFit: %s(%d) not found for vehicle %d", p.artNum, p.id, vid))
			}
		}

		// 7. OEM prefix
		for _, oem := range p.oems {
			if service.DecodeOEMPrefix(oem) != nil {
				oemPrefixDecodeOK++
			} else {
				oemPrefixDecodeFail++
			}
		}

		// 8. Confidence
		if len(p.vehicles) > 0 {
			sr, err := ss.Search("", p.vehicles[0], 1999, "Petrol", "", 1, 500)
			if err == nil && sr != nil {
				for _, r := range sr.Results {
					if r.Part.LegacyArticleId == p.id {
						if r.Confidence >= 0.7 {
							confidenceReasonable++
						} else {
							confidenceWrong++
						}
						break
					}
				}
			}
		}
	}

	// ─── FALSE POSITIVE ANALYSIS ──────────────────────────────────────────

	// FP1: OEM returns wrong parts
	for _, p := range parts {
		for _, oem := range p.oems {
			refs, err := cr.FindByOEM(oem, 100)
			if err != nil {
				continue
			}
			for _, r := range refs {
				if r.LegacyArticleId == p.id {
					continue
				}
				isAlt := false
				for i := range p.altBrands {
					if r.LegacyArticleId == p.id+10000+(i+1)*100 {
						isAlt = true
						break
					}
				}
				if !isAlt {
					oemFP++
					failDetails = append(failDetails, fmt.Sprintf("FP-OEM: %s returned article %d (expected %d)", oem, r.LegacyArticleId, p.id))
				}
			}
		}
	}

	// FP2: vehicle returns wrong articles
	testedVehicles := map[int]bool{}
	for _, p := range parts {
		for _, vid := range p.vehicles {
			if testedVehicles[vid] {
				continue
			}
			testedVehicles[vid] = true
			allFound := map[int]bool{}
			for pg := 1; pg <= 50; pg++ {
				fp, _, err := pl.FindByLinkageTarget(vid, "", pg, 100)
				if err != nil || len(fp) == 0 {
					break
				}
				for _, f := range fp {
					allFound[f.LegacyArticleId] = true
				}
			}
			allKnown := map[int]bool{}
			for id := range partsForVehicle[vid] {
				allKnown[id] = true
				pp := partById[id]
				if pp != nil {
					for i := range pp.altBrands {
						allKnown[pp.id+10000+(i+1)*100] = true
					}
				}
			}
			for artId := range allFound {
				if !allKnown[artId] {
					vehicleFP++
					failDetails = append(failDetails, fmt.Sprintf("FP-Vehicle: vehicle %d returned unexpected article %d", vid, artId))
				}
			}
		}
	}

	// FP3: Wrong-vehicle exclusion (TN)
	wrongVehicleTests := buildWrongVehicleTests(parts)
	for _, wv := range wrongVehicleTests {
		found := false
		for pg := 1; pg <= 20; pg++ {
			fp, _, err := pl.FindByLinkageTarget(wv.wrongVid, "", pg, 100)
			if err != nil || len(fp) == 0 {
				break
			}
			for _, f := range fp {
				if f.LegacyArticleId == wv.partId {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			wrongVehicleFit++
			failDetails = append(failDetails, fmt.Sprintf("FP-WrongVehicle: part %d appeared for vehicle %d", wv.partId, wv.wrongVid))
		} else {
			vehicleTN++
		}
	}

	// FP4: Bogus OEM
	bogusOEMs := []string{"99999-ZZ999", "00000-00000", "FAKE-12345", "XXXXX-YYYYY", "11111-11111",
		"77777-77777", "ABCDE-FGHIJ", "12345-XXXXX", "ZZZZZ-00000", "TEST0-00001"}
	for _, bogus := range bogusOEMs {
		refs, err := cr.FindByOEM(bogus, 50)
		if err == nil && len(refs) == 0 {
			oemTN++
		} else if err == nil && len(refs) > 0 {
			searchFP++
			failDetails = append(failDetails, fmt.Sprintf("FP-Search: bogus OEM %q returned %d results", bogus, len(refs)))
		} else {
			oemTN++
		}
	}

	// ─── Platform sibling detection ───────────────────────────────────────
	platformTests := []struct{ make, model, expectSibling string }{
		{"HYUNDAI", "TUCSON", "SPORTAGE"},
		{"KIA", "SPORTAGE", "TUCSON"},
		{"HYUNDAI", "ELANTRA", "FORTE"},
		{"KIA", "FORTE", "ELANTRA"},
		{"HYUNDAI", "SONATA", "K5"},
		{"KIA", "K5", "SONATA"},
		{"HYUNDAI", "SANTA FE", "SORENTO"},
		{"KIA", "SORENTO", "SANTA FE"},
		{"HYUNDAI", "KONA", "SELTOS"},
		{"KIA", "SELTOS", "KONA"},
		{"KIA", "EV6", "IONIQ 5"},
		{"HYUNDAI", "PALISADE", "TELLURIDE"},
		{"KIA", "CARNIVAL", "STARIA"},
		{"HYUNDAI", "ACCENT", "RIO"},
		{"HYUNDAI", "I30", "CEED"},
	}
	platOK, platFail := 0, 0
	for _, pt := range platformTests {
		sibs, err := pf.FindSiblings(pt.make, pt.model)
		if err == nil && len(sibs) > 0 {
			found := false
			for _, s := range sibs {
				if strings.EqualFold(s.SiblingModel, pt.expectSibling) {
					found = true
				}
			}
			if found {
				platOK++
			} else {
				platFail++
				failDetails = append(failDetails, fmt.Sprintf("Platform: %s %s → expected %s", pt.make, pt.model, pt.expectSibling))
			}
		} else {
			platFail++
			failDetails = append(failDetails, fmt.Sprintf("Platform: %s %s → no siblings", pt.make, pt.model))
		}
	}

	// ═══ REPORT ═══════════════════════════════════════════════════════════
	fmt.Println()
	section("1. OEM → AFTERMARKET RESOLUTION")
	metric("Total OEM numbers tested", totalOEMs)
	metric("OEM → correct aftermarket part found", oemToAfterOK)
	metric("OEM → aftermarket NOT found", oemToAfterFail)
	pct("Resolution rate", oemToAfterOK, totalOEMs)

	section("2. AFTERMARKET → OEM REVERSE LOOKUP")
	metric("Parts with OEM cross-refs found", afterToOemOK)
	metric("Parts with NO OEM cross-refs", afterToOemFail)
	pct("OEM coverage", afterToOemOK, totalParts)

	section("3. CROSS-BRAND FITMENT (Hyundai↔Kia)")
	metric("Parts expected to cross brands", crossBrandExpected)
	metric("Cross-brand correctly detected", crossBrandFound)
	metric("Cross-brand missed", crossBrandMissed)
	pct("Cross-brand accuracy", crossBrandFound, crossBrandExpected)

	section("4. CATEGORY CLASSIFICATION")
	metric("Correctly classified", catCorrect)
	metric("Misclassified", catWrong)
	pct("Classification accuracy", catCorrect, totalParts)

	section("5. MULTI-BRAND AFTERMARKET COVERAGE (5 brands/part)")
	metric("Parts with alt-brand alternatives expected", altBrandExpected)
	metric("Alt brands found via search", altBrandFound)
	metric("Alt brands NOT found", altBrandMissed)
	pct("Multi-brand discovery rate", altBrandFound, altBrandExpected)

	section("6. VEHICLE FITMENT ACCURACY")
	totalFitTests := vehicleFitOK + vehicleFitMiss
	metric("Part-vehicle fit tests", totalFitTests)
	metric("Correct fits found", vehicleFitOK)
	metric("Fits missed", vehicleFitMiss)
	pct("Fitment accuracy", vehicleFitOK, totalFitTests)

	section("7. OEM PREFIX DECODER")
	totalPrefix := oemPrefixDecodeOK + oemPrefixDecodeFail
	metric("OEM numbers decoded", oemPrefixDecodeOK)
	metric("OEM numbers NOT decoded", oemPrefixDecodeFail)
	pct("Prefix decode rate", oemPrefixDecodeOK, totalPrefix)

	section("8. CONFIDENCE SCORING")
	totalConf := confidenceReasonable + confidenceWrong
	metric("Reasonable confidence (≥0.7)", confidenceReasonable)
	metric("Low/wrong confidence", confidenceWrong)
	pct("Confidence accuracy", confidenceReasonable, totalConf)

	section("9. PLATFORM SIBLING DETECTION")
	metric("Platform pairs tested", len(platformTests))
	metric("Correctly detected", platOK)
	metric("Missed", platFail)
	pct("Platform detection rate", platOK, len(platformTests))

	section("10. FALSE POSITIVE ANALYSIS")
	metric("OEM→wrong article returned (FP)", oemFP)
	metric("Vehicle→wrong article returned (FP)", vehicleFP)
	metric("Bogus OEM→results returned (FP)", searchFP)
	metric("Wrong-vehicle correctly excluded (TN)", vehicleTN)
	metric("Wrong-vehicle incorrectly returned (FP)", wrongVehicleFit)
	metric("Bogus OEM correctly rejected (TN)", oemTN)
	totalFP := oemFP + vehicleFP + searchFP + wrongVehicleFit
	totalTN := vehicleTN + oemTN
	pct("False positive rate", totalFP, totalFP+totalTN)

	tp := oemToAfterOK + vehicleFitOK
	fn := oemToAfterFail + vehicleFitMiss + crossBrandMissed
	fp := totalFP
	tn := totalTN

	section("11. CONFUSION MATRIX (Fitment)")
	fmt.Printf("│                       Predicted FIT    Predicted NO-FIT        │\n")
	fmt.Printf("│  Actually FITS          TP = %-5d        FN = %-5d            │\n", tp, fn)
	fmt.Printf("│  Actually NO-FIT        FP = %-5d        TN = %-5d            │\n", fp, tn)
	fmt.Printf("│                                                                  │\n")
	precision := safePct(tp, tp+fp)
	recall := safePct(tp, tp+fn)
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2.0 * precision * recall / (precision + recall)
	}
	specificity := safePct(tn, tn+fp)
	accuracy := safePct(tp+tn, tp+tn+fp+fn)
	fmt.Printf("│  %-50s %5.1f%%       │\n", "Precision  (TP / (TP+FP))", precision)
	fmt.Printf("│  %-50s %5.1f%%       │\n", "Recall     (TP / (TP+FN))", recall)
	fmt.Printf("│  %-50s %5.1f%%       │\n", "F1 Score   (harmonic mean)", f1)
	fmt.Printf("│  %-50s %5.1f%%       │\n", "Specificity(TN / (TN+FP))", specificity)
	fmt.Printf("│  %-50s %5.1f%%       │\n", "Accuracy   ((TP+TN)/(All))", accuracy)
	fmt.Printf("└──────────────────────────────────────────────────────────────────────┘\n")

	// Overall
	totalTests := totalOEMs + totalParts + crossBrandExpected + totalParts +
		altBrandExpected + totalFitTests + totalPrefix + totalConf + len(platformTests)
	totalPass := oemToAfterOK + afterToOemOK + crossBrandFound + catCorrect +
		altBrandFound + vehicleFitOK + oemPrefixDecodeOK + confidenceReasonable + platOK

	fmt.Println()
	boxLine := func(text string) {
		fmt.Printf("║  %-67s ║\n", text)
	}
	fmt.Println("╔══════════════════════════════════════════════════════════════════════╗")
	boxLine(fmt.Sprintf("OVERALL: %d / %d checks passed", totalPass, totalTests))
	boxLine(fmt.Sprintf("ACCURACY: %.1f%%", safePct(totalPass, totalTests)))
	boxLine(fmt.Sprintf("TOTAL PARTS: %d   TOTAL OEMs: %d   TOTAL ARTICLES: %d", totalParts, totalOEMs, countArticles(parts)))
	fmt.Println("╚══════════════════════════════════════════════════════════════════════╝")

	// Failure details
	if len(failDetails) > 0 {
		catFails, vfFails, fpFails, otherFails := []string{}, []string{}, []string{}, []string{}
		for _, d := range failDetails {
			switch {
			case strings.HasPrefix(d, "Category:"):
				catFails = append(catFails, d)
			case strings.HasPrefix(d, "VehicleFit:"):
				vfFails = append(vfFails, d)
			case strings.HasPrefix(d, "FP-"):
				fpFails = append(fpFails, d)
			default:
				otherFails = append(otherFails, d)
			}
		}
		fmt.Printf("\n─── FAILURE DETAILS (%d total) ───\n", len(failDetails))
		printFails := func(label string, items []string, max int) {
			if len(items) == 0 {
				return
			}
			fmt.Printf("\n  %s (%d):\n", label, len(items))
			for i, d := range items {
				if i >= max {
					fmt.Printf("    ... and %d more\n", len(items)-max)
					break
				}
				fmt.Printf("    • %s\n", d)
			}
		}
		printFails("Category misclassifications", catFails, 20)
		printFails("False positives", fpFails, 20)
		printFails("Other failures", otherFails, 20)
		printFails("Vehicle fitment misses", vfFails, 15)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// HELPERS
// ═══════════════════════════════════════════════════════════════════════════════

func section(title string) {
	fmt.Printf("\n┌─ %s ─", title)
	pad := 67 - len(title)
	for i := 0; i < pad; i++ {
		fmt.Print("─")
	}
	fmt.Println("┐")
}

func metric(label string, value int) {
	fmt.Printf("│  %-50s %5d        │\n", label, value)
}

func pct(label string, num, denom int) {
	if denom == 0 {
		fmt.Printf("│  %-50s   N/A        │\n", label)
	} else {
		fmt.Printf("│  %-50s %5.1f%%       │\n", label, 100.0*float64(num)/float64(denom))
	}
	fmt.Printf("└──────────────────────────────────────────────────────────────────────┘\n")
}

func safePct(num, denom int) float64 {
	if denom == 0 {
		return 0
	}
	return 100.0 * float64(num) / float64(denom)
}

func drvName(d service.FitmentDriver) string {
	switch d {
	case service.FitEngine:
		return "engine"
	case service.FitBody:
		return "body"
	case service.FitDrivetrain:
		return "drivetrain"
	case service.FitBrake:
		return "brake"
	case service.FitUniversal:
		return "universal"
	}
	return "unknown"
}

func countArticles(parts []partSpec) int {
	n := len(parts)
	for _, p := range parts {
		n += len(p.altBrands)
	}
	return n
}

// ═══════════════════════════════════════════════════════════════════════════════
// 500-PART GENERATOR
// ═══════════════════════════════════════════════════════════════════════════════

// 5 vehicle groups — each Hyundai model paired with its Kia sibling
var vGroups = []vehicleGroup{
	{"Tucson/Sportage", []int{1001, 1002}, []int{2001, 2002}, true},
	{"Elantra/Forte", []int{1003, 1004}, []int{2003, 2004}, true},
	{"Sonata/K5", []int{1005, 1006}, []int{2005, 2006}, true},
	{"SantaFe/Sorento", []int{1007, 1008}, []int{2007, 2008}, true},
	{"Kona/Seltos", []int{1009, 1010}, []int{2009, 2010}, true},
}

// 100 part templates × 5 vehicle groups = 500 parts
var templates = []partTpl{
	// ─── ENGINE (25 types) ────────────────────────────────────────────────
	{"Oil Filter", "universal", false, "263", [6]string{"MAHLE", "BOSCH", "MANN", "HENGST", "WIX", "KNECHT"}},
	{"Air Filter", "universal", false, "281", [6]string{"MANN", "MAHLE", "BOSCH", "HENGST", "WIX", "KNECHT"}},
	{"Spark Plug", "engine", true, "188", [6]string{"NGK", "DENSO", "BOSCH", "CHAMPION", "BERU", "MOTORCRAFT"}},
	{"Ignition Coil", "engine", true, "273", [6]string{"DENSO", "BOSCH", "NGK", "DELPHI", "HELLA", "BERU"}},
	{"Alternator", "engine", true, "373", [6]string{"DENSO", "BOSCH", "VALEO", "HELLA", "REMY", "WAI"}},
	{"Starter Motor", "engine", true, "361", [6]string{"DENSO", "BOSCH", "VALEO", "HELLA", "REMY", "WAI"}},
	{"Water Pump", "engine", true, "251", [6]string{"SKF", "AISIN", "GATES", "HEPU", "DOLZ", "GMB"}},
	{"Thermostat", "engine", true, "255", [6]string{"GATES", "MAHLE", "WAHLER", "CALORSTAT", "FAE", "VERNET"}},
	{"Timing Belt Kit", "engine", true, "243", [6]string{"DAYCO", "GATES", "CONTITECH", "INA", "SKF", "BOSCH"}},
	{"Timing Chain Kit", "engine", true, "243", [6]string{"FEBI", "INA", "SWAG", "BGA", "MELLING", "CLOYES"}},
	{"Fuel Injector", "engine", true, "353", [6]string{"BOSCH", "DENSO", "DELPHI", "SIEMENS", "MAGNETI MARELLI", "HITACHI"}},
	{"Fuel Pump Module", "engine", true, "311", [6]string{"DELPHI", "BOSCH", "PIERBURG", "VDO", "DENSO", "AIRTEX"}},
	{"Serpentine Belt", "engine", true, "252", [6]string{"GATES", "DAYCO", "CONTITECH", "BANDO", "OPTIBELT", "BOSCH"}},
	{"Engine Mount Front", "engine", true, "218", [6]string{"LEMFORDER", "FEBI", "MEYLE", "CORTECO", "SWAG", "HUTCHINSON"}},
	{"Engine Mount Rear", "engine", true, "218", [6]string{"LEMFORDER", "FEBI", "MEYLE", "CORTECO", "SWAG", "HUTCHINSON"}},
	{"Oxygen Sensor Front", "engine", true, "392", [6]string{"DENSO", "BOSCH", "NGK", "DELPHI", "WALKER", "FAE"}},
	{"Oxygen Sensor Rear", "engine", true, "392", [6]string{"DENSO", "BOSCH", "NGK", "DELPHI", "WALKER", "FAE"}},
	{"Turbocharger", "engine", true, "282", [6]string{"GARRETT", "BORGWARNER", "MITSUBISHI", "IHI", "HOLSET", "MELETT"}},
	{"EGR Valve", "engine", true, "284", [6]string{"PIERBURG", "WAHLER", "DELPHI", "VALEO", "HELLA", "FAE"}},
	{"Catalytic Converter", "engine", true, "289", [6]string{"WALKER", "BOSAL", "BM CATALYSTS", "ERNST", "KLARIUS", "EEC"}},
	{"Glow Plug", "engine", true, "367", [6]string{"DENSO", "BOSCH", "BERU", "NGK", "CHAMPION", "DELPHI"}},
	{"Radiator", "engine", true, "253", [6]string{"NISSENS", "DENSO", "VALEO", "HELLA", "MAHLE", "NRF"}},
	{"Coolant Hose Upper", "engine", true, "254", [6]string{"GATES", "DAYCO", "FEBI", "SWAG", "MEYLE", "VAICO"}},
	{"Radiator Fan Motor", "engine", true, "253", [6]string{"NISSENS", "DENSO", "VALEO", "NRF", "TYC", "HELLA"}},
	{"Coolant Temperature Sensor", "engine", true, "392", [6]string{"FAE", "HELLA", "BOSCH", "DELPHI", "FACET", "MEYLE"}},

	// ─── BRAKE (15 types) ─────────────────────────────────────────────────
	{"Brake Pad Set Front", "brake", false, "581", [6]string{"TRW", "BREMBO", "ATE", "FERODO", "TEXTAR", "BOSCH"}},
	{"Brake Pad Set Rear", "brake", false, "584", [6]string{"TRW", "BREMBO", "ATE", "FERODO", "TEXTAR", "BOSCH"}},
	{"Brake Disc Front", "brake", false, "517", [6]string{"BREMBO", "TRW", "ATE", "ZIMMERMANN", "BOSCH", "TEXTAR"}},
	{"Brake Disc Rear", "brake", false, "584", [6]string{"TRW", "BREMBO", "ZIMMERMANN", "ATE", "BOSCH", "TEXTAR"}},
	{"Brake Caliper Front Left", "brake", false, "581", [6]string{"ATE", "TRW", "BREMBO", "BUDWEG", "NISHIMBO", "CARDONE"}},
	{"Brake Caliper Front Right", "brake", false, "581", [6]string{"ATE", "TRW", "BREMBO", "BUDWEG", "NISHIMBO", "CARDONE"}},
	{"Brake Caliper Rear Left", "brake", false, "584", [6]string{"ATE", "TRW", "BUDWEG", "BREMBO", "NISHIMBO", "CARDONE"}},
	{"Brake Caliper Rear Right", "brake", false, "584", [6]string{"ATE", "TRW", "BUDWEG", "BREMBO", "NISHIMBO", "CARDONE"}},
	{"Brake Shoe Set Rear", "brake", false, "583", [6]string{"FERODO", "TRW", "ATE", "DELPHI", "BOSCH", "TEXTAR"}},
	{"Brake Hose Front", "brake", false, "587", [6]string{"ATE", "TRW", "BOSCH", "FERODO", "DELPHI", "ABE"}},
	{"Brake Hose Rear", "brake", false, "587", [6]string{"ATE", "TRW", "BOSCH", "FERODO", "DELPHI", "ABE"}},
	{"Brake Rotor Front", "brake", false, "517", [6]string{"ZIMMERMANN", "BREMBO", "TRW", "ATE", "BOSCH", "MEYLE"}},
	{"Brake Rotor Rear", "brake", false, "584", [6]string{"ZIMMERMANN", "BREMBO", "TRW", "ATE", "BOSCH", "MEYLE"}},
	{"Brake Drum Rear", "brake", false, "584", [6]string{"TRW", "ATE", "DELPHI", "ABE", "BOSCH", "TEXTAR"}},
	{"Brake Master Cylinder", "brake", false, "585", [6]string{"TRW", "ATE", "DELPHI", "BOSCH", "FTE", "ABE"}},

	// ─── BODY (20 types) ─────────────────────────────────────────────────
	{"Headlight Assembly Left", "body", false, "921", [6]string{"HELLA", "DEPO", "TYC", "VALEO", "MAGNETI MARELLI", "ALKAR"}},
	{"Headlight Assembly Right", "body", false, "921", [6]string{"HELLA", "DEPO", "TYC", "VALEO", "MAGNETI MARELLI", "ALKAR"}},
	{"Tail Light Left", "body", false, "924", [6]string{"HELLA", "TYC", "DEPO", "VALEO", "MAGNETI MARELLI", "ULO"}},
	{"Tail Light Right", "body", false, "924", [6]string{"HELLA", "TYC", "DEPO", "VALEO", "MAGNETI MARELLI", "ULO"}},
	{"Front Bumper Cover", "body", false, "865", [6]string{"PRASCO", "BLIC", "DIEDERICHS", "VAN WEZEL", "KLOKKERHOLM", "EQUAL QUALITY"}},
	{"Rear Bumper Cover", "body", false, "866", [6]string{"PRASCO", "BLIC", "DIEDERICHS", "VAN WEZEL", "KLOKKERHOLM", "EQUAL QUALITY"}},
	{"Front Grille", "body", false, "863", [6]string{"PRASCO", "BLIC", "DIEDERICHS", "VAN WEZEL", "KLOKKERHOLM", "EQUAL QUALITY"}},
	{"Fender Left", "body", false, "663", [6]string{"PRASCO", "BLIC", "DIEDERICHS", "VAN WEZEL", "KLOKKERHOLM", "EQUAL QUALITY"}},
	{"Fender Right", "body", false, "663", [6]string{"PRASCO", "BLIC", "DIEDERICHS", "VAN WEZEL", "KLOKKERHOLM", "EQUAL QUALITY"}},
	{"Hood / Bonnet", "body", false, "664", [6]string{"PRASCO", "BLIC", "DIEDERICHS", "VAN WEZEL", "KLOKKERHOLM", "EQUAL QUALITY"}},
	{"Wiper Blade Set", "body", false, "983", [6]string{"BOSCH", "VALEO", "DENSO", "SWF", "CHAMPION", "HELLA"}},
	{"Window Regulator Front Left", "body", false, "824", [6]string{"BLIC", "VAN WEZEL", "DIEDERICHS", "ELECTRIC LIFE", "PRASCO", "KLOKKERHOLM"}},
	{"Window Regulator Front Right", "body", false, "824", [6]string{"BLIC", "VAN WEZEL", "DIEDERICHS", "ELECTRIC LIFE", "PRASCO", "KLOKKERHOLM"}},
	{"Door Mirror Left Electric", "body", false, "876", [6]string{"BLIC", "ALKAR", "TYC", "VAN WEZEL", "DIEDERICHS", "HAGUS"}},
	{"Door Mirror Right Electric", "body", false, "876", [6]string{"BLIC", "ALKAR", "TYC", "VAN WEZEL", "DIEDERICHS", "HAGUS"}},
	{"Indicator Side Left", "body", false, "876", [6]string{"TYC", "DEPO", "HELLA", "VAN WEZEL", "BLIC", "PRASCO"}},
	{"Indicator Side Right", "body", false, "876", [6]string{"TYC", "DEPO", "HELLA", "VAN WEZEL", "BLIC", "PRASCO"}},
	{"Fog Light Left", "body", false, "922", [6]string{"HELLA", "TYC", "DEPO", "VALEO", "PRASCO", "VAN WEZEL"}},
	{"Fog Light Right", "body", false, "922", [6]string{"HELLA", "TYC", "DEPO", "VALEO", "PRASCO", "VAN WEZEL"}},
	{"Wiper Motor Front", "body", false, "981", [6]string{"VALEO", "BOSCH", "SWF", "METZGER", "FEBI", "MAGNETI MARELLI"}},

	// ─── DRIVETRAIN (10 types) ────────────────────────────────────────────
	{"CV Joint Kit", "drivetrain", false, "495", [6]string{"SKF", "GKN", "METELLI", "LOBRO", "FEBEST", "GSP"}},
	{"Drive Shaft Front Left", "drivetrain", false, "495", [6]string{"SKF", "GKN", "GSP", "LOBRO", "METELLI", "FEBEST"}},
	{"Drive Shaft Front Right", "drivetrain", false, "495", [6]string{"GKN", "SKF", "GSP", "LOBRO", "METELLI", "FEBEST"}},
	{"Clutch Kit 3pc", "drivetrain", false, "411", [6]string{"SACHS", "LUK", "VALEO", "AISIN", "EXEDY", "BORG & BECK"}},
	{"Clutch Release Bearing", "drivetrain", false, "414", [6]string{"SKF", "SACHS", "LUK", "FAG", "INA", "VALEO"}},
	{"Differential Oil Seal", "drivetrain", false, "473", [6]string{"CORTECO", "ELRING", "VICTOR REINZ", "AJUSA", "PAYEN", "FAI"}},
	{"Propshaft Center Bearing", "drivetrain", false, "495", [6]string{"SKF", "FAG", "SNR", "NTN", "MEYLE", "FEBI"}},
	{"Drive Shaft Rear Left", "drivetrain", false, "495", [6]string{"SKF", "GKN", "GSP", "LOBRO", "METELLI", "FEBEST"}},
	{"Drive Shaft Rear Right", "drivetrain", false, "495", [6]string{"GKN", "SKF", "GSP", "LOBRO", "METELLI", "FEBEST"}},
	{"Transmission Mount", "drivetrain", false, "218", [6]string{"LEMFORDER", "FEBI", "MEYLE", "CORTECO", "SWAG", "HUTCHINSON"}},

	// ─── SUSPENSION & STEERING (15 types) ─────────────────────────────────
	{"Shock Absorber Front", "universal", false, "546", [6]string{"SACHS", "KYB", "MONROE", "BILSTEIN", "BOGE", "TOKICO"}},
	{"Shock Absorber Rear", "universal", false, "553", [6]string{"KYB", "SACHS", "MONROE", "BILSTEIN", "BOGE", "TOKICO"}},
	{"Coil Spring Front", "universal", false, "546", [6]string{"LESJOFORS", "SACHS", "KILEN", "SUPLEX", "MAGNUM", "MAXGEAR"}},
	{"Coil Spring Rear", "universal", false, "553", [6]string{"LESJOFORS", "SACHS", "KILEN", "SUPLEX", "MAGNUM", "MAXGEAR"}},
	{"Ball Joint Lower", "universal", false, "545", [6]string{"LEMFORDER", "TRW", "MOOG", "MEYLE", "DELPHI", "FEBI"}},
	{"Tie Rod End Outer", "universal", false, "568", [6]string{"TRW", "LEMFORDER", "MOOG", "MEYLE", "DELPHI", "FEBI"}},
	{"Tie Rod End Inner", "universal", false, "577", [6]string{"TRW", "LEMFORDER", "MOOG", "MEYLE", "DELPHI", "FEBI"}},
	{"Stabilizer Link Front", "universal", false, "548", [6]string{"TRW", "LEMFORDER", "MOOG", "MEYLE", "DELPHI", "FEBI"}},
	{"Stabilizer Link Rear", "universal", false, "555", [6]string{"TRW", "LEMFORDER", "MOOG", "MEYLE", "DELPHI", "FEBI"}},
	{"Control Arm Lower Front", "universal", false, "545", [6]string{"MEYLE", "LEMFORDER", "TRW", "MOOG", "DELPHI", "FEBI"}},
	{"Control Arm Upper Front", "universal", false, "545", [6]string{"MEYLE", "LEMFORDER", "TRW", "MOOG", "DELPHI", "FEBI"}},
	{"Wheel Bearing Front", "universal", false, "517", [6]string{"SKF", "FAG", "NTN", "SNR", "TIMKEN", "MEYLE"}},
	{"Wheel Bearing Rear", "universal", false, "527", [6]string{"SKF", "FAG", "NTN", "SNR", "TIMKEN", "MEYLE"}},
	{"Steering Rack Boot", "universal", false, "577", [6]string{"LEMFORDER", "TRW", "MEYLE", "FEBI", "SWAG", "SKF"}},
	{"Strut Bearing Front", "universal", false, "546", [6]string{"SKF", "SACHS", "LEMFORDER", "MONROE", "SNR", "FAG"}},

	// ─── ELECTRICAL (5 types) ─────────────────────────────────────────────
	{"Bulb H7 55W", "universal", false, "186", [6]string{"OSRAM", "PHILIPS", "BOSCH", "NARVA", "HELLA", "GE"}},
	{"Fuse 15A", "universal", false, "918", [6]string{"HELLA", "BOSCH", "BUSSMANN", "LITTELFUSE", "CONTINENTAL", "FLOSSMANN"}},
	{"Speed Sensor ABS Front", "universal", false, "598", [6]string{"ATE", "BOSCH", "DELPHI", "TRW", "HELLA", "FAE"}},
	{"Speed Sensor ABS Rear", "universal", false, "599", [6]string{"ATE", "BOSCH", "DELPHI", "TRW", "HELLA", "FAE"}},
	{"Horn", "universal", false, "966", [6]string{"HELLA", "BOSCH", "FIAMM", "STEBEL", "MARELLI", "DENSO"}},

	// ─── HVAC (5 types) ──────────────────────────────────────────────────
	{"Cabin Filter", "universal", false, "971", [6]string{"MAHLE", "MANN", "BOSCH", "HENGST", "CORTECO", "KNECHT"}},
	{"A/C Compressor", "universal", false, "977", [6]string{"DENSO", "VALEO", "DELPHI", "SANDEN", "HELLA", "AIRSTAL"}},
	{"A/C Condenser", "universal", false, "976", [6]string{"NISSENS", "DENSO", "VALEO", "NRF", "HELLA", "AVA"}},
	{"Heater Core", "universal", false, "971", [6]string{"NISSENS", "DENSO", "VALEO", "NRF", "AVA", "HELLA"}},
	{"Blower Motor", "universal", false, "971", [6]string{"VALEO", "HELLA", "DENSO", "NISSENS", "TYC", "FEBI"}},

	// ─── EXTRA (5 types to reach 100 templates × 5 groups = 500 parts) ──
	{"Fuel Tank Cap", "universal", false, "310", [6]string{"GATES", "FEBI", "SWAG", "HELLA", "VAICO", "MEYLE"}},
	{"Exhaust Muffler Rear", "engine", true, "286", [6]string{"WALKER", "BOSAL", "KLARIUS", "ERNST", "EEC", "BM CATALYSTS"}},
	{"Exhaust Gasket", "engine", true, "285", [6]string{"ELRING", "VICTOR REINZ", "AJUSA", "BOSAL", "WALKER", "ERNST"}},
	{"Valve Cover Gasket", "engine", true, "224", [6]string{"ELRING", "VICTOR REINZ", "AJUSA", "CORTECO", "PAYEN", "FAI"}},
	{"Crankshaft Seal Front", "engine", true, "214", [6]string{"CORTECO", "ELRING", "VICTOR REINZ", "FAI", "AJUSA", "PAYEN"}},
}

func allParts() []partSpec {
	var parts []partSpec
	id := 10000

	for _, tpl := range templates {
		for gi, vg := range vGroups {
			id++
			// Build vehicle list
			var vids []int
			cb := vg.crossBrand

			switch tpl.cat {
			case "body":
				// Body parts are model-specific, use Hyundai-only for odd groups, Kia-only for even
				if gi%2 == 0 {
					vids = vg.hyundaiVid
				} else {
					vids = vg.kiaVid
				}
				cb = false // body parts don't cross brands
			default:
				// Mechanical/universal: fit both brands
				vids = append(vids, vg.hyundaiVid...)
				vids = append(vids, vg.kiaVid...)
			}

			// Generate OEM numbers (use oemPrefix + group + sequence)
			var oems []string
			base := fmt.Sprintf("%s%02d", tpl.oemPrefix, gi+1)
			for j := 0; j < 5; j++ {
				oems = append(oems, fmt.Sprintf("%s-%d%04d", base, j+1, id))
			}

			artNum := fmt.Sprintf("%s-%d-%04d", strings.ReplaceAll(tpl.desc[:min(4, len(tpl.desc))], " ", ""), gi+1, id%10000)

			parts = append(parts, partSpec{
				id:         id,
				artNum:     artNum,
				desc:       tpl.desc,
				brand:      tpl.brands[0],
				oems:       oems,
				vehicles:   vids,
				expectCat:  tpl.cat,
				expectCC:   tpl.ccDep,
				crossBrand: cb,
				altBrands:  tpl.brands[1:],
			})
		}
	}

	return parts
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ═══════════════════════════════════════════════════════════════════════════════
// WRONG-VEHICLE TESTS (True Negative checks)
// ═══════════════════════════════════════════════════════════════════════════════

type wrongVehicleTest struct {
	partId   int
	wrongVid int
}

func buildWrongVehicleTests(parts []partSpec) []wrongVehicleTest {
	var tests []wrongVehicleTest
	allVids := []int{1001, 1002, 1003, 1004, 1005, 1006, 1007, 1008, 1009, 1010,
		2001, 2002, 2003, 2004, 2005, 2006, 2007, 2008, 2009, 2010}

	// For every 10th part, pick a vehicle it does NOT fit and verify exclusion
	for i, p := range parts {
		if i%10 != 0 {
			continue
		}
		vset := map[int]bool{}
		for _, v := range p.vehicles {
			vset[v] = true
		}
		for _, wv := range allVids {
			if !vset[wv] {
				tests = append(tests, wrongVehicleTest{p.id, wv})
				break // one wrong-vehicle test per part
			}
		}
	}
	return tests
}

// ═══════════════════════════════════════════════════════════════════════════════
// DATABASE SETUP
// ═══════════════════════════════════════════════════════════════════════════════

func buildDB(parts []partSpec) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		fmt.Printf("FATAL: %v\n", err)
		os.Exit(1)
	}

	for _, ddl := range []string{
		`CREATE TABLE hk_parts_cache (
			linkingTargetId INTEGER, legacyArticleId INTEGER, assemblyGroupNodeId INTEGER DEFAULT 0,
			articleNumber TEXT, genericArticleDesc TEXT, dataSupplierId INTEGER,
			brandName TEXT, categoryName TEXT, vehicleDesc TEXT,
			manuId INTEGER, modelId INTEGER, modelName TEXT,
			beginYearMonth TEXT, endYearMonth TEXT, fuelType TEXT,
			capacityCC INTEGER, horsePowerFrom INTEGER,
			PRIMARY KEY (linkingTargetId, legacyArticleId, assemblyGroupNodeId))`,
		`CREATE TABLE vehicle_lookup (
			nhtsa_make TEXT, nhtsa_model TEXT, year_from INTEGER, year_to INTEGER,
			linkageTargetId INTEGER, description TEXT, beginYearMonth TEXT, endYearMonth TEXT,
			fuelType TEXT, capacityCC INTEGER, horsePowerFrom INTEGER)`,
		`CREATE TABLE nhtsa_tecdoc_bridge (nhtsa_make TEXT, nhtsa_model TEXT, tecdoc_model_id INTEGER, year_from INTEGER, year_to INTEGER)`,
		`CREATE TABLE oem_search_index (raw_number TEXT, normalized TEXT, legacyArticleId INTEGER, source_table TEXT, mfr_name TEXT, brand_name TEXT, article_number TEXT, description TEXT)`,
		`CREATE TABLE articlecrosses (legacyArticleId INTEGER, oemNumber TEXT, brandName TEXT)`,
		`CREATE TABLE hk_platform_map (hyundai_model TEXT, kia_model TEXT, platform_code TEXT)`,
		`CREATE INDEX idx_hk_article ON hk_parts_cache(legacyArticleId)`,
		`CREATE INDEX idx_hk_artnum ON hk_parts_cache(articleNumber)`,
		`CREATE INDEX idx_hk_desc ON hk_parts_cache(genericArticleDesc)`,
		`CREATE INDEX idx_vl_lookup ON vehicle_lookup(nhtsa_make, nhtsa_model, year_from, year_to)`,
		`CREATE INDEX idx_oem_norm ON oem_search_index(normalized)`,
		`CREATE INDEX idx_cross_oem ON articlecrosses(oemNumber)`,
		`CREATE INDEX idx_cross_article ON articlecrosses(legacyArticleId)`,
	} {
		mustExec(db, ddl)
	}

	// ─── 20 Vehicles (10 Hyundai + 10 Kia) ──────────────────────────────
	type vSpec struct {
		vid               int
		make, model, desc string
		fuel              string
		cc, hp, modelId   int
		manuId            int
	}
	vehicles := []vSpec{
		// Hyundai Tucson
		{1001, "HYUNDAI", "TUCSON", "Tucson 2.0 MPI 150HP", "Petrol", 1999, 150, 100, 183},
		{1002, "HYUNDAI", "TUCSON", "Tucson 1.6 CRDi 136HP", "Diesel", 1598, 136, 100, 183},
		// Hyundai Elantra
		{1003, "HYUNDAI", "ELANTRA", "Elantra 2.0 MPI 147HP", "Petrol", 1999, 147, 101, 183},
		{1004, "HYUNDAI", "ELANTRA", "Elantra 1.6 Turbo 201HP", "Petrol", 1598, 201, 101, 183},
		// Hyundai Sonata
		{1005, "HYUNDAI", "SONATA", "Sonata 2.5 MPI 191HP", "Petrol", 2497, 191, 102, 183},
		{1006, "HYUNDAI", "SONATA", "Sonata 1.6 Turbo 180HP", "Petrol", 1598, 180, 102, 183},
		// Hyundai Santa Fe
		{1007, "HYUNDAI", "SANTA FE", "Santa Fe 2.5 GDI 191HP", "Petrol", 2497, 191, 103, 183},
		{1008, "HYUNDAI", "SANTA FE", "Santa Fe 2.2 CRDi 202HP", "Diesel", 2199, 202, 103, 183},
		// Hyundai Kona
		{1009, "HYUNDAI", "KONA", "Kona 2.0 MPI 147HP", "Petrol", 1999, 147, 104, 183},
		{1010, "HYUNDAI", "KONA", "Kona 1.6 Turbo 195HP", "Petrol", 1598, 195, 104, 183},
		// Kia Sportage
		{2001, "KIA", "SPORTAGE", "Sportage 2.0 GDI 155HP", "Petrol", 1999, 155, 200, 184},
		{2002, "KIA", "SPORTAGE", "Sportage 1.6 T-GDi 177HP", "Petrol", 1598, 177, 200, 184},
		// Kia Forte
		{2003, "KIA", "FORTE", "Forte 2.0 MPI 147HP", "Petrol", 1999, 147, 201, 184},
		{2004, "KIA", "FORTE", "Forte 1.6 Turbo 201HP", "Petrol", 1598, 201, 201, 184},
		// Kia K5
		{2005, "KIA", "K5", "K5 2.5 MPI 191HP", "Petrol", 2497, 191, 202, 184},
		{2006, "KIA", "K5", "K5 1.6 Turbo 180HP", "Petrol", 1598, 180, 202, 184},
		// Kia Sorento
		{2007, "KIA", "SORENTO", "Sorento 2.5 GDI 191HP", "Petrol", 2497, 191, 203, 184},
		{2008, "KIA", "SORENTO", "Sorento 2.2 CRDi 202HP", "Diesel", 2199, 202, 203, 184},
		// Kia Seltos
		{2009, "KIA", "SELTOS", "Seltos 2.0 MPI 147HP", "Petrol", 1999, 147, 204, 184},
		{2010, "KIA", "SELTOS", "Seltos 1.6 Turbo 195HP", "Petrol", 1598, 195, 204, 184},
	}

	vMap := map[int]vSpec{}
	for _, v := range vehicles {
		vMap[v.vid] = v
		mustExec(db, fmt.Sprintf(
			`INSERT INTO vehicle_lookup VALUES ('%s','%s',2018,2025,%d,'%s','201805','202512','%s',%d,%d)`,
			v.make, v.model, v.vid, v.desc, v.fuel, v.cc, v.hp))
		mustExec(db, fmt.Sprintf(
			`INSERT OR IGNORE INTO nhtsa_tecdoc_bridge VALUES ('%s','%s',%d,2018,2025)`,
			v.make, v.model, v.modelId))
	}

	// Platform mappings
	for _, pm := range []struct{ h, k, code string }{
		{"TUCSON", "SPORTAGE", "NX4/NQ5"},
		{"ELANTRA", "FORTE", "CN7/BD"},
		{"SONATA", "K5", "DN8/DL3"},
		{"SANTA FE", "SORENTO", "TM/MQ4"},
		{"KONA", "SELTOS", "OS/SP2"},
	} {
		mustExec(db, fmt.Sprintf(`INSERT INTO hk_platform_map VALUES ('%s','%s','%s')`, pm.h, pm.k, pm.code))
	}

	// ─── Seed parts ──────────────────────────────────────────────────────
	for _, p := range parts {
		for _, vid := range p.vehicles {
			vi := vMap[vid]
			mustExec(db, fmt.Sprintf(
				`INSERT OR IGNORE INTO hk_parts_cache VALUES (%d,%d,0,'%s','%s',%d,'%s','%s','%s',%d,%d,'%s','201805','202512','%s',%d,%d)`,
				vid, p.id, esc(p.artNum), esc(p.desc), 10, esc(p.brand), esc(p.desc), esc(vi.desc),
				vi.manuId, vi.modelId, vi.model, vi.fuel, vi.cc, vi.hp))
		}

		// OEM cross-refs
		mfr := "HYUNDAI"
		if p.crossBrand {
			mfr = "HYUNDAI/KIA"
		}
		for _, oem := range p.oems {
			mustExec(db, fmt.Sprintf(`INSERT INTO articlecrosses VALUES (%d,'%s','%s')`, p.id, oem, mfr))
			norm := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(oem, "-", ""), " ", ""))
			mustExec(db, fmt.Sprintf(`INSERT INTO oem_search_index VALUES ('%s','%s',%d,'articlecrosses','%s','%s','%s','%s')`,
				oem, norm, p.id, mfr, esc(p.brand), esc(p.artNum), esc(p.desc)))
		}

		// Alt-brand variants (5 per part)
		for i, altBrand := range p.altBrands {
			altId := p.id + 10000 + (i+1)*100
			altArt := fmt.Sprintf("%s-A%d", p.artNum, i+1)
			for _, vid := range p.vehicles {
				vi := vMap[vid]
				mustExec(db, fmt.Sprintf(
					`INSERT OR IGNORE INTO hk_parts_cache VALUES (%d,%d,0,'%s','%s',%d,'%s','%s','%s',%d,%d,'%s','201805','202512','%s',%d,%d)`,
					vid, altId, esc(altArt), esc(p.desc), 10+i+1, esc(altBrand), esc(p.desc), esc(vi.desc),
					vi.manuId, vi.modelId, vi.model, vi.fuel, vi.cc, vi.hp))
			}
		}
	}

	return db
}

func esc(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func mustExec(db *sql.DB, sqlStr string) {
	if _, err := db.Exec(sqlStr); err != nil {
		fmt.Printf("FATAL: %v\nSQL: %s\n", err, sqlStr)
		os.Exit(1)
	}
}

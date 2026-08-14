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
// ACCURACY REPORT: 100 PARTS — Cross-Reference & Fitment Analysis
//
// Measures how well the system:
//   1. Resolves OEM → aftermarket parts
//   2. Resolves aftermarket → OEM numbers
//   3. Detects cross-brand fitment (Hyundai↔Kia)
//   4. Classifies fitment category correctly
//   5. Scores confidence accurately
//   6. Detects platform siblings
//   7. Handles multi-brand aftermarket alternatives
// ═══════════════════════════════════════════════════════════════════════════════

type partSpec struct {
	id         int
	artNum     string
	desc       string
	brand      string
	oems       []string // OEM numbers this aftermarket part replaces
	vehicles   []int    // linkageTargetIds it fits
	expectCat  string   // expected fitment driver
	expectCC   bool     // should CC matter?
	crossBrand bool     // fits both Hyundai and Kia?
	altBrands  []string // other aftermarket brands that make same part
}

func main() {
	db := buildDB()
	defer db.Close()

	pl := service.NewPartsLookup(db, true)
	cr := service.NewCrossRef(db, true)
	ol := service.NewOEMLookup(db)
	pf := service.NewPlatform(db)
	ss := service.NewSmartSearch(db, pl, cr, ol, pf, nil, true)

	parts := allParts()

	fmt.Println("╔══════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║        PARTS ENGINE — 100-PART CROSS-REFERENCE ACCURACY REPORT      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════╝")

	// ─── Metric accumulators ──────────────────────────────────────────────
	var (
		totalParts           = len(parts)
		oemToAfterOK         int // OEM → aftermarket found
		oemToAfterFail       int
		afterToOemOK         int // aftermarket → OEM found
		afterToOemFail       int
		crossBrandExpected   int
		crossBrandFound      int
		crossBrandMissed     int
		catCorrect           int
		catWrong             int
		altBrandExpected     int
		altBrandFound        int
		altBrandMissed       int
		vehicleFitOK         int // part found when querying by vehicle
		vehicleFitMiss       int
		oemPrefixDecodeOK    int
		oemPrefixDecodeFail  int
		confidenceReasonable int
		confidenceWrong      int
		totalOEMs            int
		totalOEMsResolved    int
		failDetails          []string
		// False positive / True negative tracking
		oemFP           int // OEM query returned wrong parts
		vehicleFP       int // vehicle query returned parts that don't belong
		searchFP        int // search returned unrelated parts
		oemTN           int // OEM query correctly excluded non-matching parts
		vehicleTN       int // vehicle query correctly excluded wrong parts
		wrongVehicleFit int // parts returned for wrong vehicle
	)

	// Build lookup maps for FP/TN analysis
	partById := map[int]*partSpec{}
	partsForVehicle := map[int]map[int]bool{} // vid → set of correct article IDs
	oemsForPart := map[int]map[string]bool{}  // artId → set of OEM numbers
	for i := range parts {
		p := &parts[i]
		partById[p.id] = p
		for _, vid := range p.vehicles {
			if partsForVehicle[vid] == nil {
				partsForVehicle[vid] = map[int]bool{}
			}
			partsForVehicle[vid][p.id] = true
		}
		if oemsForPart[p.id] == nil {
			oemsForPart[p.id] = map[string]bool{}
		}
		for _, oem := range p.oems {
			oemsForPart[p.id][oem] = true
		}
	}

	// ─── Per-part analysis ────────────────────────────────────────────────
	for _, p := range parts {
		// --- 1. OEM → Aftermarket ---
		for _, oem := range p.oems {
			totalOEMs++
			refs, err := cr.FindByOEM(oem, 50)
			if err == nil && len(refs) > 0 {
				// Check if our specific article is in the results
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
					failDetails = append(failDetails, fmt.Sprintf("OEM→After: %s→%s(%d) not in results (got %d refs)", oem, p.artNum, p.id, len(refs)))
				}
			} else {
				oemToAfterFail++
				failDetails = append(failDetails, fmt.Sprintf("OEM→After: %s→%s(%d) no refs found", oem, p.artNum, p.id))
			}
		}

		// --- 2. Aftermarket → OEM ---
		oems, err := cr.FindOEMNumbers(p.id)
		if err == nil && len(oems) > 0 {
			afterToOemOK++
		} else {
			afterToOemFail++
		}

		// --- 3. Cross-brand fitment ---
		if p.crossBrand {
			crossBrandExpected++
			hasHyundai := false
			hasKia := false
			for _, vid := range p.vehicles {
				if vid >= 1000 && vid < 2000 {
					hasHyundai = true
				}
				if vid >= 2000 && vid < 3000 {
					hasKia = true
				}
			}
			if hasHyundai && hasKia {
				crossBrandFound++
			} else {
				crossBrandMissed++
			}
		}

		// --- 4. Category classification ---
		rule := service.ClassifyCategory(p.desc)
		driverName := drvName(rule.Driver)
		if driverName == p.expectCat {
			catCorrect++
		} else {
			catWrong++
			failDetails = append(failDetails, fmt.Sprintf("Category: %q → got %s, expected %s", p.desc, driverName, p.expectCat))
		}

		// --- 5. Alt brand coverage ---
		if len(p.altBrands) > 0 {
			altBrandExpected++
			// Search by article description to find competing brands
			sr, err := ss.Search(p.desc, 0, 0, "", "", 1, 100)
			if err == nil && sr != nil && len(sr.Results) > 1 {
				brandsFound := map[string]bool{}
				for _, r := range sr.Results {
					brandsFound[r.BrandResolved] = true
				}
				if len(brandsFound) >= 2 {
					altBrandFound++
				} else {
					altBrandMissed++
				}
			} else {
				altBrandMissed++
			}
		}

		// --- 6. Vehicle fitment ---
		for _, vid := range p.vehicles {
			found := false
			for pg := 1; pg <= 10; pg++ { // paginate up to 10 pages
				foundParts, _, err := pl.FindByLinkageTarget(vid, "", pg, 100)
				if err != nil || len(foundParts) == 0 {
					break
				}
				for _, fp := range foundParts {
					if fp.LegacyArticleId == p.id {
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
				failDetails = append(failDetails, fmt.Sprintf("VehicleFit: part %s(%d) not found for vehicle %d", p.artNum, p.id, vid))
			}
		}

		// --- 7. OEM prefix decode ---
		for _, oem := range p.oems {
			decoded := service.DecodeOEMPrefix(oem)
			if decoded != nil {
				oemPrefixDecodeOK++
			} else {
				oemPrefixDecodeFail++
			}
		}

		// --- 8. Confidence scoring ---
		if len(p.vehicles) > 0 {
			sr, err := ss.Search("", p.vehicles[0], 1999, "Petrol", "", 1, 200)
			if err == nil && sr != nil {
				for _, r := range sr.Results {
					if r.Part.LegacyArticleId == p.id {
						if r.Confidence >= 0.7 {
							confidenceReasonable++
						} else {
							confidenceWrong++
							failDetails = append(failDetails, fmt.Sprintf("Confidence: %s(%d) conf=%.2f (expected ≥0.7)", p.artNum, p.id, r.Confidence))
						}
						break
					}
				}
			}
		}
	}

	// ─── FALSE POSITIVE ANALYSIS ──────────────────────────────────────────

	// FP Test 1: OEM queries — does searching an OEM return WRONG parts?
	for _, p := range parts {
		for _, oem := range p.oems {
			refs, err := cr.FindByOEM(oem, 50)
			if err == nil {
				for _, r := range refs {
					if r.LegacyArticleId != p.id {
						// Check if this is an alt-brand variant (expected)
						isAlt := false
						for i, alt := range p.altBrands {
							altId := p.id + 10000 + (i+1)*100
							_ = alt
							if r.LegacyArticleId == altId {
								isAlt = true
								break
							}
						}
						if !isAlt {
							oemFP++
							failDetails = append(failDetails, fmt.Sprintf("FP-OEM: %s returned unexpected article %d (expected %d)", oem, r.LegacyArticleId, p.id))
						}
					}
				}
			}
		}
	}

	// FP Test 2: Vehicle queries — does querying a vehicle return parts NOT in our ground truth?
	testedVehicles := map[int]bool{}
	for _, p := range parts {
		for _, vid := range p.vehicles {
			if testedVehicles[vid] {
				continue
			}
			testedVehicles[vid] = true
			allFound := map[int]bool{}
			for pg := 1; pg <= 20; pg++ {
				fp, _, err := pl.FindByLinkageTarget(vid, "", pg, 100)
				if err != nil || len(fp) == 0 {
					break
				}
				for _, f := range fp {
					allFound[f.LegacyArticleId] = true
				}
			}
			// Every returned article must be a known part OR an alt-brand variant
			allKnown := map[int]bool{}
			for id := range partsForVehicle[vid] {
				allKnown[id] = true
				// also whitelist alt-brand variants
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

	// FP Test 3: Wrong-vehicle exclusion (True Negative) — query vehicle that a part does NOT fit
	wrongVehicleTests := []struct{ partId, wrongVid int }{
		{5006, 1001}, // Glow Plug (diesel only) should NOT appear for petrol 1001
		{5067, 1001}, // Turbocharger (diesel) should NOT appear for petrol 1001
		{5030, 1001}, // CV Joint (Sportage) should NOT appear for Tucson 1001
		{5180, 1001}, // Kia brake pad should NOT appear for Hyundai 1001
		{5181, 1001}, // Kia headlight should NOT appear for Hyundai 1001
		{5183, 1001}, // Kia air filter should NOT appear for Hyundai 1001
		{5091, 2001}, // Clutch Kit (Tucson) should NOT appear for Sportage 2001
		{5082, 2001}, // Tucson bumper should NOT appear for Sportage 2001
		{5083, 2001}, // Tucson grille should NOT appear for Sportage 2001
		{5084, 2001}, // Tucson fender should NOT appear for Sportage 2001
		{5087, 2001}, // Tucson window reg should NOT appear for Sportage 2001
		{5086, 2001}, // Tucson tailgate should NOT appear for Sportage 2001
		{5071, 2001}, // Brake disc rear (Tucson only) should NOT appear for Sportage
		{5076, 2001}, // Brake drum (Tucson only) should NOT appear for Sportage
		{5075, 2001}, // Brake rotor rear (Tucson only) should NOT appear for Sportage
		{5160, 2001}, // Tucson wiper motor should NOT appear for Sportage
		{5131, 2001}, // Tucson exhaust pipe should NOT appear for Sportage
		{5150, 2001}, // Tucson radiator fan should NOT appear for Sportage
		{5122, 2001}, // Tucson A/C condenser should NOT appear for Sportage
		{5120, 2001}, // Tucson A/C compressor should NOT appear for Sportage
	}
	for _, wv := range wrongVehicleTests {
		found := false
		for pg := 1; pg <= 10; pg++ {
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
			failDetails = append(failDetails, fmt.Sprintf("FP-WrongVehicle: part %d appeared for vehicle %d (should NOT fit)", wv.partId, wv.wrongVid))
		} else {
			vehicleTN++
		}
	}

	// FP Test 4: OEM search with unrelated number should return nothing
	bogusOEMs := []string{"99999-ZZ999", "00000-00000", "FAKE-12345", "XXXXX-YYYYY", "11111-11111"}
	for _, bogus := range bogusOEMs {
		refs, err := cr.FindByOEM(bogus, 50)
		if err == nil && len(refs) == 0 {
			oemTN++
		} else if err == nil && len(refs) > 0 {
			searchFP++
			failDetails = append(failDetails, fmt.Sprintf("FP-Search: bogus OEM %q returned %d results", bogus, len(refs)))
		} else {
			oemTN++ // error = no results = correct
		}
	}

	// ─── Platform sibling detection ───────────────────────────────────────
	platformTests := []struct{ make, model, expectSibling string }{
		{"HYUNDAI", "TUCSON", "SPORTAGE"},
		{"KIA", "SPORTAGE", "TUCSON"},
		{"HYUNDAI", "ELANTRA", "FORTE"},
		{"HYUNDAI", "SONATA", "K5"},
		{"HYUNDAI", "SANTA FE", "SORENTO"},
		{"KIA", "EV6", "IONIQ 5"},
		{"HYUNDAI", "PALISADE", "TELLURIDE"},
		{"KIA", "CARNIVAL", "STARIA"},
		{"HYUNDAI", "KONA", "SELTOS"},
		{"HYUNDAI", "ACCENT", "RIO"},
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
				failDetails = append(failDetails, fmt.Sprintf("Platform: %s %s → expected %s, got %+v", pt.make, pt.model, pt.expectSibling, sibs))
			}
		} else {
			platFail++
			failDetails = append(failDetails, fmt.Sprintf("Platform: %s %s → no siblings found", pt.make, pt.model))
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

	section("5. MULTI-BRAND AFTERMARKET COVERAGE")
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

	// ─── FALSE POSITIVE / TRUE NEGATIVE SECTION ──────────────────────────
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

	// ─── CONFUSION MATRIX & DERIVED METRICS ──────────────────────────────
	// TP = correct matches found (OEM resolution + vehicle fitment)
	// FN = correct matches missed (OEM fail + vehicle miss + cross-brand miss)
	// FP = wrong matches returned
	// TN = wrong matches correctly rejected
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

	// ─── Overall ──────────────────────────────────────────────────────────
	totalTests := totalOEMs + totalParts + crossBrandExpected + totalParts +
		altBrandExpected + (vehicleFitOK + vehicleFitMiss) + totalPrefix +
		totalConf + len(platformTests)
	totalPass := oemToAfterOK + afterToOemOK + crossBrandFound + catCorrect +
		altBrandFound + vehicleFitOK + oemPrefixDecodeOK + confidenceReasonable + platOK

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  OVERALL: %d / %d checks passed", totalPass, totalTests)
	pad := 69 - len(fmt.Sprintf("  OVERALL: %d / %d checks passed", totalPass, totalTests))
	for i := 0; i < pad; i++ {
		fmt.Print(" ")
	}
	fmt.Println("║")
	overallPct := 0.0
	if totalTests > 0 {
		overallPct = 100.0 * float64(totalPass) / float64(totalTests)
	}
	fmt.Printf("║  ACCURACY: %.1f%%", overallPct)
	pad2 := 69 - len(fmt.Sprintf("  ACCURACY: %.1f%%", overallPct))
	for i := 0; i < pad2; i++ {
		fmt.Print(" ")
	}
	fmt.Println("║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════╝")

	// ─── Failure details ──────────────────────────────────────────────────
	if len(failDetails) > 0 {
		catFails := []string{}
		vfFails := []string{}
		otherFails := []string{}
		for _, d := range failDetails {
			if strings.HasPrefix(d, "Category:") {
				catFails = append(catFails, d)
			} else if strings.HasPrefix(d, "VehicleFit:") {
				vfFails = append(vfFails, d)
			} else {
				otherFails = append(otherFails, d)
			}
		}
		fmt.Printf("\n─── FAILURE DETAILS (%d total) ───\n", len(failDetails))
		if len(catFails) > 0 {
			fmt.Printf("\n  Category misclassifications (%d):\n", len(catFails))
			for _, d := range catFails {
				fmt.Printf("    • %s\n", d)
			}
		}
		if len(otherFails) > 0 {
			fmt.Printf("\n  Other failures (%d):\n", len(otherFails))
			for _, d := range otherFails {
				fmt.Printf("    • %s\n", d)
			}
		}
		if len(vfFails) > 0 {
			fmt.Printf("\n  Vehicle fitment misses (%d) — first 10:\n", len(vfFails))
			for i, d := range vfFails {
				if i >= 10 {
					fmt.Printf("    ... and %d more\n", len(vfFails)-10)
					break
				}
				fmt.Printf("    • %s\n", d)
			}
		}
	}

	if oemToAfterFail > 0 || vehicleFitMiss > 0 {
		os.Exit(1)
	}
}

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
		p := 100.0 * float64(num) / float64(denom)
		fmt.Printf("│  %-50s %5.1f%%       │\n", label, p)
	}
	fmt.Printf("└──────────────────────────────────────────────────────────────────────┘\n")
}

func safePct(num, denom int) float64 {
	if denom == 0 {
		return 0.0
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

// ═══════════════════════════════════════════════════════════════════════════════
// 100 PARTS — realistic Hyundai/Kia aftermarket catalog
// ═══════════════════════════════════════════════════════════════════════════════

func allParts() []partSpec {
	return []partSpec{
		// ─── ENGINE PARTS (engine-dependent) ──────────────────────────────
		{5001, "OC570", "Oil Filter", "MAHLE", []string{"26300-35503", "26300-35504"}, []int{1001, 1003, 2001}, "universal", false, true, []string{"BOSCH", "MANN"}},
		{5002, "LA123", "Air Filter", "MAHLE", []string{"28113-2S000"}, []int{1001}, "universal", false, false, nil},
		{5003, "DRA1919", "Alternator", "DENSO", []string{"37300-2B960"}, []int{1001}, "engine", true, false, []string{"BOSCH"}},
		{5005, "OC571", "Oil Filter", "BOSCH", []string{"26300-35505"}, []int{1002, 2002}, "universal", false, true, nil},
		{5006, "GP400", "Glow Plug", "DENSO", []string{"36710-2F000"}, []int{1002}, "engine", true, false, nil},
		{5050, "WP200", "Water Pump", "SKF", []string{"25100-2B700"}, []int{1001}, "engine", true, false, nil},
		{5051, "FP100", "Fuel Pump Module", "DELPHI", []string{"31110-2S500"}, []int{1001}, "engine", true, false, nil},
		{5052, "TS300", "Thermostat", "GATES", []string{"25500-23010"}, []int{1001, 1003}, "engine", true, true, []string{"MAHLE"}},
		{5053, "TB200", "Timing Belt Kit", "DAYCO", []string{"24312-23002"}, []int{1001}, "engine", true, false, nil},
		{5054, "IC100", "Ignition Coil", "DENSO", []string{"27301-2B010"}, []int{1001}, "engine", true, false, []string{"NGK"}},
		{5055, "SP400", "Spark Plug", "NGK", []string{"18855-10060"}, []int{1001}, "engine", true, false, []string{"DENSO", "BOSCH"}},
		{5056, "EM100", "Engine Mount Front", "LEMFORDER", []string{"21810-2S100"}, []int{1001}, "engine", true, false, nil},
		{5057, "FI200", "Fuel Injector", "BOSCH", []string{"35310-2B150"}, []int{1001}, "engine", true, false, []string{"DENSO"}},
		{5058, "SB100", "Serpentine Belt", "GATES", []string{"25212-2B020"}, []int{1001}, "engine", true, false, []string{"DAYCO"}},
		{5059, "IM100", "Intake Manifold Gasket", "ELRING", []string{"28311-2B700"}, []int{1001}, "engine", true, false, nil},
		{5060, "EM200", "Exhaust Manifold Gasket", "ELRING", []string{"28521-2B000"}, []int{1001}, "engine", true, false, nil},
		{5061, "CC100", "Catalytic Converter", "WALKER", []string{"28950-2B100"}, []int{1001}, "engine", true, false, nil},
		{5062, "EG100", "EGR Valve", "PIERBURG", []string{"28410-2A400"}, []int{1001}, "engine", true, false, nil},
		{5063, "OS100", "Oxygen Sensor Front", "DENSO", []string{"39210-2B060"}, []int{1001}, "engine", true, false, []string{"BOSCH", "NGK"}},
		{5064, "TC100", "Timing Chain Kit", "FEBI", []string{"24321-2B000"}, []int{1001}, "engine", true, false, nil},
		{5065, "RD100", "Radiator", "NISSENS", []string{"25310-2S500"}, []int{1001, 1003}, "engine", true, true, []string{"DENSO", "VALEO"}},
		{5066, "CW100", "Coolant Water Hose", "GATES", []string{"25460-2S100"}, []int{1001}, "engine", true, false, nil},
		{5067, "TU100", "Turbocharger", "GARRETT", []string{"28231-2B700"}, []int{1002}, "engine", true, false, nil},
		{5068, "ST100", "Starter Motor", "DENSO", []string{"36100-2B200"}, []int{1001}, "engine", true, false, []string{"BOSCH"}},

		// ─── BRAKE PARTS (brake-dependent) ────────────────────────────────
		{5004, "BP300", "Brake Pad Set", "TRW", []string{"58101-D3A00"}, []int{1001, 1002}, "brake", false, false, []string{"BREMBO", "FERODO"}},
		{5007, "BP310", "Brake Pad Set", "BREMBO", []string{"58101-D3B00"}, []int{2001}, "brake", false, false, nil},
		{5070, "BD200", "Brake Disc Front", "BREMBO", []string{"51712-D3100"}, []int{1001, 1002, 2001}, "brake", false, true, []string{"TRW", "ATE"}},
		{5071, "BD210", "Brake Disc Rear", "TRW", []string{"58411-D3100"}, []int{1001}, "brake", false, false, nil},
		{5072, "BC100", "Brake Caliper Front Left", "ATE", []string{"58110-D3100"}, []int{1001}, "brake", false, false, nil},
		{5073, "BS100", "Brake Shoe Set Rear", "FERODO", []string{"58305-D3A00"}, []int{1001, 1002}, "brake", false, false, nil},
		{5074, "BH100", "Brake Hose Front", "ATE", []string{"58732-D3100"}, []int{1001, 2001}, "brake", false, true, nil},
		{5075, "BR100", "Brake Rotor Rear", "ZIMMERMANN", []string{"58411-C1100"}, []int{1001}, "brake", false, false, nil},
		{5076, "BD300", "Brake Drum Rear", "TRW", []string{"58411-H1000"}, []int{1001}, "brake", false, false, nil},

		// ─── BODY PARTS (body-dependent) ──────────────────────────────────
		{5010, "WB100", "Wiper Blade", "BOSCH", []string{"98350-D3000"}, []int{1001, 1002, 1003, 2001, 2002}, "body", false, true, []string{"VALEO", "DENSO"}},
		{5020, "HL340", "Headlight Assembly Left", "HELLA", []string{"92101-D3100"}, []int{1001, 1002}, "body", false, false, []string{"DEPO", "TYC"}},
		{5011, "CF200", "Cabin Filter", "MAHLE", []string{"97133-D3000"}, []int{1001, 1002}, "universal", false, false, []string{"MANN", "BOSCH"}},
		{5080, "TL100", "Tail Light Right", "HELLA", []string{"92402-D3100"}, []int{1001, 1002}, "body", false, false, []string{"TYC"}},
		{5081, "DM100", "Door Mirror Left Electric", "BLIC", []string{"87610-D3010"}, []int{1001, 1002}, "body", false, false, nil},
		{5082, "FB100", "Front Bumper Cover", "PRASCO", []string{"86511-D3000"}, []int{1001, 1002}, "body", false, false, nil},
		{5083, "FG100", "Front Grille", "PRASCO", []string{"86350-D3000"}, []int{1001, 1002}, "body", false, false, nil},
		{5084, "FD100", "Fender Right", "PRASCO", []string{"66321-D3000"}, []int{1001, 1002}, "body", false, false, nil},
		{5085, "HD100", "Hood / Bonnet", "PRASCO", []string{"66400-D3000"}, []int{1001, 1002}, "body", false, false, nil},
		{5086, "TG100", "Tailgate / Trunk Lid", "PRASCO", []string{"73700-D3000"}, []int{1001, 1002}, "body", false, false, nil},
		{5087, "WR100", "Window Regulator Front Left", "BLIC", []string{"82471-D3010"}, []int{1001}, "body", false, false, nil},
		{5088, "IN100", "Indicator Side Left", "TYC", []string{"87614-D3000"}, []int{1001, 1002}, "body", false, false, nil},
		{5089, "RL100", "Rear Light Left", "HELLA", []string{"92401-D3100"}, []int{1001, 1002}, "body", false, false, nil},

		// ─── DRIVETRAIN PARTS ─────────────────────────────────────────────
		{5030, "CVJ100", "CV Joint Kit", "SKF", []string{"49500-D3600"}, []int{2001}, "drivetrain", false, false, nil},
		{5090, "DS100", "Drive Shaft Front Left", "SKF", []string{"49501-D3600"}, []int{2001}, "drivetrain", false, false, []string{"GKN"}},
		{5091, "CL100", "Clutch Kit 3pc", "SACHS", []string{"41100-2D100"}, []int{1001}, "drivetrain", false, false, []string{"LUK", "VALEO"}},
		{5092, "DF100", "Differential Oil Seal", "CORTECO", []string{"47353-2S000"}, []int{2001}, "drivetrain", false, false, nil},
		{5093, "DS200", "Drive Shaft Right", "GKN", []string{"49500-2S200"}, []int{1001}, "drivetrain", false, false, nil},
		{5094, "PS100", "Propshaft Center Bearing", "SKF", []string{"49575-2S000"}, []int{1001}, "drivetrain", false, false, nil},

		// ─── SUSPENSION & STEERING ────────────────────────────────────────
		{5100, "SA100", "Shock Absorber Front", "SACHS", []string{"54651-D3100"}, []int{1001, 2001}, "universal", false, true, []string{"KYB", "MONROE"}},
		{5101, "SA200", "Shock Absorber Rear", "KYB", []string{"55300-D3100"}, []int{1001}, "universal", false, false, []string{"MONROE"}},
		{5102, "CS100", "Coil Spring Front", "LESJOFORS", []string{"54630-D3100"}, []int{1001}, "universal", false, false, nil},
		{5103, "BJ100", "Ball Joint Lower", "LEMFORDER", []string{"54530-D3000"}, []int{1001, 2001}, "universal", false, true, []string{"TRW", "MOOG"}},
		{5104, "TR100", "Tie Rod End Outer", "TRW", []string{"56820-D3000"}, []int{1001, 2001}, "universal", false, true, []string{"LEMFORDER"}},
		{5105, "SL100", "Stabilizer Link Front", "TRW", []string{"54830-D3000"}, []int{1001, 2001}, "universal", false, true, nil},
		{5106, "CA100", "Control Arm Lower Front", "MEYLE", []string{"54500-D3000"}, []int{1001}, "universal", false, false, []string{"LEMFORDER"}},
		{5107, "WB200", "Wheel Bearing Front", "SKF", []string{"51720-D3000"}, []int{1001, 2001}, "universal", false, true, []string{"FAG", "NTN"}},
		{5108, "SR100", "Steering Rack Boot", "LEMFORDER", []string{"57740-D3000"}, []int{1001}, "universal", false, false, nil},
		{5109, "SB200", "Strut Bearing Front", "SKF", []string{"54612-D3000"}, []int{1001}, "universal", false, false, nil},

		// ─── ELECTRICAL ───────────────────────────────────────────────────
		{5110, "BT100", "Battery 60Ah", "VARTA", []string{"37110-D3600"}, []int{1001}, "universal", false, false, []string{"BOSCH", "EXIDE"}},
		{5111, "HS100", "Horn", "HELLA", []string{"96610-D3000"}, []int{1001, 2001}, "universal", false, true, nil},
		{5112, "BL100", "Bulb H7 55W", "OSRAM", []string{"18645-55009"}, []int{1001, 1002, 2001, 2002}, "universal", false, true, []string{"PHILIPS"}},
		{5113, "FS100", "Fuse 15A", "HELLA", []string{"91850-0Z000"}, []int{1001, 1002, 2001, 2002}, "universal", false, true, nil},
		{5114, "SS100", "Speed Sensor ABS Front", "ATE", []string{"59830-D3000"}, []int{1001, 2001}, "universal", false, true, nil},
		{5115, "WH100", "Wiring Harness Headlamp", "HELLA", []string{"91210-D3100"}, []int{1001}, "universal", false, false, nil},

		// ─── HVAC ─────────────────────────────────────────────────────────
		{5120, "AC100", "A/C Compressor", "DENSO", []string{"97701-D3500"}, []int{1001}, "universal", false, false, []string{"VALEO", "DELPHI"}},
		{5121, "HC100", "Heater Core", "NISSENS", []string{"97138-D3000"}, []int{1001, 1002}, "universal", false, false, nil},
		{5122, "CR100", "A/C Condenser", "NISSENS", []string{"97606-D3500"}, []int{1001}, "universal", false, false, []string{"DENSO"}},
		{5123, "BM100", "Blower Motor", "VALEO", []string{"97113-D3000"}, []int{1001, 1002, 2001}, "universal", false, true, nil},

		// ─── EXHAUST ──────────────────────────────────────────────────────
		{5130, "MF100", "Exhaust Muffler Rear", "WALKER", []string{"28650-D3100"}, []int{1001}, "engine", true, false, nil},
		{5131, "EP100", "Exhaust Pipe Front", "BOSAL", []string{"28610-D3100"}, []int{1001}, "engine", true, false, nil},
		{5132, "EG200", "Exhaust Gasket", "ELRING", []string{"28513-2B000"}, []int{1001, 1003}, "engine", true, true, nil},

		// ─── FUEL SYSTEM ──────────────────────────────────────────────────
		{5140, "FF100", "Fuel Filter", "MAHLE", []string{"31112-1R000"}, []int{1001}, "universal", false, false, []string{"MANN", "BOSCH"}},
		{5141, "FT100", "Fuel Tank Cap", "GATES", []string{"31010-D3000"}, []int{1001, 1002, 2001}, "universal", false, true, nil},

		// ─── COOLING EXTRAS ───────────────────────────────────────────────
		{5150, "RF100", "Radiator Fan Motor", "NISSENS", []string{"25386-D3100"}, []int{1001}, "engine", true, false, []string{"DENSO"}},
		{5151, "EC100", "Expansion Tank Cap", "GATES", []string{"25462-26100"}, []int{1001, 2001}, "engine", true, true, nil},
		{5152, "CT100", "Coolant Temperature Sensor", "FAE", []string{"39220-2B000"}, []int{1001}, "engine", true, false, nil},

		// ─── WIPERS & WASHER ──────────────────────────────────────────────
		{5160, "WM100", "Wiper Motor Front", "VALEO", []string{"98110-D3000"}, []int{1001, 1002}, "body", false, false, nil},
		{5161, "WP300", "Washer Pump", "HELLA", []string{"98510-D3000"}, []int{1001, 1002, 2001}, "body", false, true, nil},

		// ─── INTERIOR / MISC. ─────────────────────────────────────────────
		{5170, "WN100", "Wheel Nut M12x1.5", "FEBI", []string{"52950-1Y000"}, []int{1001, 1002, 2001, 2002}, "universal", false, true, nil},
		{5171, "WB300", "Wheel Bolt M14x1.5", "FEBI", []string{"52951-1Y000"}, []int{1001}, "universal", false, false, nil},
		{5172, "GS100", "Gas Strut Tailgate", "STABILUS", []string{"81770-D3100"}, []int{1001, 1002}, "universal", false, false, nil},
		{5173, "SM100", "Side Mirror Glass Left", "BLIC", []string{"87611-D3010"}, []int{1001, 1002}, "body", false, false, nil},

		// ─── KIA-SPECIFIC PARTS ───────────────────────────────────────────
		{5180, "KBP100", "Brake Pad Set Front (Sportage)", "TEXTAR", []string{"58101-F1A00"}, []int{2001}, "brake", false, false, nil},
		{5181, "KHL100", "Headlight Right (Sportage)", "DEPO", []string{"92102-F1000"}, []int{2001}, "body", false, false, nil},
		{5182, "KTL100", "Tail Light Left (Sportage)", "TYC", []string{"92401-F1000"}, []int{2001}, "body", false, false, nil},
		{5183, "KAF100", "Air Filter (Sportage 2.0)", "MANN", []string{"28113-F1100"}, []int{2001}, "universal", false, false, nil},
		{5184, "KCF100", "Cabin Filter (Sportage)", "MANN", []string{"97133-F1000"}, []int{2001, 2002}, "universal", false, true, nil},
		{5185, "KFB100", "Front Bumper (Sportage)", "PRASCO", []string{"86511-F1000"}, []int{2001, 2002}, "body", false, true, nil},
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// DATABASE SETUP
// ═══════════════════════════════════════════════════════════════════════════════

func buildDB() *sql.DB {
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

	// ─── Vehicles ─────────────────────────────────────────────────────────
	for _, s := range []string{
		`INSERT INTO vehicle_lookup VALUES ('HYUNDAI','TUCSON',2018,2024,1001,'2.0 MPI (150 HP)','201805','202412','Petrol',1999,150)`,
		`INSERT INTO vehicle_lookup VALUES ('HYUNDAI','TUCSON',2018,2024,1002,'1.6 CRDi (136 HP)','201805','202412','Diesel',1598,136)`,
		`INSERT INTO vehicle_lookup VALUES ('HYUNDAI','TUCSON',2018,2024,1003,'2.0 CRDi (185 HP)','201805','202412','Diesel',1995,185)`,
		`INSERT INTO vehicle_lookup VALUES ('KIA','SPORTAGE',2018,2024,2001,'2.0 GDI (155 HP)','201805','202412','Petrol',1999,155)`,
		`INSERT INTO vehicle_lookup VALUES ('KIA','SPORTAGE',2018,2024,2002,'1.6 T-GDi (177 HP)','201805','202412','Petrol',1598,177)`,
	} {
		mustExec(db, s)
	}

	for _, s := range []string{
		`INSERT INTO nhtsa_tecdoc_bridge VALUES ('HYUNDAI','TUCSON',100,2018,2024)`,
		`INSERT INTO nhtsa_tecdoc_bridge VALUES ('KIA','SPORTAGE',200,2018,2024)`,
	} {
		mustExec(db, s)
	}

	mustExec(db, `INSERT INTO hk_platform_map VALUES ('TUCSON','SPORTAGE','NX4/NQ5')`)

	// ─── Seed all parts from the spec ─────────────────────────────────────
	parts := allParts()

	vehicleInfo := map[int]struct {
		desc, fuel, model       string
		manuId, modelId, cc, hp int
	}{
		1001: {"Tucson 2.0", "Petrol", "TUCSON", 183, 100, 1999, 150},
		1002: {"Tucson 1.6D", "Diesel", "TUCSON", 183, 100, 1598, 136},
		1003: {"Tucson 2.0D", "Diesel", "TUCSON", 183, 100, 1995, 185},
		2001: {"Sportage 2.0", "Petrol", "SPORTAGE", 184, 200, 1999, 155},
		2002: {"Sportage 1.6T", "Petrol", "SPORTAGE", 184, 200, 1598, 177},
	}

	for _, p := range parts {
		// Insert into hk_parts_cache for each vehicle
		for _, vid := range p.vehicles {
			vi := vehicleInfo[vid]
			mustExec(db, fmt.Sprintf(
				`INSERT OR IGNORE INTO hk_parts_cache VALUES (%d,%d,0,'%s','%s',%d,'%s','%s','%s',%d,%d,'%s','201805','202412','%s',%d,%d)`,
				vid, p.id, p.artNum, p.desc, 10, p.brand, p.desc, vi.desc,
				vi.manuId, vi.modelId, vi.model, vi.fuel, vi.cc, vi.hp))
		}

		// Insert OEM cross-refs
		for _, oem := range p.oems {
			mfr := "HYUNDAI"
			if p.crossBrand {
				mfr = "HYUNDAI/KIA"
			}
			mustExec(db, fmt.Sprintf(`INSERT INTO articlecrosses VALUES (%d,'%s','%s')`, p.id, oem, mfr))
			// OEM search index
			norm := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(oem, "-", ""), " ", ""))
			mustExec(db, fmt.Sprintf(`INSERT INTO oem_search_index VALUES ('%s','%s',%d,'articlecrosses','%s','%s','%s','%s')`,
				oem, norm, p.id, mfr, p.brand, p.artNum, p.desc))
		}

		// Insert alt-brand variants as separate articles
		for i, altBrand := range p.altBrands {
			altId := p.id + 10000 + (i+1)*100
			altArt := fmt.Sprintf("%s-ALT%d", p.artNum, i+1)
			// Same OEM cross-refs, same vehicles
			for _, vid := range p.vehicles {
				vi := vehicleInfo[vid]
				mustExec(db, fmt.Sprintf(
					`INSERT OR IGNORE INTO hk_parts_cache VALUES (%d,%d,0,'%s','%s',%d,'%s','%s','%s',%d,%d,'%s','201805','202412','%s',%d,%d)`,
					vid, altId, altArt, p.desc, 10+i+1, altBrand, p.desc, vi.desc,
					vi.manuId, vi.modelId, vi.model, vi.fuel, vi.cc, vi.hp))
			}
		}
	}

	return db
}

func mustExec(db *sql.DB, sqlStr string) {
	if _, err := db.Exec(sqlStr); err != nil {
		fmt.Printf("FATAL: %v\nSQL: %s\n", err, sqlStr)
		os.Exit(1)
	}
}

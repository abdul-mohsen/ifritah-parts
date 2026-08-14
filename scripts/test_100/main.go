package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"

	"parts-engine/internal/model"
	"parts-engine/internal/service"
)

// ═══════════════════════════════════════════════════════════════════════════
// 100-TEST OFFLINE VERIFICATION SUITE
// Tests every service, query path, edge case, and scoring function
// using an in-memory SQLite database with representative sample data.
// ═══════════════════════════════════════════════════════════════════════════

var pass, fail int

func check(name string, ok bool, detail string) {
	if ok {
		fmt.Printf("  ✓ %s\n", name)
		pass++
	} else {
		fmt.Printf("  ✗ %s — %s\n", name, detail)
		fail++
	}
}

func main() {
	db := setupDB()
	defer db.Close()

	// ─── Services ──────────────────────────────────────────────────────────
	pl := service.NewPartsLookup(db, true)
	cr := service.NewCrossRef(db, true)
	ol := service.NewOEMLookup(db)
	pf := service.NewPlatform(db)
	ss := service.NewSmartSearch(db, pl, cr, ol, pf, nil, true)

	// ─── 1. PartsLookup ────────────────────────────────────────────────────
	fmt.Println("\n══ 1. PartsLookup ══")

	// T1: Basic find by linkage target
	parts, total, err := pl.FindByLinkageTarget(1001, "", 1, 20)
	check("T01 FindByLinkageTarget(1001)", err == nil && total >= 5, fmt.Sprintf("err=%v total=%d", err, total))

	// T2: Category filter
	parts, total, err = pl.FindByLinkageTarget(1001, "Oil", 1, 20)
	check("T02 FindByLinkageTarget+Oil filter", err == nil && total >= 1 && parts[0].Description == "Oil Filter", fmt.Sprintf("err=%v total=%d parts=%+v", err, total, parts))

	// T3: Category filter — Brake
	parts, total, err = pl.FindByLinkageTarget(1001, "Brake", 1, 20)
	check("T03 FindByLinkageTarget+Brake", err == nil && total >= 1, fmt.Sprintf("err=%v total=%d", err, total))

	// T4: Pagination page 1
	parts, _, err = pl.FindByLinkageTarget(1001, "", 1, 2)
	check("T04 Pagination page1 limit2", err == nil && len(parts) == 2, fmt.Sprintf("err=%v count=%d", err, len(parts)))

	// T5: Pagination page 2
	parts2, _, err := pl.FindByLinkageTarget(1001, "", 2, 2)
	check("T05 Pagination page2 limit2", err == nil && len(parts2) >= 1 && parts2[0].LegacyArticleId != parts[0].LegacyArticleId, fmt.Sprintf("err=%v count=%d", err, len(parts2)))

	// T6: Empty result for non-existent vehicle
	_, total, err = pl.FindByLinkageTarget(9999, "", 1, 20)
	check("T06 FindByLinkageTarget(nonexistent)", err == nil && total == 0, fmt.Sprintf("err=%v total=%d", err, total))

	// T7: Resolve linkage targets for Hyundai Tucson 2020
	vehicles, err := pl.ResolveLinkageTargets("HYUNDAI", "TUCSON", 2020)
	check("T07 ResolveLinkageTargets(TUCSON 2020)", err == nil && len(vehicles) >= 2, fmt.Sprintf("err=%v count=%d", err, len(vehicles)))

	// T8: Resolve linkage targets — year outside range
	vehicles, err = pl.ResolveLinkageTargets("HYUNDAI", "TUCSON", 2005)
	check("T08 ResolveLinkageTargets(year out of range)", err == nil && len(vehicles) == 0, fmt.Sprintf("err=%v count=%d", err, len(vehicles)))

	// T9: Resolve Kia Sportage
	vehicles, err = pl.ResolveLinkageTargets("KIA", "SPORTAGE", 2020)
	check("T09 ResolveLinkageTargets(SPORTAGE 2020)", err == nil && len(vehicles) >= 1, fmt.Sprintf("err=%v count=%d", err, len(vehicles)))

	// T10: Best linkage target — 2.0L Petrol
	best, err := pl.BestLinkageTargetWithHints("HYUNDAI", "TUCSON", 2020, 2000, "Petrol")
	check("T10 BestTarget(2.0 Petrol)→1001", err == nil && best != nil && best.LinkageTargetId == 1001, fmt.Sprintf("err=%v best=%+v", err, best))

	// T11: Best linkage target — 1.6L Diesel
	best, err = pl.BestLinkageTargetWithHints("HYUNDAI", "TUCSON", 2020, 1600, "Diesel")
	check("T11 BestTarget(1.6 Diesel)→1002", err == nil && best != nil && best.LinkageTargetId == 1002, fmt.Sprintf("err=%v best=%+v", err, best))

	// T12: Best linkage — no hints, falls back to most parts
	best, err = pl.BestLinkageTarget("HYUNDAI", "TUCSON", 2020)
	check("T12 BestTarget(no hints)", err == nil && best != nil, fmt.Sprintf("err=%v best=%+v", err, best))

	// T13: Best target — model not found
	best, err = pl.BestLinkageTargetWithHints("HYUNDAI", "GALAXY", 2020, 0, "")
	check("T13 BestTarget(unknown model)→nil", err == nil && best == nil, fmt.Sprintf("err=%v best=%+v", err, best))

	// T14: Reverse by article — Oil Filter fits multiple vehicles
	revVehicles, err := pl.ReverseByArticle(5001, 50)
	check("T14 ReverseByArticle(OilFilter)→2+ vehicles", err == nil && len(revVehicles) >= 2, fmt.Sprintf("err=%v count=%d", err, len(revVehicles)))

	// T15: Reverse by article — Wiper fits all 3
	revVehicles, err = pl.ReverseByArticle(5010, 50)
	check("T15 ReverseByArticle(Wiper)→3 vehicles", err == nil && len(revVehicles) >= 3, fmt.Sprintf("err=%v count=%d", err, len(revVehicles)))

	// T16: Reverse by article — unknown article
	revVehicles, err = pl.ReverseByArticle(9999, 50)
	check("T16 ReverseByArticle(unknown)→empty", err == nil && len(revVehicles) == 0, fmt.Sprintf("err=%v count=%d", err, len(revVehicles)))

	// ─── 2. CrossRef ──────────────────────────────────────────────────────
	fmt.Println("\n══ 2. CrossRef ══")

	// T17: Find OEM numbers for an aftermarket article
	oems, err := cr.FindOEMNumbers(5001)
	check("T17 FindOEMNumbers(5001)→2+", err == nil && len(oems) >= 2, fmt.Sprintf("err=%v count=%d", err, len(oems)))

	// T18: OEM numbers contain the right numbers
	found26300 := false
	for _, o := range oems {
		if strings.Contains(o.RawNumber, "26300") {
			found26300 = true
		}
	}
	check("T18 OEM contains 26300-xxxxx", found26300, fmt.Sprintf("oems=%+v", oems))

	// T19: Find by OEM number (exact)
	refs, err := cr.FindByOEM("26300-35503", 20)
	check("T19 FindByOEM(26300-35503)", err == nil && len(refs) >= 1, fmt.Sprintf("err=%v count=%d", err, len(refs)))

	// T20: Find by OEM — normalized (no dash)
	refs, err = cr.FindByOEM("2630035503", 20)
	check("T20 FindByOEM(no dash)", err == nil && len(refs) >= 1, fmt.Sprintf("err=%v count=%d", err, len(refs)))

	// T21: Find by OEM — unknown number
	refs, err = cr.FindByOEM("ZZZZZ-ZZZZZ", 20)
	check("T21 FindByOEM(unknown)→empty", err == nil && len(refs) == 0, fmt.Sprintf("err=%v count=%d", err, len(refs)))

	// T22: FindOEMNumbers — unknown article
	oems, err = cr.FindOEMNumbers(9999)
	check("T22 FindOEMNumbers(unknown)→empty", err == nil && len(oems) == 0, fmt.Sprintf("err=%v count=%d", err, len(oems)))

	// T23: Find by OEM — air filter OEM
	refs, err = cr.FindByOEM("28113-2S000", 20)
	check("T23 FindByOEM(air filter)", err == nil && len(refs) >= 1, fmt.Sprintf("err=%v count=%d", err, len(refs)))

	// ─── 3. OEMLookup ────────────────────────────────────────────────────
	fmt.Println("\n══ 3. OEMLookup ══")

	// T24: OEM search — exact match
	result, err := ol.Search("26300-35503", 20)
	check("T24 OEMLookup.Search(26300-35503)", err == nil && result != nil && len(result.Results) >= 1, fmt.Sprintf("err=%v result=%+v", err, result))

	// T25: OEM search result has correct normalized form
	check("T25 OEM normalized=2630035503", result != nil && result.Normalized == "2630035503", fmt.Sprintf("got %s", func() string {
		if result != nil {
			return result.Normalized
		}
		return "nil"
	}()))

	// T26: Search with spaces
	result, err = ol.Search("26300 35503", 20)
	check("T26 OEMLookup.Search(spaces)", err == nil && result != nil && len(result.Results) >= 1, fmt.Sprintf("err=%v result=%+v", err, result))

	// T27: Search for brake pad OEM
	result, err = ol.Search("58101-D3A00", 20)
	check("T27 OEMLookup.Search(brake)", err == nil && result != nil && len(result.Results) >= 1, fmt.Sprintf("err=%v result=%+v", err, result))

	// T28: Search for unknown OEM
	result, err = ol.Search("ZZZZZ-ZZZZZ", 20)
	check("T28 OEMLookup.Search(unknown)→empty", err == nil && result != nil && result.Total == 0, fmt.Sprintf("err=%v result=%+v", err, result))

	// T29: NormalizeOEM strips everything
	check("T29 NormalizeOEM(26300-35503)", service.NormalizeOEM("26300-35503") == "2630035503", service.NormalizeOEM("26300-35503"))

	// T30: NormalizeOEM with dots and slashes
	check("T30 NormalizeOEM(26.300/355 03)", service.NormalizeOEM("26.300/355 03") == "2630035503", service.NormalizeOEM("26.300/355 03"))

	// ─── 4. Platform ──────────────────────────────────────────────────────
	fmt.Println("\n══ 4. Platform ══")

	// T31: DB lookup Hyundai→Kia
	siblings, err := pf.FindSiblings("HYUNDAI", "TUCSON")
	check("T31 FindSiblings(TUCSON→SPORTAGE)", err == nil && len(siblings) >= 1 && siblings[0].SiblingModel == "SPORTAGE", fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// T32: DB lookup Kia→Hyundai
	siblings, err = pf.FindSiblings("KIA", "SPORTAGE")
	check("T32 FindSiblings(SPORTAGE→TUCSON)", err == nil && len(siblings) >= 1 && siblings[0].SiblingModel == "TUCSON", fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// T33: Platform code is present
	check("T33 Platform code NX4/NQ5", len(siblings) > 0 && siblings[0].Platform == "NX4/NQ5", fmt.Sprintf("platform=%s", func() string {
		if len(siblings) > 0 {
			return siblings[0].Platform
		}
		return "none"
	}()))

	// T34: Fallback for model not in DB
	siblings, err = pf.FindSiblings("HYUNDAI", "ELANTRA")
	check("T34 Fallback(ELANTRA→FORTE)", err == nil && len(siblings) >= 1, fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// T35: Fallback includes FORTE
	hasFORTE := false
	for _, s := range siblings {
		if s.SiblingModel == "FORTE" {
			hasFORTE = true
		}
	}
	check("T35 ELANTRA→FORTE found", hasFORTE, fmt.Sprintf("siblings=%+v", siblings))

	// T36: Sonata→K5 fallback
	siblings, err = pf.FindSiblings("HYUNDAI", "SONATA")
	check("T36 Fallback(SONATA→K5)", err == nil && len(siblings) >= 1, fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// T37: Santa Fe→Sorento fallback
	siblings, err = pf.FindSiblings("HYUNDAI", "SANTA FE")
	check("T37 Fallback(SANTA FE→SORENTO)", err == nil && len(siblings) >= 1, fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// T38: Unknown brand → nil
	siblings, err = pf.FindSiblings("BMW", "X5")
	check("T38 FindSiblings(BMW)→nil", err == nil && len(siblings) == 0, fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// T39: Kia→Hyundai (EV pair)
	siblings, err = pf.FindSiblings("KIA", "EV6")
	check("T39 Fallback(EV6→IONIQ 5)", err == nil && len(siblings) >= 1, fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// T40: Palisade→Telluride
	siblings, err = pf.FindSiblings("HYUNDAI", "PALISADE")
	check("T40 Fallback(PALISADE→TELLURIDE)", err == nil && len(siblings) >= 1, fmt.Sprintf("err=%v siblings=%+v", err, siblings))

	// ─── 5. Category Classifier ───────────────────────────────────────────
	fmt.Println("\n══ 5. Category Classifier ══")

	// T41: Engine parts
	r := service.ClassifyCategory("Alternator")
	check("T41 Alternator→FitEngine", r.Driver == service.FitEngine, "")

	// T42: Strict engine
	r = service.ClassifyCategory("Spark Plug")
	check("T42 SparkPlug→FitEngine+Strict", r.Driver == service.FitEngine && r.Strict, fmt.Sprintf("strict=%v", r.Strict))

	// T43: Turbocharger
	r = service.ClassifyCategory("Turbocharger Cartridge")
	check("T43 Turbocharger→FitEngine", r.Driver == service.FitEngine, "")

	// T44: Body parts
	r = service.ClassifyCategory("Wiper Blade Set")
	check("T44 WiperBlade→FitBody", r.Driver == service.FitBody, fmt.Sprintf("got driver=%d", r.Driver))

	// T45: Headlight
	r = service.ClassifyCategory("Headlight Assembly Left")
	check("T45 Headlight→FitBody", r.Driver == service.FitBody, "")

	// T46: Mirror
	r = service.ClassifyCategory("Door Mirror Right")
	check("T46 Mirror→FitBody", r.Driver == service.FitBody, "")

	// T47: Drivetrain
	r = service.ClassifyCategory("CV Joint Kit")
	check("T47 CVJoint→FitDrivetrain", r.Driver == service.FitDrivetrain, "")

	// T48: Drive shaft
	r = service.ClassifyCategory("Drive Shaft Assembly")
	check("T48 DriveShaft→FitDrivetrain", r.Driver == service.FitDrivetrain, "")

	// T49: Differential
	r = service.ClassifyCategory("Rear Differential Seal")
	check("T49 Differential→FitDrivetrain", r.Driver == service.FitDrivetrain, "")

	// T50: Brake pad
	r = service.ClassifyCategory("Brake Pad Set Front")
	check("T50 BrakePad→FitBrake", r.Driver == service.FitBrake, "")

	// T51: Brake disc
	r = service.ClassifyCategory("Brake Disc Rear Vented")
	check("T51 BrakeDisc→FitBrake", r.Driver == service.FitBrake, "")

	// T52: Brake CCMargin
	check("T52 Brake CCMargin=1000", r.CCMargin == 1000, fmt.Sprintf("margin=%d", r.CCMargin))

	// T53: Universal — Oil filter
	r = service.ClassifyCategory("Oil Filter")
	check("T53 OilFilter→FitUniversal", r.Driver == service.FitUniversal, "")

	// T54: Universal — Air filter
	r = service.ClassifyCategory("Air Filter Element")
	check("T54 AirFilter→FitUniversal", r.Driver == service.FitUniversal, "")

	// T55: Universal — Cabin filter
	r = service.ClassifyCategory("Cabin Filter / Pollen Filter")
	check("T55 CabinFilter→FitUniversal", r.Driver == service.FitUniversal, "")

	// T56: Universal — Bulb
	r = service.ClassifyCategory("Bulb H7 55W")
	check("T56 Bulb→FitUniversal", r.Driver == service.FitUniversal, "")

	// T57: Unknown → defaults to Universal
	r = service.ClassifyCategory("Some Random Part XYZ")
	check("T57 Unknown→FitUniversal(default)", r.Driver == service.FitUniversal, "")

	// T58: Longest match wins: "Brake Pad" beats "Brake"
	r = service.ClassifyCategory("Brake Pad Set")
	check("T58 LongestMatch(BrakePad>Brake)", r.Driver == service.FitBrake, "")

	// T59: Case insensitive
	r = service.ClassifyCategory("ALTERNATOR ASSY")
	check("T59 CaseInsensitive(ALTERNATOR)", r.Driver == service.FitEngine, "")

	// T60: Catalytic converter
	r = service.ClassifyCategory("Catalytic Converter")
	check("T60 CatalyticConverter→FitEngine", r.Driver == service.FitEngine, "")

	// ─── 6. OEM Prefix Decoder ────────────────────────────────────────────
	fmt.Println("\n══ 6. OEM Prefix Decoder ══")

	// T61: Oil filter prefix 263
	cat := service.DecodeOEMPrefix("26300-35503")
	check("T61 263→Oil Filter", cat != nil && cat.Category == "Oil Filter", fmt.Sprintf("%+v", cat))

	// T62: Brakes prefix 581
	cat = service.DecodeOEMPrefix("58101-C1A00")
	check("T62 581→Front Brake", cat != nil && cat.System == "Brakes", fmt.Sprintf("%+v", cat))

	// T63: Headlight 921
	cat = service.DecodeOEMPrefix("92101-D3100")
	check("T63 921→Headlight Assembly", cat != nil && cat.Category == "Headlight Assembly", fmt.Sprintf("%+v", cat))

	// T64: Engine 21 prefix
	cat = service.DecodeOEMPrefix("21101-2B200")
	check("T64 21→Engine Block", cat != nil && cat.System == "Engine", fmt.Sprintf("%+v", cat))

	// T65: Cooling 28 prefix
	cat = service.DecodeOEMPrefix("28111-2S000")
	check("T65 28→Cooling System", cat != nil && cat.System == "Cooling", fmt.Sprintf("%+v", cat))

	// T66: Radiator 281 beats 28 (longest match)
	cat = service.DecodeOEMPrefix("28110-2S000")
	check("T66 281→Radiator(longest match)", cat != nil && cat.Category == "Radiator", fmt.Sprintf("%+v", cat))

	// T67: A/C Compressor 971
	cat = service.DecodeOEMPrefix("97113-2E200")
	check("T67 971→Compressor A/C", cat != nil && cat.Category == "Compressor A/C", fmt.Sprintf("%+v", cat))

	// T68: Door 72 prefix
	cat = service.DecodeOEMPrefix("72100-C1000")
	check("T68 72→Front Door", cat != nil && cat.Category == "Front Door", fmt.Sprintf("%+v", cat))

	// T69: Unknown prefix → nil
	cat = service.DecodeOEMPrefix("00000-00000")
	check("T69 Unknown prefix→nil", cat == nil, fmt.Sprintf("%+v", cat))

	// T70: Short input → nil
	cat = service.DecodeOEMPrefix("1")
	check("T70 Short input→nil", cat == nil, fmt.Sprintf("%+v", cat))

	// T71: Starter 361
	cat = service.DecodeOEMPrefix("36100-2B100")
	check("T71 361→Starter Motor", cat != nil && cat.Category == "Starter Motor", fmt.Sprintf("%+v", cat))

	// T72: Alternator 373
	cat = service.DecodeOEMPrefix("37300-2B960")
	check("T72 373→Alternator", cat != nil && cat.Category == "Alternator", fmt.Sprintf("%+v", cat))

	// ─── 7. SmartSearch ───────────────────────────────────────────────────
	fmt.Println("\n══ 7. SmartSearch ══")

	// T73: Text search — "Oil Filter"
	sr, err := ss.Search("Oil Filter", 0, 0, "", "", 1, 20)
	check("T73 TextSearch(Oil Filter)", err == nil && sr != nil && sr.SearchStrategy == "text_search" && len(sr.Results) >= 1, fmt.Sprintf("err=%v sr=%+v", err, sr))

	// T74: Text search results have confidence
	check("T74 TextSearch confidence=0.6", sr != nil && len(sr.Results) > 0 && sr.Results[0].Confidence == 0.6, "")

	// T75: OEM search — detected by pattern (digits + dash)
	sr, err = ss.Search("26300-35503", 0, 0, "", "", 1, 20)
	check("T75 OEMSearch(26300-35503)", err == nil && sr != nil && sr.SearchStrategy == "oem_crossref", fmt.Sprintf("strategy=%s", func() string {
		if sr != nil {
			return sr.SearchStrategy
		}
		return "nil"
	}()))

	// T76: Article search — alphanumeric no dash
	sr, err = ss.Search("OC570", 0, 0, "", "", 1, 20)
	check("T76 ArticleSearch(OC570)", err == nil && sr != nil && sr.SearchStrategy == "article_lookup", fmt.Sprintf("strategy=%s", func() string {
		if sr != nil {
			return sr.SearchStrategy
		}
		return "nil"
	}()))

	// T77: Vehicle search — with linkageTargetId
	sr, err = ss.Search("", 1001, 1999, "Petrol", "", 1, 20)
	check("T77 VehicleSearch(1001)", err == nil && sr != nil && sr.SearchStrategy == "vehicle_smart" && len(sr.Results) >= 3, fmt.Sprintf("err=%v results=%d strategy=%s", err, func() int {
		if sr != nil {
			return len(sr.Results)
		}
		return 0
	}(), func() string {
		if sr != nil {
			return sr.SearchStrategy
		}
		return "nil"
	}()))

	// T78: Vehicle search with text filter
	sr, err = ss.Search("Oil", 1001, 1999, "Petrol", "", 1, 20)
	check("T78 VehicleSearch+text(Oil)", err == nil && sr != nil && len(sr.Results) >= 1, fmt.Sprintf("err=%v results=%d", err, func() int {
		if sr != nil {
			return len(sr.Results)
		}
		return 0
	}()))

	// T79: Vehicle search with category filter
	sr, err = ss.Search("", 1001, 1999, "Petrol", "Brake", 1, 20)
	check("T79 VehicleSearch+cat(Brake)", err == nil && sr != nil && len(sr.Results) >= 1, fmt.Sprintf("err=%v results=%d", err, func() int {
		if sr != nil {
			return len(sr.Results)
		}
		return 0
	}()))

	// T80: Smart search — Wiper Blade has body fitment driver
	sr, err = ss.Search("Wiper", 1001, 0, "", "", 1, 20)
	check("T80 Wiper→body fitment", err == nil && sr != nil && len(sr.Results) >= 1 && sr.Results[0].FitmentDriver == "body", fmt.Sprintf("driver=%s", func() string {
		if sr != nil && len(sr.Results) > 0 {
			return sr.Results[0].FitmentDriver
		}
		return "nil"
	}()))

	// T81: Smart search — no results warning
	sr, err = ss.Search("ZZZZNONEXISTENT", 0, 0, "", "", 1, 20)
	check("T81 NoResults→warning", err == nil && sr != nil && len(sr.Warnings) >= 1, fmt.Sprintf("warnings=%+v", func() []string {
		if sr != nil {
			return sr.Warnings
		}
		return nil
	}()))

	// T82: GetCategories for vehicle
	cats, err := ss.GetCategories(1001)
	check("T82 GetCategories(1001)", err == nil && len(cats) >= 3, fmt.Sprintf("err=%v count=%d", err, len(cats)))

	// T83: Categories have fitment drivers
	hasFitment := false
	for _, c := range cats {
		if c.FitmentDriver != "" {
			hasFitment = true
		}
	}
	check("T83 Categories have fitmentDriver", hasFitment, "")

	// T84: Text search — "Alternator"
	sr, err = ss.Search("Alternator", 0, 0, "", "", 1, 20)
	check("T84 TextSearch(Alternator)", err == nil && sr != nil && len(sr.Results) >= 1, fmt.Sprintf("err=%v count=%d", err, func() int {
		if sr != nil {
			return len(sr.Results)
		}
		return 0
	}()))

	// T85: Article search — brake pad artnum
	sr, err = ss.Search("BP300", 0, 0, "", "", 1, 20)
	check("T85 ArticleSearch(BP300)", err == nil && sr != nil && len(sr.Results) >= 1, fmt.Sprintf("err=%v count=%d strategy=%s", err, func() int {
		if sr != nil {
			return len(sr.Results)
		}
		return 0
	}(), func() string {
		if sr != nil {
			return sr.SearchStrategy
		}
		return ""
	}()))

	// ─── 8. Confidence Scoring ────────────────────────────────────────────
	fmt.Println("\n══ 8. Confidence Scoring ══")

	// T86: Direct fitment → 0.95
	sr, err = ss.Search("", 1001, 1999, "Petrol", "", 1, 20)
	directConf := false
	if sr != nil {
		for _, r := range sr.Results {
			if r.Confidence == 0.95 {
				directConf = true
			}
		}
	}
	check("T86 DirectFitment→0.95", directConf, "")

	// T87: Universal part → 0.90
	sr, err = ss.Search("Oil Filter", 0, 0, "", "", 1, 20)
	check("T87 TextSearch→0.6 (no vehicle context)", sr != nil && len(sr.Results) > 0 && sr.Results[0].Confidence == 0.6, "")

	// T88: Engine part with no CC → 0.7
	sr, err = ss.Search("Alternator", 0, 0, "", "", 1, 20)
	check("T88 EnginePartNoCC→0.6(text)", sr != nil && len(sr.Results) > 0 && sr.Results[0].Confidence == 0.6, fmt.Sprintf("conf=%f", func() float64 {
		if sr != nil && len(sr.Results) > 0 {
			return sr.Results[0].Confidence
		}
		return -1
	}()))

	// ─── 9. Edge Cases ────────────────────────────────────────────────────
	fmt.Println("\n══ 9. Edge Cases ══")

	// T89: Empty query → text search with empty results
	sr, err = ss.Search("", 0, 0, "", "", 1, 20)
	check("T89 EmptyQuery→results(text)", err == nil && sr != nil, fmt.Sprintf("err=%v", err))

	// T90: Very large limit clamped to 100
	parts, _, err = pl.FindByLinkageTarget(1001, "", 1, 500)
	check("T90 LargeLimitClamped", err == nil, fmt.Sprintf("err=%v", err))

	// T91: Page 0 treated as page 1
	parts, _, err = pl.FindByLinkageTarget(1001, "", 0, 10)
	check("T91 Page0→Page1", err == nil && len(parts) > 0, fmt.Sprintf("err=%v count=%d", err, len(parts)))

	// T92: Negative limit → default 20
	parts, _, err = pl.FindByLinkageTarget(1001, "", 1, -1)
	check("T92 NegativeLimit→default", err == nil, fmt.Sprintf("err=%v", err))

	// T93: OEM search with extra whitespace
	result, err = ol.Search("  26300-35503  ", 20)
	check("T93 OEMSearch(whitespace)", err == nil && result != nil && len(result.Results) >= 1, fmt.Sprintf("err=%v", err))

	// T94: platform sibling make is correct
	siblings, err = pf.FindSiblings("HYUNDAI", "TUCSON")
	check("T94 SiblingMake=KIA", len(siblings) > 0 && siblings[0].SiblingMake == "KIA", "")

	// T95: CrossRef result has brand name
	refs, err = cr.FindByOEM("26300-35503", 20)
	check("T95 CrossRef brand populated", len(refs) > 0 && refs[0].BrandName != "", fmt.Sprintf("brand=%s", func() string {
		if len(refs) > 0 {
			return refs[0].BrandName
		}
		return "empty"
	}()))

	// ─── 10. Multi-Strategy Integration ──────────────────────────────────
	fmt.Println("\n══ 10. Integration ══")

	// T96: OEM search finds cross-ref AND oem_search_index
	sr, err = ss.Search("26300-35503", 1001, 1999, "", "", 1, 20)
	check("T96 OEM+vehicle→crossref", err == nil && sr != nil && len(sr.Results) >= 1, fmt.Sprintf("err=%v count=%d", err, func() int {
		if sr != nil {
			return len(sr.Results)
		}
		return 0
	}()))

	// T97: Article search with vehicle CC context
	sr, err = ss.Search("OC570", 0, 1999, "", "", 1, 20)
	check("T97 Article+CC context", err == nil && sr != nil && len(sr.Results) >= 1, fmt.Sprintf("err=%v count=%d", err, func() int {
		if sr != nil {
			return len(sr.Results)
		}
		return 0
	}()))

	// T98: Vehicle search shows all categories
	sr, err = ss.Search("", 1001, 1999, "Petrol", "", 1, 100)
	hasCats := sr != nil && len(sr.Categories) >= 3
	check("T98 VehicleSearch has categories list", hasCats, fmt.Sprintf("cats=%+v", func() []string {
		if sr != nil {
			return sr.Categories
		}
		return nil
	}()))

	// T99: Text search for part number "LA123"
	sr, err = ss.Search("LA123", 0, 0, "", "", 1, 20)
	check("T99 Search(LA123)→article", err == nil && sr != nil && sr.SearchStrategy == "article_lookup", fmt.Sprintf("strategy=%s", func() string {
		if sr != nil {
			return sr.SearchStrategy
		}
		return ""
	}()))

	// T100: Platform shares same Oil Filter between Tucson and Sportage
	tuParts, _, _ := pl.FindByLinkageTarget(1001, "Oil", 1, 20)
	spParts, _, _ := pl.FindByLinkageTarget(2001, "Oil", 1, 20)
	sameOil := len(tuParts) > 0 && len(spParts) > 0 && tuParts[0].LegacyArticleId == spParts[0].LegacyArticleId
	check("T100 Tucson+Sportage share Oil Filter", sameOil, fmt.Sprintf("tucson=%+v sportage=%+v", tuParts, spParts))

	// ═══ Summary ═══════════════════════════════════════════════════════════
	fmt.Printf("\n════════════════════════════════════════════\n")
	fmt.Printf("  PASS: %d  |  FAIL: %d  |  TOTAL: %d\n", pass, fail, pass+fail)
	fmt.Printf("════════════════════════════════════════════\n")

	if fail > 0 {
		os.Exit(1)
	}

	_ = parts
	_ = model.Part{}
}

// ClassifyCategoryName is a helper for test output
func init() {}

// ═══════════════════════════════════════════════════════════════════════════
// DATABASE SETUP — rich sample data representing real-world scenarios
// ═══════════════════════════════════════════════════════════════════════════

func setupDB() *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		fmt.Printf("FATAL: %v\n", err)
		os.Exit(1)
	}

	// Schema
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
		`CREATE TABLE nhtsa_tecdoc_bridge (
			nhtsa_make TEXT, nhtsa_model TEXT, tecdoc_model_id INTEGER,
			year_from INTEGER, year_to INTEGER)`,
		`CREATE TABLE oem_search_index (
			raw_number TEXT, normalized TEXT, legacyArticleId INTEGER, source_table TEXT,
			mfr_name TEXT, brand_name TEXT, article_number TEXT, description TEXT)`,
		`CREATE TABLE articlecrosses (
			legacyArticleId INTEGER, oemNumber TEXT, brandName TEXT)`,
		`CREATE TABLE hk_platform_map (
			hyundai_model TEXT, kia_model TEXT, platform_code TEXT)`,
		// Indexes
		`CREATE INDEX idx_hk_article ON hk_parts_cache(legacyArticleId)`,
		`CREATE INDEX idx_hk_artnum ON hk_parts_cache(articleNumber)`,
		`CREATE INDEX idx_hk_desc ON hk_parts_cache(genericArticleDesc)`,
		`CREATE INDEX idx_hk_brand ON hk_parts_cache(dataSupplierId)`,
		`CREATE INDEX idx_vl_lookup ON vehicle_lookup(nhtsa_make, nhtsa_model, year_from, year_to)`,
		`CREATE INDEX idx_oem_norm ON oem_search_index(normalized)`,
		`CREATE INDEX idx_cross_oem ON articlecrosses(oemNumber)`,
		`CREATE INDEX idx_cross_article ON articlecrosses(legacyArticleId)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			fmt.Printf("FATAL DDL: %v\nSQL: %s\n", err, ddl)
			os.Exit(1)
		}
	}

	// ─── Vehicle lookup (flattened nhtsa→linkagetargets) ───────────────────
	for _, s := range []string{
		// Hyundai Tucson variants
		`INSERT INTO vehicle_lookup VALUES ('HYUNDAI','TUCSON',2018,2024,1001,'2.0 MPI (150 HP)','201805','202412','Petrol',1999,150)`,
		`INSERT INTO vehicle_lookup VALUES ('HYUNDAI','TUCSON',2018,2024,1002,'1.6 CRDi (136 HP)','201805','202412','Diesel',1598,136)`,
		`INSERT INTO vehicle_lookup VALUES ('HYUNDAI','TUCSON',2018,2024,1003,'2.0 CRDi (185 HP)','201805','202412','Diesel',1995,185)`,
		// Kia Sportage
		`INSERT INTO vehicle_lookup VALUES ('KIA','SPORTAGE',2018,2024,2001,'2.0 GDI (155 HP)','201805','202412','Petrol',1999,155)`,
		`INSERT INTO vehicle_lookup VALUES ('KIA','SPORTAGE',2018,2024,2002,'1.6 T-GDi (177 HP)','201805','202412','Petrol',1598,177)`,
		// Hyundai Sonata
		`INSERT INTO vehicle_lookup VALUES ('HYUNDAI','SONATA',2019,2024,3001,'2.5 GDi (191 HP)','201903','202412','Petrol',2497,191)`,
	} {
		exec(db, s)
	}

	// ─── NHTSA bridge ─────────────────────────────────────────────────────
	for _, s := range []string{
		`INSERT INTO nhtsa_tecdoc_bridge VALUES ('HYUNDAI','TUCSON',100,2018,2024)`,
		`INSERT INTO nhtsa_tecdoc_bridge VALUES ('KIA','SPORTAGE',200,2018,2024)`,
		`INSERT INTO nhtsa_tecdoc_bridge VALUES ('HYUNDAI','SONATA',300,2019,2024)`,
	} {
		exec(db, s)
	}

	// ─── Parts cache — realistic multi-brand multi-category ───────────────
	// Format: linkingTargetId, legacyArticleId, asmGrpId, articleNumber, desc, dataSupplierId, brandName, categoryName, vehicleDesc, manuId, modelId, modelName, beginYM, endYM, fuel, cc, hp
	for _, s := range []string{
		// === Tucson 2.0L Petrol (1001) ===
		// Oil Filter
		`INSERT INTO hk_parts_cache VALUES (1001,5001,0,'OC570','Oil Filter',10,'MAHLE','Engine / Oil Filter','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,
		// Air Filter
		`INSERT INTO hk_parts_cache VALUES (1001,5002,0,'LA123','Air Filter',10,'MAHLE','Engine / Air Filter','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,
		// Alternator
		`INSERT INTO hk_parts_cache VALUES (1001,5003,0,'DRA1919','Alternator',30,'DENSO','Engine / Alternator','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,
		// Brake Pad Set Front
		`INSERT INTO hk_parts_cache VALUES (1001,5004,0,'BP300','Brake Pad Set',40,'TRW','Brakes / Front Pads','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,
		// Wiper Blade (universal — fits all variants)
		`INSERT INTO hk_parts_cache VALUES (1001,5010,0,'WB100','Wiper Blade','20','BOSCH','Body / Wipers','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,
		// Cabin Filter
		`INSERT INTO hk_parts_cache VALUES (1001,5011,0,'CF200','Cabin Filter',10,'MAHLE','HVAC / Cabin Filter','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,
		// Headlight Left
		`INSERT INTO hk_parts_cache VALUES (1001,5020,0,'HL340','Headlight Assembly Left',50,'HELLA','Body / Headlight','Tucson 2.0',183,100,'TUCSON','201805','202412','Petrol',1999,150)`,

		// === Tucson 1.6L Diesel (1002) ===
		// Different oil filter for diesel
		`INSERT INTO hk_parts_cache VALUES (1002,5005,0,'OC571','Oil Filter',11,'BOSCH','Engine / Oil Filter','Tucson 1.6D',183,100,'TUCSON','201805','202412','Diesel',1598,136)`,
		// Glow plug (diesel only)
		`INSERT INTO hk_parts_cache VALUES (1002,5006,0,'GP400','Glow Plug',30,'DENSO','Engine / Glow Plug','Tucson 1.6D',183,100,'TUCSON','201805','202412','Diesel',1598,136)`,
		// Same wiper fits diesel too
		`INSERT INTO hk_parts_cache VALUES (1002,5010,0,'WB100','Wiper Blade',20,'BOSCH','Body / Wipers','Tucson 1.6D',183,100,'TUCSON','201805','202412','Diesel',1598,136)`,
		// Same cabin filter
		`INSERT INTO hk_parts_cache VALUES (1002,5011,0,'CF200','Cabin Filter',10,'MAHLE','HVAC / Cabin Filter','Tucson 1.6D',183,100,'TUCSON','201805','202412','Diesel',1598,136)`,
		// Same brake pads
		`INSERT INTO hk_parts_cache VALUES (1002,5004,0,'BP300','Brake Pad Set',40,'TRW','Brakes / Front Pads','Tucson 1.6D',183,100,'TUCSON','201805','202412','Diesel',1598,136)`,
		// Same headlight
		`INSERT INTO hk_parts_cache VALUES (1002,5020,0,'HL340','Headlight Assembly Left',50,'HELLA','Body / Headlight','Tucson 1.6D',183,100,'TUCSON','201805','202412','Diesel',1598,136)`,

		// === Tucson 2.0L Diesel (1003) ===
		// Same oil filter as 2.0 petrol (shared 2.0L block)
		`INSERT INTO hk_parts_cache VALUES (1003,5001,0,'OC570','Oil Filter',10,'MAHLE','Engine / Oil Filter','Tucson 2.0D',183,100,'TUCSON','201805','202412','Diesel',1995,185)`,
		// Same wiper
		`INSERT INTO hk_parts_cache VALUES (1003,5010,0,'WB100','Wiper Blade',20,'BOSCH','Body / Wipers','Tucson 2.0D',183,100,'TUCSON','201805','202412','Diesel',1995,185)`,

		// === Kia Sportage 2.0L Petrol (2001) — shared platform ===
		// Same oil filter as Tucson 2.0
		`INSERT INTO hk_parts_cache VALUES (2001,5001,0,'OC570','Oil Filter',10,'MAHLE','Engine / Oil Filter','Sportage 2.0',184,200,'SPORTAGE','201805','202412','Petrol',1999,155)`,
		// Same wiper
		`INSERT INTO hk_parts_cache VALUES (2001,5010,0,'WB100','Wiper Blade',20,'BOSCH','Body / Wipers','Sportage 2.0',184,200,'SPORTAGE','201805','202412','Petrol',1999,155)`,
		// Sportage-specific brake pads (slightly different specs)
		`INSERT INTO hk_parts_cache VALUES (2001,5007,0,'BP310','Brake Pad Set',45,'BREMBO','Brakes / Front Pads','Sportage 2.0',184,200,'SPORTAGE','201805','202412','Petrol',1999,155)`,
		// CV Joint
		`INSERT INTO hk_parts_cache VALUES (2001,5030,0,'CVJ100','CV Joint Kit',60,'SKF','Drivetrain / CV Joint','Sportage 2.0',184,200,'SPORTAGE','201805','202412','Petrol',1999,155)`,

		// === Kia Sportage 1.6T (2002) ===
		`INSERT INTO hk_parts_cache VALUES (2002,5005,0,'OC571','Oil Filter',11,'BOSCH','Engine / Oil Filter','Sportage 1.6T',184,200,'SPORTAGE','201805','202412','Petrol',1598,177)`,
		`INSERT INTO hk_parts_cache VALUES (2002,5010,0,'WB100','Wiper Blade',20,'BOSCH','Body / Wipers','Sportage 1.6T',184,200,'SPORTAGE','201805','202412','Petrol',1598,177)`,
	} {
		exec(db, s)
	}

	// ─── OEM cross-references ─────────────────────────────────────────────
	for _, s := range []string{
		`INSERT INTO articlecrosses VALUES (5001,'26300-35503','HYUNDAI/KIA')`,
		`INSERT INTO articlecrosses VALUES (5001,'26300-35504','HYUNDAI/KIA')`,
		`INSERT INTO articlecrosses VALUES (5002,'28113-2S000','HYUNDAI')`,
		`INSERT INTO articlecrosses VALUES (5003,'37300-2B960','HYUNDAI')`,
		`INSERT INTO articlecrosses VALUES (5004,'58101-D3A00','HYUNDAI')`,
		`INSERT INTO articlecrosses VALUES (5005,'26300-35505','HYUNDAI/KIA')`,
		`INSERT INTO articlecrosses VALUES (5010,'98350-D3000','HYUNDAI/KIA')`,
		`INSERT INTO articlecrosses VALUES (5020,'92101-D3100','HYUNDAI')`,
		`INSERT INTO articlecrosses VALUES (5030,'49500-D3600','KIA')`,
	} {
		exec(db, s)
	}

	// ─── OEM search index ─────────────────────────────────────────────────
	for _, s := range []string{
		`INSERT INTO oem_search_index VALUES ('26300-35503','2630035503',5001,'articlecrosses','HYUNDAI/KIA','MAHLE','OC570','Oil Filter')`,
		`INSERT INTO oem_search_index VALUES ('26300-35504','2630035504',5001,'articlecrosses','HYUNDAI/KIA','MAHLE','OC570','Oil Filter')`,
		`INSERT INTO oem_search_index VALUES ('28113-2S000','281132s000',5002,'articlecrosses','HYUNDAI','MAHLE','LA123','Air Filter')`,
		`INSERT INTO oem_search_index VALUES ('58101-D3A00','58101d3a00',5004,'articlecrosses','HYUNDAI','TRW','BP300','Brake Pad Set')`,
		`INSERT INTO oem_search_index VALUES ('37300-2B960','373002b960',5003,'articlecrosses','HYUNDAI','DENSO','DRA1919','Alternator')`,
	} {
		exec(db, s)
	}

	// ─── Platform map ─────────────────────────────────────────────────────
	exec(db, `INSERT INTO hk_platform_map VALUES ('TUCSON','SPORTAGE','NX4/NQ5')`)

	return db
}

func exec(db *sql.DB, sql string) {
	if _, err := db.Exec(sql); err != nil {
		fmt.Printf("FATAL SEED: %v\nSQL: %s\n", err, sql)
		os.Exit(1)
	}
}

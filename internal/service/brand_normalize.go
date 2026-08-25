package service

import "strings"

// NormalizeBrand collapses common brand-name variants to a single canonical
// form so dedup works across TecDoc + scraper + community sources.
//
// Input examples that map to canonical "Bosch":
//
//	"BOSCH", "Bosch", "bosch", "Robert Bosch GmbH", "BOSCH GmbH", "R BOSCH"
//
// Rules (in order):
//  1. Trim whitespace + uppercase.
//  2. Strip trailing corporate suffixes (GmbH, Inc, Ltd, AG, KG, S.A., SpA,
//     Co, LLC, Co Ltd, Corp, Corporation, Company).
//  3. Look up in brandCanonical - return canonical form when matched.
//  4. Otherwise return the cleaned string title-cased.
//
// Returns "" for empty input.
func NormalizeBrand(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	upper := strings.ToUpper(trimmed)
	// Strip corporate suffixes so "Robert Bosch GmbH" and "Robert Bosch"
	// hit the same map entry.
	for _, suffix := range corporateSuffixes {
		upper = strings.TrimSuffix(upper, " "+suffix)
	}
	upper = strings.TrimSpace(upper)

	// Also strip common word prefixes / trailing filler ("brand", "auto",
	// "parts") that TecDoc data has for some suppliers.
	upper = strings.TrimSuffix(upper, " BRAND")
	upper = strings.TrimSuffix(upper, " PARTS")
	upper = strings.TrimSuffix(upper, " AUTO")

	// Also collapse repeated inner whitespace.
	upper = strings.Join(strings.Fields(upper), " ")

	if canonical, ok := brandCanonical[upper]; ok {
		return canonical
	}

	// No canonical match — return a title-cased cleaned version so
	// downstream dedup at least treats capitalisation variants as one.
	return titleCase(upper)
}

// corporateSuffixes are TrimSuffix'd off the upper-cased input in
// NormalizeBrand before the canonical lookup.
var corporateSuffixes = []string{
	"GMBH",
	"INC",
	"INC.",
	"LTD",
	"LTD.",
	"LIMITED",
	"AG",
	"KG",
	"SA",
	"S.A.",
	"SPA",
	"S.P.A.",
	"CO",
	"CO.",
	"LLC",
	"L.L.C.",
	"CORP",
	"CORPORATION",
	"COMPANY",
	"COMPANIES",
	"& CO",
	"& CO.",
	"& SONS",
	"HOLDINGS",
	"GROUP",
}

// brandCanonical maps every known upper-case variant to its canonical
// display form. Add entries here when a new variant shows up in an audit
// with a "brand mismatch" note.
//
// Sourced from the top 200 aftermarket brands in the TecDoc-supplied
// ambrand table for HK OEMs (2026-08). Manufacturer and OEM entries also
// included so we don't dedup them incorrectly against aftermarket names.
var brandCanonical = map[string]string{
	// ═══ Filtration ═══
	"BOSCH":          "Bosch",
	"ROBERT BOSCH":   "Bosch",
	"BOSCH FILTER":   "Bosch",
	"MANN":           "MANN-FILTER",
	"MANN-FILTER":    "MANN-FILTER",
	"MANNFILTER":     "MANN-FILTER",
	"MANN FILTER":    "MANN-FILTER",
	"MANN HUMMEL":    "MANN-FILTER",
	"MANN+HUMMEL":    "MANN-FILTER",
	"MAHLE":          "MAHLE",
	"MAHLE ORIGINAL": "MAHLE",
	"MAHLEBEHR":      "MAHLE",
	"MAHLE BEHR":     "MAHLE",
	"KNECHT":         "MAHLE",
	"HENGST":         "Hengst",
	"HENGST FILTER":  "Hengst",
	"FILTRON":        "Filtron",
	"WIX":            "WIX",
	"WIX FILTERS":    "WIX",
	"FRAM":           "Fram",
	"DONALDSON":      "Donaldson",
	"K&N":            "K&N",
	"KN FILTERS":     "K&N",
	"UFI":            "UFI",
	"UFI FILTERS":    "UFI",
	"CLEAN FILTERS":  "Clean Filters",
	"BLUE PRINT":     "Blue Print",
	"BLUEPRINT":      "Blue Print",
	"PURFLUX":        "Purflux",
	"CROSLAND":       "Crosland",
	"COOPERSFIAAM":   "CoopersFiaam",
	"MICRO":          "Micro",
	"NIPPON":         "Nippon",
	"KOLBENSCHMIDT":  "Kolbenschmidt",
	"MECAFILTER":     "Mecafilter",

	// ═══ Braking ═══
	"TEXTAR":         "Textar",
	"FERODO":         "Ferodo",
	"TRW":            "TRW",
	"ATE":            "ATE",
	"ATE POWER DISC": "ATE",
	"BREMBO":         "Brembo",
	"BENDIX":         "Bendix",
	"WAGNER":         "Wagner",
	"WAGNERLITE":     "Wagner",
	"AKEBONO":        "Akebono",
	"NISSHINBO":      "Nisshinbo",
	"BOSCH BRAKES":   "Bosch",
	"PAGID":          "Pagid",
	"HELLA PAGID":    "Pagid",
	"JURID":          "Jurid",
	"EBC":            "EBC",
	"MINTEX":         "Mintex",
	"REMSA":          "Remsa",
	"ICER":           "Icer",
	"HI-Q":           "Hi-Q",
	"HIQ":            "Hi-Q",
	"HANKOOK":        "Hankook",

	// ═══ Ignition / Electrical ═══
	"NGK":               "NGK",
	"DENSO":             "Denso",
	"DENSO CORPORATION": "Denso",
	"NIPPONDENSO":       "Denso",
	"CHAMPION":          "Champion",
	"BERU":              "Beru",
	"MAGNETI MARELLI":   "Magneti Marelli",
	"MAGNETIMARELLI":    "Magneti Marelli",
	"VALEO":             "Valeo",
	"HELLA":             "Hella",
	"HELLA GUTMANN":     "Hella",
	"OSRAM":             "Osram",
	"PHILIPS":           "Philips",
	"NARVA":             "Narva",
	"GENERAL MOTORS":    "GM",
	"BOSCH SPARK":       "Bosch",
	"DELPHI":            "Delphi",
	"BORG WARNER":       "BorgWarner",
	"BORGWARNER":        "BorgWarner",

	// ═══ Bearings / Rolling elements ═══
	"SKF":          "SKF",
	"NSK":          "NSK",
	"NTN":          "NTN",
	"FAG":          "FAG",
	"FAG BEARINGS": "FAG",
	"KOYO":         "Koyo",
	"INA":          "INA",
	"SNR":          "SNR",
	"RUVILLE":      "Ruville",

	// ═══ Suspension / Shocks ═══
	"KYB":           "KYB",
	"KAYABA":        "KYB",
	"MONROE":        "Monroe",
	"GABRIEL":       "Gabriel",
	"SACHS":         "Sachs",
	"BILSTEIN":      "Bilstein",
	"TEIN":          "Tein",
	"H&R":           "H&R",
	"KONI":          "Koni",
	"OME":           "OME",
	"LEMFORDER":     "Lemforder",
	"LEMFOERDER":    "Lemforder",
	"MEYLE":         "Meyle",
	"MEYLE HD":      "Meyle",
	"MOOG":          "Moog",
	"FEBI":          "Febi",
	"FEBI BILSTEIN": "Febi",
	"FEBIBILSTEIN":  "Febi",
	"TRISCAN":       "Triscan",
	"KAGER":         "Kager",
	"MASTER-SPORT":  "Master-Sport",
	"MASTERSPORT":   "Master-Sport",

	// ═══ Timing / Belt / Chain ═══
	"GATES":                 "Gates",
	"DAYCO":                 "Dayco",
	"CONTITECH":             "Contitech",
	"CONTINENTAL":           "Continental",
	"CONTINENTAL CONTITECH": "Contitech",
	"OPTIBELT":              "Optibelt",

	// ═══ Cooling ═══
	"BEHR":          "MAHLE",
	"BEHR HELLA":    "MAHLE",
	"VALEO CLIMATE": "Valeo",
	"NRF":           "NRF",
	"NISSENS":       "Nissens",
	"AKS DASIS":     "AKS Dasis",
	"AKSDASIS":      "AKS Dasis",
	"THERMOTEC":     "Thermotec",

	// ═══ Sensors / Fuel injection ═══
	"HITACHI":      "Hitachi",
	"AISIN":        "Aisin",
	"MOTORCRAFT":   "Motorcraft",
	"STANDARD":     "Standard",
	"HERTH+BUSS":   "Herth+Buss",
	"HERTH BUSS":   "Herth+Buss",
	"HERTHBUSS":    "Herth+Buss",
	"JAKOPARTS":    "Herth+Buss",
	"WALKER":       "Walker",
	"BOSCH SENSOR": "Bosch",

	// ═══ Body / Lighting / Trim ═══
	"DEPO":                "Depo",
	"TYC":                 "TYC",
	"AUTOMOTIVE LIGHTING": "Automotive Lighting",
	"VISTEON":             "Visteon",
	"GORDON":              "Gordon",
	"POLCAR":              "Polcar",
	"ULO":                 "ULO",

	// ═══ Wipers ═══
	"CHAMPION WIPERS": "Champion",
	"VALEO WIPERS":    "Valeo",
	"SWF":             "SWF",
	"TRICO":           "Trico",
	"BOSCH AEROTWIN":  "Bosch",
	"BOSCH TWIN":      "Bosch",

	// ═══ Fluids / Consumables ═══
	"CASTROL":    "Castrol",
	"MOBIL":      "Mobil",
	"MOBIL 1":    "Mobil",
	"SHELL":      "Shell",
	"TOTAL":      "Total",
	"MOTUL":      "Motul",
	"FUCHS":      "Fuchs",
	"LIQUI MOLY": "Liqui Moly",
	"LIQUIMOLY":  "Liqui Moly",

	// ═══ OEM manufacturers (not aftermarket - keep distinct) ═══
	"MOBIS":         "Mobis",
	"HYUNDAI MOBIS": "Mobis",
	"HYUNDAIMOBIS":  "Mobis",
	"HMC":           "Mobis",
	"HYUNDAI":       "Hyundai",
	"KIA":           "Kia",
	"HYUNDAI / KIA": "Hyundai/Kia",
	"HYUNDAI/KIA":   "Hyundai/Kia",
	"HYUNDAIKIA":    "Hyundai/Kia",
	"GENESIS":       "Genesis",

	// ═══ Other Korean tier-1 suppliers ═══
	"HANON":           "Hanon Systems",
	"HANON SYSTEMS":   "Hanon Systems",
	"DOOWON":          "Doowon",
	"MANDO":           "Mando",
	"HALLA":           "Halla",
	"HYUNDAI MOTOR":   "Mobis",
	"HANDOK":          "Handok",
	"HYUNDAI TRANSYS": "Hyundai Transys",
	"HYUNDAI KEFICO":  "Kefico",
	"KEFICO":          "Kefico",
	"KEEKUL":          "Keekul",

	// ═══ Japanese tier-1 ═══
	"TOKICO": "Tokico",
	"AISHIN": "Aisin",
	"ASHUKI": "Ashuki",
	"YAMATO": "Yamato",

	// ═══ Common Chinese-origin brands ═══
	"HYUNDAI GENUINE": "Hyundai",
	"KIA GENUINE":     "Kia",
	"OEM":             "OEM",
	"GENUINE":         "OEM",
	"ORIGINAL":        "OEM",
	"GENUINE PARTS":   "OEM",
}

// titleCase renders "MANN FILTER" -> "Mann Filter" and "BOSCH" -> "Bosch"
// so unknown brands at least get a consistent-looking form.
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) == 1 {
			words[i] = strings.ToUpper(w)
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

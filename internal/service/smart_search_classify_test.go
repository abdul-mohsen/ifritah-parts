package service

import "testing"

// TestLooksLikeOEMNumber_AllSeedOEMs verifies that every Hyundai/KIA OEM
// number in the seed database is recognised as an OEM query.
// All numbers share the canonical NNNNN-XXXXX format: digit-first + dash +
// at least 4 digits across the whole string.
// Source: seed_db/main.go — 102 OEM numbers, confirmed live 2026-08-15.
func TestLooksLikeOEMNumber_AllSeedOEMs(t *testing.T) {
	oems := []string{
		// Engine — oil filter, air filter, ignition coil, water pump, timing chain
		"26300-35505", "26300-35530",
		"28113-D3100", "28113-F2100", "28113-L1100", "28113-S8100",
		"27301-2B100",
		"18843-10062", "18855-10080",
		"25100-2E100", "25100-2B000",
		"25500-2B100",
		"25310-2S500", "25380-2S500",
		"25212-2B020", "25281-2B010",
		"21810-2S000", "21930-2S200", "21830-2S200",
		"24312-2B000",
		// Electrical — alternator, starter, sensors, ECU
		"39210-2B100", "39350-2B100", "39180-2B000", "39450-2S500",
		"37300-2B100", "36100-2B100",
		// Brakes — pads, shoes, calipers, parking brake
		"59830-D3000", "59930-D3000",
		"58101-D3A70", "51712-D3100",
		"58101-F2A00", "51712-F2100",
		"58101-S8A70",
		"58302-D3A70",
		"58411-D3100", "58411-F2100",
		"58510-2S300", "58732-2S000",
		// Suspension — shocks, struts, control arms, bushings
		"54651-D3000", "54530-D3000",
		"54500-D3000", "54501-D3000",
		"54830-D3000", "51720-D3000",
		"55300-D3000", "55530-D3000",
		"56820-D3000", "57724-D3000",
		"54651-J9000", "54651-L1000", "54651-S1000",
		// Brakes continued
		"58101-J9A00", "58101-L0A00",
		// Lighting — headlights, tail lights, fog lights
		"92101-D3100", "92102-D3100",
		"92101-Q5100", "92102-Q5100",
		"92101-F2020", "92102-F2020",
		"92401-D3100", "92402-D3100",
		// Body & mirrors
		"86511-D3100", "86611-D3100", "86350-D3100",
		"66311-D3100", "66321-D3100", "66400-D3100",
		"86511-Q5000",
		"87610-D3100", "87620-D3100", "87610-D3520",
		// Maintenance — wipers, sensors
		"98350-D3100", "98100-D3100",
		// Drivetrain — clutch, driveshaft, differential
		"41100-2D100",
		"49500-D3600", "49501-D3600", "49590-D3000",
		// HVAC — A/C compressor, cabin filter, blower
		"97701-D3000", "97606-D3000",
		"97133-D3000", "97133-F2000", "97133-J9000",
		"97113-D3000", "97115-D3000",
		// Engine (ignition/starter) continued
		"18640-11080",
		// Electrical continued
		"96610-D3100", "31112-D3000",
		// Transmission
		"35310-2S000",
		// Air filter continued
		"28510-2S500", "28410-2B100", "28830-2U000",
		// Air/cabin filter additional variants
		"52933-1P000", "52933-D4100", "52933-3X300",
		// Electrical switches
		"82401-D3010", "82402-D3010",
		// Suspension continued
		"51750-D3000", "52730-D3100",
		// Engine oil pressure sensors
		"25411-D3100", "25412-D3100",
		// Transmission/engine
		"29100-2B800", "39110-2B000",
	}
	for _, q := range oems {
		if !looksLikeOEMNumber(q) {
			t.Errorf("looksLikeOEMNumber(%q) = false, want true", q)
		}
	}
}

// TestLooksLikeOEMNumber_RealAftermarketWithSeparators tests real aftermarket
// article numbers from the live API that start with a digit and contain a
// dash or space separator. The classifier treats these as OEM-like (true).
//
// Also tests digit-first strings that use only dot separators (not '-' or ' '),
// which the classifier correctly rejects (false) because the dashes counter
// only increments on '-' or ' '.
func TestLooksLikeOEMNumber_RealAftermarketWithSeparators(t *testing.T) {
	cases := []struct {
		q    string
		want bool
		note string
	}{
		// ── Digit-first + '-' or ' ' separator + ≥4 digits → TRUE ──────────
		// These aftermarket articles are mis-classified as OEM-like, which is
		// harmless: searchByOEM falls back to searchByArticle when no xref found.
		{"22-263544", true, "BILSTEIN 22-263544 shock absorber (legacyArticleId 307452203)"},
		{"821 871", true, "TOPRAN 821 871 cabin filter (legacyArticleId 956819673)"},
		{"001-10-25291", true, "BBR Automotive 001-10-25291 cabin filter (legacyArticleId 399866819)"},
		{"0 242 129 521", true, "BOSCH 0 242 129 521 spark plug"},
		{"0 986 025 720", true, "BOSCH 0 986 025 720 alternator"},
		{"0 986 494 557", true, "BOSCH 0 986 494 557 brake pad"},
		{"050 006 1255", true, "MEYLE 050 006 1255 poly-V belt"},
		{"503-07003", true, "IAP 503-07003"},
		{"72-0H-H76L", true, "ASHIKA 72-0H-H76L — digit-first, two dashes, 5 digits"},
		{"22-0886-1", true, "METELLI 22-0886-1 clutch bearing"},
		{"535 0271 10", true, "LUK 535 0271 10 (spaces, 9 digits)"},
		{"535 0326 10", true, "LUK 535 0326 10 (spaces)"},
		// Dot separators are NOT '-' or ' ' — only one real dash counts
		{"28.0002-2225.2", true, "LUCAS — dots ignored, one '-' present, 11 digits → true"},
		{"43-Y16", true, "part 43-Y16 — digit first, dash, exactly 4 digits"},
		// ── Digit-first but ONLY dot separators → FALSE ─────────────────────
		// The classifier counts '-' and ' ' only; dots do not increment dashes.
		{"0133.3043", false, "dot separator only → dashes=0 → false"},
		{"903.1", false, "dot separator only → dashes=0 → false"},
		{"112172.1", false, "dot separator only → dashes=0 → false"},
	}
	for _, tc := range cases {
		got := looksLikeOEMNumber(tc.q)
		if got != tc.want {
			t.Errorf("looksLikeOEMNumber(%q) [%s] = %v, want %v",
				tc.q, tc.note, got, tc.want)
		}
	}
}

// TestLooksLikeOEMNumber_LetterFirstReturnsFalse verifies that every
// aftermarket article number starting with a letter (A-Z) returns false.
// The classifier short-circuits at `q[0] < '0' || q[0] > '9'`.
// Source: all 182 real article numbers from qa.ifritah.com 2026-08-15.
func TestLooksLikeOEMNumber_LetterFirstReturnsFalse(t *testing.T) {
	letterFirst := []string{
		// OEM 26300-35505 crossrefs (MANN / PURFLUX / BOSCH / HERTH+BUSS / FRAM / HENGST)
		"W 811/80", "LS489A", "F 026 407 124", "J1317003", "PH6811", "H13W01",
		// OEM 97133-D3000 crossrefs (MANN / TOPRAN→see with-separator / AMC / HERTH / HENGST / BBR)
		"CU 23 019", "HC-8232", "J1340529", "E4961LI",
		// Bosch and multi-vendor
		"SM 125", "BFO4198", "QFL0370",
		"S 3583 R", "C 28 040", "MD-8948", "MFA-K370", "HA-743",
		"N1320556", "H132I56", "EAF950",
		"J1320558", "CU 24 013",
		"DCF577P", "PC8495", "ADG02592",
		"SA 1338", "AH521", "CF12160",
		// Mixed alphanumeric letter-first (XUH / WG / OE / CCH)
		"XUH20TTi", "WG1462276", "OE197/T10", "CCH9023",
		// BSG (digit after prefix, but B-first → false)
		"BSG 40-835-007", "BSG 40-840-011",
		// CBE / AQ / PA / FWP / ADG
		"CBE5413", "AQ-2363", "PA1517", "PA10119", "FWP2233", "ADG09162",
		// SKF drive-belt related: VKPC / VKM / APV / P
		"VKPC 95895", "VKPC 95898", "APV2998", "VKM 64056", "P254005",
		// AD / WG / EEM (with dashes) — exact live-API article numbers
		"AD06R1255", "WG1781552", "EEM-3125", "EEM-4094",
		// CS / CSR / WG
		"CS0204", "CSR3275", "WG1253830",
		// BPHY (with dash) / JQ / J3 / A-5272GL
		"BPHY-2004", "JQ101268", "J3610526", "A-5272GL",
		// EX (MANDO prefix) / CBKH / SBJ
		"EX54651D3000", "CBKH-42L", "SBJ-3041",
		// S / BS / SCA / MSA / SAK / S063
		"S080986", "BS-H76L", "SCA-4173", "MSA010082", "SAK-8772L", "S063033",
		// CLKK / SS / FDL / BDL / JRSHY
		"CLKK-44", "SS8093", "FDL7445", "BDL7445", "JRSHY-051",
		// DB / HYK / KA / KI-LS / JTE
		"DB78391", "HYK452", "KA2238", "KI-LS-16571", "JTE1860",
		// FTR / BTR / HN
		"FTR6016", "BTR6016", "HN8061011",
		// Pure letter strings and free text
		"ABCDE", "cabin air filter", "oil filter",
	}
	for _, q := range letterFirst {
		if looksLikeOEMNumber(q) {
			t.Errorf("looksLikeOEMNumber(%q) = true, want false (starts with letter)", q)
		}
	}
}

// TestLooksLikeArticleNumber_RealAftermarketPositives verifies that real
// aftermarket article numbers from the live API are recognised by
// looksLikeArticleNumber.
//
// Dispatch order in Search(): looksLikeOEMNumber is checked FIRST.
// Entries below either:
//   (a) start with a letter  → OEM check fails at q[0] > '9'
//   (b) start with a digit but have no '-' or ' '  → OEM check fails at dashes==0
//
// The article classifier then applies:
//   • purely numeric, len ≥ 5 → true
//   • letter + digit, no '-' or ' ' → true  (second block)
//   • letter + digit, has space but no dash → true  (third block: Atoi fails, no '-')
func TestLooksLikeArticleNumber_RealAftermarketPositives(t *testing.T) {
	positives := []string{
		// ── Letter + digit, no separator ────────────────────────────────────
		// OEM 26300-35505 crossrefs
		"LS489A",    // PURFLUX (legacyArticleId 19460777)
		"PH6811",    // FRAM    (legacyArticleId 26475081)
		"H13W01",    // HENGST FILTER (legacyArticleId 38011051)
		// OEM 97133-D3000 crossrefs
		"J1340529",  // HERTH+BUSS JAKOPARTS cabin filter (legacyArticleId 206947358)
		"E4961LI",   // HENGST FILTER cabin filter (legacyArticleId 436188559)
		// Additional live-API confirmed aftermarket articles
		"BFO4198",
		"QFL0370",
		"N1320556",  // HERTH+BUSS
		"H132I56",
		"EAF950",
		"J1320558",  // HERTH+BUSS JAKOPARTS
		"DCF577P",
		"PC8495",
		"ADG02592",
		"AH521",
		"CF12160",
		"XUH20TTi",     // mixed case — upper conversion still yields letter+digit
		"WG1462276",    // TOPRAN
		"CCH9023",
		"CBE5413",
		"PA1517",
		"PA10119",
		"FWP2233",
		"ADG09162",
		"APV2998",
		"P254005",
		"AD06R1255",    // starts 'A' → not OEM, letter+digit no dash → article
		"WG1781552",    // starts 'W' → not OEM
		"EEM4094",      // no dash — "EEM-4094" with dash would be false
		"CS0204",
		"CSR3275",
		"WG1253830",
		"BPHY2004",     // no dash — "BPHY-2004" with dash would be false
		"JQ101268",
		"J3610526",
		"EX54651D3000", // MANDO shock absorber
		"MSA010082",
		"SS8093",
		"DB78391",
		"HYK452",
		"J4890536",     // HERTH+BUSS
		"JTE1860",
		"FTR6016",
		"BTR6016",
		"HN8061011",
		// NOTE S2-T3 (BUG-6): purely-numeric strings (518408, 254850, 261141, 87662 etc.)
		// are now handled by looksLikeOEMNumber (all-digit ≥5 rule). They route OEM-first
		// then fall through to searchByArticle on miss. looksLikeArticleNumber no longer
		// claims them — removed from this list.
		//
		// ── Drive-belt codes: digit-prefix + letters + digits, no separator ─
		// Article check: hasLetter(P,K) + hasDigit, no '-'/' ' → true (second block)
		"6PK1256", // CONTITECH / GATES poly-V belt
		"6PK1255", // CONTITECH / GATES poly-V belt
	}
	for _, q := range positives {
		if !looksLikeArticleNumber(q) {
			t.Errorf("looksLikeArticleNumber(%q) = false, want true", q)
		}
	}
}

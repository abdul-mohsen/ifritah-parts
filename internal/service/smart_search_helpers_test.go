package service

import "testing"

// ─── looksLikeOEMNumber ────────────────────────────────────────────────────

// TestLooksLikeOEMNumber_PositiveCases verifies that genuine Hyundai/Kia OEM
// numbers are recognised as OEM queries.
func TestLooksLikeOEMNumber_PositiveCases(t *testing.T) {
	positives := []string{
		"26300-35505", // oil filter (canonical dash format)
		"97133-D3000", // cabin air filter
		"58101-D4A30", // brake pad
		"36100-2B000", // starter motor
		"54610-2S000", // shock absorber
		"21100-2G000", // cylinder block part
	}
	for _, q := range positives {
		if !looksLikeOEMNumber(q) {
			t.Errorf("looksLikeOEMNumber(%q) = false, want true", q)
		}
	}
}

// TestLooksLikeOEMNumber_NegativeCases verifies that aftermarket article
// numbers and free-text queries are NOT misidentified as OEM numbers.
func TestLooksLikeOEMNumber_NegativeCases(t *testing.T) {
	negatives := []string{
		"W 811/80",       // MANN-FILTER aftermarket (letter-first)
		"OC 205",         // MAHLE aftermarket (letter-first)
		"CUK 26 013",     // MANN cabin filter (letter-first)
		"cabin air filter", // free text
		"oil filter",       // free text
		"2630",            // too short (fewer than 5 digits)
		// NOTE S2-T3: "12345" was here but now routes as an OEM stem (all-digit ≥5).
		// OEM lookup misses → falls through to searchByArticle. Routing still works.
		"AB-CD-EF",        // letters, not digit-first
		"",                // empty
		"26",              // too short
	}
	for _, q := range negatives {
		if looksLikeOEMNumber(q) {
			t.Errorf("looksLikeOEMNumber(%q) = true, want false", q)
		}
	}
}

// ─── looksLikeArticleNumber ───────────────────────────────────────────────

// TestLooksLikeArticleNumber_PositiveCases verifies recognition of typical
// aftermarket article number formats (alphanumeric without dashes/spaces).
// NOTE S2-T3 (BUG-6): purely-numeric strings are now handled by looksLikeOEMNumber
// (all-digit ≥5 rule) and are NOT claimed by looksLikeArticleNumber. They still
// reach searchByArticle via the OEM-miss fallthrough — routing is correct.
func TestLooksLikeArticleNumber_PositiveCases(t *testing.T) {
	positives := []string{
		"DRA1919",    // letter+digit, no separators
		"J1320561",   // letter+digit, no separators
		// "0986035731" and "12345" removed — now routed as OEM stems (S2-T3)
		"BP1234",     // letter prefix + digits
		"F026407006", // Bosch-style article
	}
	for _, q := range positives {
		if !looksLikeArticleNumber(q) {
			t.Errorf("looksLikeArticleNumber(%q) = false, want true", q)
		}
	}
}

// TestLooksLikeArticleNumber_NegativeCases ensures that clear non-article
// patterns are not flagged as article numbers.
//
// Design note: looksLikeArticleNumber is intentionally permissive because
// looksLikeOEMNumber is checked first in Search() dispatch. Strings that are
// also valid OEM numbers (e.g. "26300-35505") are caught by the OEM check
// first; looksLikeArticleNumber classifying them as true is harmless.
func TestLooksLikeArticleNumber_NegativeCases(t *testing.T) {
	negatives := []string{
		"cabin filter",  // free text, no digits
		"oil filter",    // free text, no digits
		"AB",            // too short (< 3 chars)
		"12",            // too short (< 3 chars)
		"ABCDE",         // pure letters, no digits
	}
	for _, q := range negatives {
		if looksLikeArticleNumber(q) {
			t.Errorf("looksLikeArticleNumber(%q) = true, want false", q)
		}
	}
}

// ─── generateOEMCandidates ────────────────────────────────────────────────

// TestGenerateOEMCandidates_InsertsDashAtCorrectPositions verifies that a
// dash-less OEM number gets dashes inserted at the expected 5-char and
// 4-char split positions.
func TestGenerateOEMCandidates_InsertsDashAtCorrectPositions(t *testing.T) {
	cases := []struct {
		input        string
		mustContain  []string
	}{
		{
			input:       "2630035505", // 10 digits — the canonical oil filter without dash
			mustContain: []string{"26300-35505", "2630-035505"},
		},
		{
			input:       "123456789", // 9 chars — only 5-char split possible
			mustContain: []string{"12345-6789"},
		},
	}
	for _, tc := range cases {
		candidates := generateOEMCandidates(tc.input)
		for _, want := range tc.mustContain {
			found := false
			for _, c := range candidates {
				if c == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("generateOEMCandidates(%q): %q not in %v", tc.input, want, candidates)
			}
		}
	}
}

// TestGenerateOEMCandidates_ShortInputReturnsEmpty ensures that inputs
// shorter than 8 characters produce no candidates (avoiding out-of-bounds).
func TestGenerateOEMCandidates_ShortInputReturnsEmpty(t *testing.T) {
	for _, input := range []string{"", "123", "1234567"} {
		candidates := generateOEMCandidates(input)
		if len(candidates) != 0 {
			t.Errorf("generateOEMCandidates(%q): expected empty slice, got %v", input, candidates)
		}
	}
}

// ─── stripColorSuffix ─────────────────────────────────────────────────────

// TestStripColorSuffix_KnownSuffixesAreStripped verifies that the known
// color/region/trim codes listed in knownColorSuffixes are removed from the
// end of an OEM number, leaving the base part number intact.
func TestStripColorSuffix_KnownSuffixesAreStripped(t *testing.T) {
	cases := []struct {
		oem     string
		want    string
		stripped bool
	}{
		// MZH is in knownColorSuffixes
		{"26300-35505MZH", "26300-35505", true},
		// EB is in knownColorSuffixes
		{"77220-3M000EB", "77220-3M000", true},
		// IM is in knownColorSuffixes
		{"64110-2Y000IM", "64110-2Y000", true},
	}
	for _, tc := range cases {
		got, wasStripped := stripColorSuffix(tc.oem)
		if wasStripped != tc.stripped {
			t.Errorf("stripColorSuffix(%q): stripped=%v, want %v (got=%q)", tc.oem, wasStripped, tc.stripped, got)
		}
		if wasStripped && got != tc.want {
			t.Errorf("stripColorSuffix(%q): result=%q, want %q", tc.oem, got, tc.want)
		}
	}
}

// TestStripColorSuffix_CleanNumberUnchanged verifies that an OEM number without
// a color suffix is returned unchanged with stripped=false.
func TestStripColorSuffix_CleanNumberUnchanged(t *testing.T) {
	clean := []string{
		"26300-35505",
		"97133-D3000",
		"58101-D4A30",
	}
	for _, oem := range clean {
		got, wasStripped := stripColorSuffix(oem)
		if wasStripped {
			t.Errorf("stripColorSuffix(%q): unexpectedly stripped to %q", oem, got)
		}
	}
}

// TestStripColorSuffix_UnknownSuffixNotStripped ensures that a random suffix
// that is NOT in knownColorSuffixes is left intact.
func TestStripColorSuffix_UnknownSuffixNotStripped(t *testing.T) {
	oem := "26300-35505ZZZZ99"
	_, wasStripped := stripColorSuffix(oem)
	if wasStripped {
		t.Errorf("stripColorSuffix(%q): should not have stripped unknown suffix", oem)
	}
}

// ─── driverName ───────────────────────────────────────────────────────────

// TestDriverName covers all FitmentDriver constants and the unknown fallback.
func TestDriverName(t *testing.T) {
	cases := []struct {
		d    FitmentDriver
		want string
	}{
		{FitEngine, "engine"},
		{FitBody, "body"},
		{FitDrivetrain, "drivetrain"},
		{FitBrake, "brake"},
		{FitUniversal, "universal"},
		{FitmentDriver(99), "unknown"}, // sentinel value not in the switch
	}
	for _, tc := range cases {
		got := driverName(tc.d)
		if got != tc.want {
			t.Errorf("driverName(%d) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// ─── computeConfidenceForVehicle ─────────────────────────────────────────

// TestComputeConfidenceForVehicle exercises every driver branch and the main
// confidence ladder for FitEngine so the scoring rules are regression-tested.
func TestComputeConfidenceForVehicle(t *testing.T) {
	s := &SmartSearch{} // no DB needed — pure computation

	t.Run("FitEngine exact CC match", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitEngine, CCMargin: 500}, 2000, 2000, "", "")
		if conf != 0.95 {
			t.Errorf("exact CC: got %v, want 0.95", conf)
		}
	})

	t.Run("FitEngine no vehicle CC → 0.7", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitEngine, CCMargin: 500}, 0, 2000, "", "")
		if conf != 0.7 {
			t.Errorf("no vehicle CC: got %v, want 0.7", conf)
		}
	})

	t.Run("FitEngine no part CC → 0.7", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitEngine, CCMargin: 500}, 2000, 0, "", "")
		if conf != 0.7 {
			t.Errorf("no part CC: got %v, want 0.7", conf)
		}
	})

	t.Run("FitEngine within ±500 margin → 0.85", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitEngine, CCMargin: 500}, 2000, 1700, "", "")
		if conf != 0.85 {
			t.Errorf("within margin: got %v, want 0.85", conf)
		}
	})

	t.Run("FitEngine marginal (diff = 1.5× margin) → 0.5", func(t *testing.T) {
		// diff 750, margin 500, 750 <= 1000 → marginal 0.5
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitEngine, CCMargin: 500}, 2000, 1250, "", "")
		if conf != 0.5 {
			t.Errorf("marginal: got %v, want 0.5", conf)
		}
	})

	t.Run("FitEngine mismatch (diff >> 2× margin) → 0.2", func(t *testing.T) {
		// diff 2000, margin 500, 2000 > 1000 → mismatch 0.2
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitEngine, CCMargin: 500}, 2000, 4000, "", "")
		if conf != 0.2 {
			t.Errorf("mismatch: got %v, want 0.2", conf)
		}
	})

	t.Run("FitEngine zero CCMargin uses default 500", func(t *testing.T) {
		// CCMargin=0 should default to 500 — exact match still 0.95
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitEngine, CCMargin: 0}, 2000, 2000, "", "")
		if conf != 0.95 {
			t.Errorf("zero CCMargin exact: got %v, want 0.95", conf)
		}
	})

	t.Run("FitBody → 0.85", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitBody}, 0, 0, "", "")
		if conf != 0.85 {
			t.Errorf("body: got %v, want 0.85", conf)
		}
	})

	t.Run("FitDrivetrain → 0.80", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitDrivetrain}, 0, 0, "", "")
		if conf != 0.80 {
			t.Errorf("drivetrain: got %v, want 0.80", conf)
		}
	})

	t.Run("FitBrake no CC context → 0.75", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitBrake, CCMargin: 1000}, 0, 0, "", "")
		if conf != 0.75 {
			t.Errorf("brake no CC: got %v, want 0.75", conf)
		}
	})

	t.Run("FitBrake CC within 1000 → 0.85", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitBrake, CCMargin: 1000}, 2000, 1500, "", "")
		if conf != 0.85 {
			t.Errorf("brake within CC: got %v, want 0.85", conf)
		}
	})

	t.Run("FitBrake CC outside 1000 → 0.6", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitBrake, CCMargin: 1000}, 2000, 500, "", "")
		if conf != 0.6 {
			t.Errorf("brake outside CC: got %v, want 0.6", conf)
		}
	})

	t.Run("FitUniversal → 0.90", func(t *testing.T) {
		conf, _ := s.computeConfidenceForVehicle(
			CategoryRule{Driver: FitUniversal}, 0, 0, "", "")
		if conf != 0.90 {
			t.Errorf("universal: got %v, want 0.90", conf)
		}
	})
}

// ─── hasVehicleContext ────────────────────────────────────────────────────

// TestHasVehicleContext_Table covers the helper that gates fitment-evidence
// classification so all three context fields are independently exercised.
func TestHasVehicleContext_Table(t *testing.T) {
	cases := []struct {
		linkage  int
		cc       int
		fuel     string
		wantTrue bool
	}{
		{linkage: 10001, cc: 0, fuel: "", wantTrue: true},
		{linkage: 0, cc: 2000, fuel: "", wantTrue: true},
		{linkage: 0, cc: 0, fuel: "petrol", wantTrue: true},
		{linkage: 0, cc: 0, fuel: "", wantTrue: false},
	}
	for _, tc := range cases {
		got := hasVehicleContext(tc.linkage, tc.cc, tc.fuel)
		if got != tc.wantTrue {
			t.Errorf("hasVehicleContext(linkage=%d, cc=%d, fuel=%q) = %v, want %v",
				tc.linkage, tc.cc, tc.fuel, got, tc.wantTrue)
		}
	}
}

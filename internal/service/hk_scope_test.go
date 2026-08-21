package service

import "testing"

func TestIsHKOEM_GoldenCases(t *testing.T) {
	golden := []struct {
		oem  string
		want bool
	}{
		{"26300-35505", true},  // Hyundai/Kia oil filter — golden set
		{"97133-D3000", true},  // Hyundai/Kia Tucson cabin filter — golden set
		{"46321-3B650", true},  // Hyundai auto-trans mount (real HK OEM)
		{"54528-4A100", true},  // Kia lower ball joint
		{"92101-3S050", true},  // Hyundai Sonata headlight
		{"58101-3SA00", true},  // Hyundai Sonata front brake pad
		{"55700-3S000", true},  // Hyundai Sonata rear axle
		{"2630035505", true},   // Same number without dash
		{"26300 35505", true},  // With whitespace — should still match dashed after compact
		{"26300-35505 ", true}, // Trailing whitespace tolerated
	}
	for _, tc := range golden {
		got := IsHKOEM(tc.oem)
		if got.IsHK != tc.want {
			t.Errorf("IsHKOEM(%q) = %+v, want IsHK=%v", tc.oem, got, tc.want)
		}
	}
}

// TestIsHKOEM_MultiDashNonHK_ViaDenyList verifies that non-HK OEMs which
// FAIL the format regex (because they have more dashes than the HK 5-5
// pattern allows) are still rejected via the nonHKMakeHints deny-list.
//
// Regression test for the 2026-08-22 audit finding: BMW "11-42-7-521-353"
// has 4 dashes, matches neither hkOEMFormatDashed (`^\d{5}-[A-Z0-9]{5}$`)
// nor hkOEMFormatFlat, and used to fall through to the "unknown" branch.
// The deny-list correctly identified it as BMW via prefix "11427" — but
// the searchByOEM caller ignored the SuggestedMake result and only gated
// on Format != "unknown". After the fix (checking SuggestedMake regardless
// of Format), searchByOEM AND searchCombined reject these instantly.
func TestIsHKOEM_MultiDashNonHK_ViaDenyList(t *testing.T) {
	cases := []struct {
		oem               string
		wantIsHK          bool
		wantSuggestedMake string
	}{
		{"11-42-7-521-353", false, "BMW"}, // BMW oil filter — 4 dashes
		{"11-42-7-634-292", false, "BMW"}, // BMW variant
		{"07-11-9-905-428", false, "BMW"}, // BMW washer nozzle — 4 dashes
	}
	for _, tc := range cases {
		got := IsHKOEM(tc.oem)
		if got.IsHK != tc.wantIsHK {
			t.Errorf("IsHKOEM(%q).IsHK = %v, want %v (full: %+v)", tc.oem, got.IsHK, tc.wantIsHK, got)
		}
		if got.SuggestedMake != tc.wantSuggestedMake {
			t.Errorf("IsHKOEM(%q).SuggestedMake = %q, want %q — deny-list must fire on multi-dash form",
				tc.oem, got.SuggestedMake, tc.wantSuggestedMake)
		}
	}
}

func TestIsHKOEM_BoundaryRejects(t *testing.T) {
	// These must be rejected. If any of them classify as HK, the search
	// cascade will happily fall back to online scrape and fabricate a hit.
	rejects := []string{
		"90915-YZZE1", // Toyota oil filter
		"90915-YZZD3", // Toyota oil filter variant
		"11427634292", // BMW
		"11-42-7-634-292",
		"15208-65F0A",   // Nissan oil filter
		"15400-PLM-A02", // Honda oil filter
		"AL3Z-6584-A",   // Ford
		"F 026 407 124", // Bosch aftermarket
		"C 26 013",      // Mann aftermarket
		"OC 205",        // Mahle aftermarket
	}
	for _, r := range rejects {
		got := IsHKOEM(r)
		if got.IsHK {
			t.Errorf("IsHKOEM(%q) = IsHK=true (want false); result=%+v", r, got)
		}
		if got.Reason == "" {
			t.Errorf("IsHKOEM(%q) missing Reason (needed for user warning)", r)
		}
	}
}

func TestIsHKOEM_SuggestsMake(t *testing.T) {
	// When a non-HK OEM matches a known competitor prefix, we should tell
	// the user which make it looks like — the honest "not in scope" message.
	cases := []struct {
		oem  string
		make string
	}{
		{"90915-YZZE1", "Toyota"},
		{"15208-65F0A", "Nissan"},
		{"15400-PLM-A02", "Honda"},
		{"11427634292", "BMW"},
	}
	for _, tc := range cases {
		got := IsHKOEM(tc.oem)
		if got.SuggestedMake != tc.make {
			t.Errorf("IsHKOEM(%q).SuggestedMake = %q, want %q (full result: %+v)",
				tc.oem, got.SuggestedMake, tc.make, got)
		}
	}
}

func TestIsJunkDescription(t *testing.T) {
	junk := []string{
		"Sign up with",
		"sign in to view",
		"Please log in",
		"CLICK HERE",
		"",
		"     ",
		"Cookie Preferences",
		"life-time-filter",
	}
	for _, j := range junk {
		if !IsJunkDescription(j) {
			t.Errorf("IsJunkDescription(%q) = false (want true)", j)
		}
	}

	legit := []string{
		"FILTER ASSY-ENGINE OIL",
		"Brake pad set (front)",
		"Cabin filter with activated carbon",
		"Shock absorber (rear left)",
		"Timing chain kit — engine 1.6 T-GDI",
	}
	for _, l := range legit {
		if IsJunkDescription(l) {
			t.Errorf("IsJunkDescription(%q) = true (want false)", l)
		}
	}
}

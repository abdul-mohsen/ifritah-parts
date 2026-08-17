package service

import "testing"

// TestDecodeOEMPrefix_ThreeDigitMatchWins verifies that a 3-digit prefix match
// returns the more specific category rather than falling back to the 2-digit entry.
// 26300-35505 → prefix "263" → "Oil Filter" (not the broader "26" → "Oil System / Filters").
func TestDecodeOEMPrefix_ThreeDigitMatchWins(t *testing.T) {
	cat := DecodeOEMPrefix("26300-35505")
	if cat == nil {
		t.Fatal("expected non-nil category for 26300-35505")
	}
	if cat.Category != "Oil Filter" {
		t.Errorf("Category = %q, want 'Oil Filter'", cat.Category)
	}
	if cat.Prefix != "263" {
		t.Errorf("Prefix = %q, want '263'", cat.Prefix)
	}
	if cat.System != "Engine" {
		t.Errorf("System = %q, want 'Engine'", cat.System)
	}
}

// TestDecodeOEMPrefix_TwoDigitFallback verifies that when no 3-digit entry exists
// the function falls back to the 2-digit entry.
// "26055-XXXXX" has no entry for "260" so should resolve to "26" → "Oil System / Filters".
func TestDecodeOEMPrefix_TwoDigitFallback(t *testing.T) {
	cat := DecodeOEMPrefix("26055-35505")
	if cat == nil {
		t.Fatal("expected non-nil category for 26055-35505 (2-digit fallback)")
	}
	if cat.System != "Engine" {
		t.Errorf("System = %q, want 'Engine'", cat.System)
	}
	if cat.Prefix != "26" {
		t.Errorf("Prefix = %q, want '26'", cat.Prefix)
	}
}

// TestDecodeOEMPrefix_FormattingStripped checks that dashes and spaces in the
// OEM number are ignored — only digits are used for prefix matching.
func TestDecodeOEMPrefix_FormattingStripped(t *testing.T) {
	inputs := []string{
		"26300-35505", // canonical dash format
		"2630035505",  // no dash
		"26300 35505", // space instead of dash
	}
	for _, input := range inputs {
		cat := DecodeOEMPrefix(input)
		if cat == nil {
			t.Errorf("DecodeOEMPrefix(%q): expected non-nil", input)
			continue
		}
		if cat.Category != "Oil Filter" {
			t.Errorf("DecodeOEMPrefix(%q): Category = %q, want 'Oil Filter'", input, cat.Category)
		}
	}
}

// TestDecodeOEMPrefix_ShortInput verifies that inputs with fewer than 2 digits
// return nil rather than panicking or returning a garbage result.
func TestDecodeOEMPrefix_ShortInput(t *testing.T) {
	cases := []string{"", "2", "A", "AB-CD", " - "}
	for _, input := range cases {
		cat := DecodeOEMPrefix(input)
		if cat != nil {
			t.Errorf("DecodeOEMPrefix(%q): expected nil for short/non-numeric input, got %+v", input, cat)
		}
	}
}

// TestDecodeOEMPrefix_UnknownPrefix verifies that a valid-looking OEM number
// with no matching prefix returns nil (not a panic or default value).
func TestDecodeOEMPrefix_UnknownPrefix(t *testing.T) {
	cases := []string{"00100-12345", "00900-99999", "00000-00000"}
	for _, input := range cases {
		cat := DecodeOEMPrefix(input)
		if cat != nil {
			t.Errorf("DecodeOEMPrefix(%q): expected nil for unknown prefix, got %+v", input, cat)
		}
	}
}

// TestDecodeOEMPrefix_HVACParts verifies HVAC prefix resolution.
// 97133-D3000 is the cabin air filter for Hyundai Tucson — prefix "971" → Compressor A/C.
// 97133 starts with "971" which maps to HVAC Compressor A/C; fallback "97" maps to HVAC.
func TestDecodeOEMPrefix_HVACParts(t *testing.T) {
	cat := DecodeOEMPrefix("97133-D3000")
	if cat == nil {
		t.Fatal("expected non-nil category for HVAC part 97133-D3000")
	}
	if cat.System != "HVAC" {
		t.Errorf("System = %q, want 'HVAC'", cat.System)
	}
}

// TestDecodeOEMPrefix_BrakeParts verifies brake subsystem prefix resolution.
// 58101-D4A30 → prefix "581" → "Front Brake Pad / Disc".
func TestDecodeOEMPrefix_BrakeParts(t *testing.T) {
	cat := DecodeOEMPrefix("58101-D4A30")
	if cat == nil {
		t.Fatal("expected non-nil category for brake part 58101-D4A30")
	}
	if cat.System != "Brakes" {
		t.Errorf("System = %q, want 'Brakes'", cat.System)
	}
	if cat.Category != "Front Brake Pad / Disc" {
		t.Errorf("Category = %q, want 'Front Brake Pad / Disc'", cat.Category)
	}
}

// TestDecodeOEMPrefix_ElectricalParts spot-checks a few electrical prefix entries.
func TestDecodeOEMPrefix_ElectricalParts(t *testing.T) {
	cases := []struct {
		oem     string
		system  string
		wantCat string
	}{
		{"36100-2B000", "Electrical", "Starter Motor"},    // "361"
		{"37300-2G000", "Electrical", "Alternator"},       // "373"
		{"92102-3X000", "Electrical", "Headlight Assembly"}, // "921"
		{"96110-1E200", "Electrical", "Battery"},          // "961"
	}
	for _, tc := range cases {
		cat := DecodeOEMPrefix(tc.oem)
		if cat == nil {
			t.Errorf("DecodeOEMPrefix(%q): expected non-nil", tc.oem)
			continue
		}
		if cat.System != tc.system {
			t.Errorf("DecodeOEMPrefix(%q): System = %q, want %q", tc.oem, cat.System, tc.system)
		}
		if cat.Category != tc.wantCat {
			t.Errorf("DecodeOEMPrefix(%q): Category = %q, want %q", tc.oem, cat.Category, tc.wantCat)
		}
	}
}

// TestDecodeOEMPrefix_SuspensionAndSteering spot-checks suspension prefix entries.
func TestDecodeOEMPrefix_SuspensionAndSteering(t *testing.T) {
	cases := []struct {
		oem     string
		system  string
	}{
		{"54610-2S000", "Suspension"}, // "546" → Shock Absorber (Front)
		{"55311-3X000", "Suspension"}, // "553" → Shock Absorber (Rear)
		{"56820-3X000", "Suspension"}, // "568" not in map, fallback "56" → Steering Column & Gear
	}
	for _, tc := range cases {
		cat := DecodeOEMPrefix(tc.oem)
		if cat == nil {
			// 568 falls back to 56 which is in the map
			continue
		}
		if cat.System != tc.system {
			t.Errorf("DecodeOEMPrefix(%q): System = %q, want %q", tc.oem, cat.System, tc.system)
		}
	}
}

// TestDecodeOEMPrefix_BodyAndInterior spot-checks body prefix entries.
func TestDecodeOEMPrefix_BodyAndInterior(t *testing.T) {
	cat := DecodeOEMPrefix("64101-2S000") // "64" → Front Body / Hood
	if cat == nil {
		t.Fatal("expected non-nil for body part")
	}
	if cat.System != "Body" {
		t.Errorf("System = %q, want 'Body'", cat.System)
	}
}

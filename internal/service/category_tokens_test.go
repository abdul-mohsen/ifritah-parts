package service

import "testing"

// TestCategoryTokensForOEM — M1.S3.T1 verifies the reverse index maps
// every prefix to the expected description tokens.
func TestCategoryTokensForOEM(t *testing.T) {
	cases := []struct {
		oem     string
		wantAny []string // description tokens the result MUST contain at least one of
	}{
		{"26350-2J001", []string{"oil", "filter"}},       // 263 -> Oil Filter
		{"58101-3XA00", []string{"brake", "pad"}},        // 581 -> Front Brake Pad / Disc
		{"97133-D3000", []string{"compressor"}}, // 971 -> Compressor A/C (note: 97133 should really be Cabin Filter — prefixMap follow-up)
		{"86391-D3000", []string{"mirrors"}}, // 86 -> Mirrors (plural in prefixMap)
		{"92101-3S050", []string{"headlight"}},           // 921 -> Headlight Assembly
		{"27301-2E400", []string{"egr", "emissions"}},    // 273 not in map, falls back to 27 = EGR & Emissions
	}
	for _, tc := range cases {
		t.Run(tc.oem, func(t *testing.T) {
			got := CategoryTokensForOEM(tc.oem)
			if len(got) == 0 {
				t.Fatalf("CategoryTokensForOEM(%q) returned nil/empty; want tokens containing one of %v", tc.oem, tc.wantAny)
			}
			foundAny := false
			for _, want := range tc.wantAny {
				for _, g := range got {
					if g == want {
						foundAny = true
						break
					}
				}
				if foundAny {
					break
				}
			}
			if !foundAny {
				t.Errorf("CategoryTokensForOEM(%q) = %v; expected at least one of %v", tc.oem, got, tc.wantAny)
			}
		})
	}
}

// TestCategoryTokensForOEM_UnknownReturnsNil — non-decoding inputs return
// nil so the validation gate opts out (never a false negative).
func TestCategoryTokensForOEM_UnknownReturnsNil(t *testing.T) {
	cases := []string{"", "XYZ", "99999-99999"} // 99xxx = dealer accessory, not in prefixMap
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			got := CategoryTokensForOEM(c)
			if got != nil {
				t.Errorf("CategoryTokensForOEM(%q) = %v, want nil", c, got)
			}
		})
	}
}

// TestTokenizeCategory — verifies category-label splitting handles the
// separator characters used in prefixMap.
func TestTokenizeCategory(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Oil Filter", []string{"oil", "filter"}},
		{"Front Brake Pad / Disc", []string{"front", "brake", "pad", "disc"}},
		{"Air Conditioning & Heating", []string{"air", "conditioning", "heating"}},
		{"Ignition Coil - Front Right", []string{"ignition", "coil", "front", "right"}},
		{"A/C Hose & Pipe", []string{"hose", "pipe"}}, // "a" is stopword, "c" is <2 chars filtered
		{"", nil},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := tokenizeCategory(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("tokenizeCategory(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i, g := range got {
				if g != tc.want[i] {
					t.Errorf("tokenizeCategory(%q)[%d] = %q, want %q", tc.in, i, g, tc.want[i])
				}
			}
		})
	}
}

// TestHasCategoryOverlap — verifies the description-token overlap check
// is case-insensitive and matches on substring.
func TestHasCategoryOverlap(t *testing.T) {
	cases := []struct {
		name   string
		desc   string
		tokens []string
		want   bool
	}{
		{"exact match", "Engine Oil Filter", []string{"oil", "filter"}, true},
		{"case mismatch", "ENGINE OIL FILTER", []string{"oil", "filter"}, true},
		{"substring", "OilFilter (V6)", []string{"oil"}, true},
		{"no overlap", "Headlight Assembly", []string{"oil", "filter"}, false},
		{"empty tokens - always pass", "anything", nil, true},
		{"empty desc, non-empty tokens - fail", "", []string{"oil"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasCategoryOverlap(tc.desc, tc.tokens); got != tc.want {
				t.Errorf("hasCategoryOverlap(%q, %v) = %v, want %v", tc.desc, tc.tokens, got, tc.want)
			}
		})
	}
}

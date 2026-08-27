package handler

import (
	"testing"
)

// TestNormalizeCatalogArg — the M0.T4 fix. vehicle_lookup stores nhtsa_make
// and nhtsa_model as UPPERCASE (populated by scripts/derive_hk_maps:
// `'HYUNDAI'`, `'KIA'`, `'TUCSON'`, `'ELANTRA'`, etc.). The old handler
// passed user input straight through so `?make=Hyundai&model=Elantra`
// silently matched zero rows. Normaliser applied → matches cleanly.
func TestNormalizeCatalogArg(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"HYUNDAI", "HYUNDAI"},                     // already correct
		{"Hyundai", "HYUNDAI"},                     // browser sends title case
		{"hyundai", "HYUNDAI"},                     // lowercase
		{"  hyundai  ", "HYUNDAI"},                 // stray whitespace
		{"\tHYUNDAI\n", "HYUNDAI"},                 // tabs / newlines
		{"Elantra", "ELANTRA"},                     // model title case
		{"i30", "I30"},                             // model with digits
		{"cee'd", "CEE'D"},                         // preserves apostrophes
		{"tucson ", "TUCSON"},                      // trailing space only
		{" ", ""},                                  // whitespace-only → empty
		{"HYUNDAI (BEIJING)", "HYUNDAI (BEIJING)"}, // spaces + parens preserved
	}
	for _, tc := range cases {
		got := normalizeCatalogArg(tc.input)
		if got != tc.want {
			t.Errorf("normalizeCatalogArg(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

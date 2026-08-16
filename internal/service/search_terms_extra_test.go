package service

import "testing"

// normalizeTextSearchQuery behaviour recap (from search_terms.go):
//  1. strings.ToLower(strings.TrimSpace(query))
//  2. alias map lookup on the FULL lowercased string — BEFORE splitting
//     aliases: "cabin air filter" → "cabin filter"
//              "pollen filter"   → "cabin filter"
//  3. strings.FieldsFunc — splits on every rune that is not letter/digit
//  4. deduplication (first-occurrence wins)
//  5. strings.Join with " "
//
// Consequence: "cabin-air-filter" does NOT trigger the alias because the
// alias key "cabin air filter" uses spaces; after FieldsFunc the result is
// "cabin air filter" (not aliased).

// TestNormalizeTextSearchQuery_AllAliasVariants covers every known alias and its
// case variants, plus common non-alias queries. 42 sub-tests.
func TestNormalizeTextSearchQuery_AllAliasVariants(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// ── cabin air filter alias (7 variants) ──────────────────────────────
		{"cabin air filter", "cabin filter"},
		{"Cabin Air Filter", "cabin filter"},
		{"CABIN AIR FILTER", "cabin filter"},
		{"Cabin air FILTER", "cabin filter"},
		{"CABIN air filter", "cabin filter"},
		{"cabin AIR filter", "cabin filter"},
		{"cabin air FILTER", "cabin filter"},

		// ── pollen filter alias (3 variants) ─────────────────────────────────
		{"pollen filter", "cabin filter"},
		{"Pollen Filter", "cabin filter"},
		{"POLLEN FILTER", "cabin filter"},
		{"pollen FILTER", "cabin filter"},
		{"POLLEN filter", "cabin filter"},

		// ── "cabin filter" literal — NOT in alias map as a key ───────────────
		// splits to ["cabin","filter"] → "cabin filter"
		{"cabin filter", "cabin filter"},
		{"Cabin Filter", "cabin filter"},

		// ── leading / trailing whitespace stripped before alias check ─────────
		{"  cabin air filter  ", "cabin filter"},
		{"  cabin  filter  ", "cabin filter"},

		// ── non-alias queries ─────────────────────────────────────────────────
		{"oil filter", "oil filter"},
		{"Oil Filter", "oil filter"},
		{"OIL FILTER", "oil filter"},
		{"oil  filter", "oil filter"},   // double space → FieldsFunc merges
		{"  oil filter  ", "oil filter"}, // trim then split
		{"Oil-Filter", "oil filter"},
		{"oil-filter", "oil filter"},
		{"OIL-FILTER", "oil filter"},
		{"brake pad", "brake pad"},
		{"BRAKE PAD", "brake pad"},
		{"Brake Pad Set", "brake pad set"},
		{"BRAKE PAD SET", "brake pad set"},
		{"shock absorber", "shock absorber"},
		{"SHOCK ABSORBER", "shock absorber"},
		{"Shock Absorber Front", "shock absorber front"},
		{"ignition coil", "ignition coil"},
		{"Ignition Coil", "ignition coil"},
		{"water pump", "water pump"},
		{"Water Pump", "water pump"},
		{"timing chain", "timing chain"},
		{"Timing Chain Kit", "timing chain kit"},
		{"TIMING CHAIN KIT", "timing chain kit"},
		{"WATER PUMP", "water pump"},
		{"IGNITION coil", "ignition coil"},
		{"BRAKE pad", "brake pad"},
		{"Shock ABSORBER", "shock absorber"},
		{"oil  Filter", "oil filter"}, // double space + mixed case
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			if got := normalizeTextSearchQuery(tc.input); got != tc.want {
				t.Errorf("normalizeTextSearchQuery(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeTextSearchQuery_TokenDeduplication verifies first-occurrence
// deduplication: repeated tokens are dropped, order is preserved. 30 sub-tests.
func TestNormalizeTextSearchQuery_TokenDeduplication(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// ── single token repeated ────────────────────────────────────────────
		{"oil oil", "oil"},
		{"Oil Oil", "oil"},
		{"filter filter", "filter"},
		{"brake brake", "brake"},
		{"oil oil oil", "oil"},       // triple
		{"filter filter filter", "filter"},
		{"OIL OIL", "oil"},
		{"BRAKE BRAKE", "brake"},

		// ── two-token, one repeated ───────────────────────────────────────────
		{"oil filter oil", "oil filter"},    // second "oil" dropped
		{"brake pad brake", "brake pad"},    // second "brake" dropped
		{"oil filter filter", "oil filter"}, // second "filter" dropped
		{"brake brake pad", "brake pad"},    // second "brake" dropped
		{"ignition coil ignition", "ignition coil"},
		{"water pump water", "water pump"},
		{"timing chain timing", "timing chain"},

		// ── three-token, one repeated ─────────────────────────────────────────
		{"brake pad set brake", "brake pad set"},
		{"shock absorber shock", "shock absorber"},
		{"Oil Oil Filter", "oil filter"},    // "oil" deduped, "filter" kept
		{"Filter Filter Oil", "filter oil"},

		// ── dedup with alias already applied ─────────────────────────────────
		// "cabin air filter" → alias → "cabin filter" → tokens ["cabin","filter"] → no dupe
		// (the alias result itself has no duplicates)

		// ── single unique tokens (baseline) ──────────────────────────────────
		{"oil", "oil"},
		{"brake", "brake"},
		{"filter", "filter"},
		{"absorber", "absorber"},
		{"coil", "coil"},

		// ── cabin air cabin: alias NOT applied (full string ≠ "cabin air filter")
		// split → ["cabin","air","cabin"] → dedup → ["cabin","air"] → "cabin air"
		{"cabin air cabin", "cabin air"},

		// ── multi-word with repeated token ───────────────────────────────────
		{"starter starter motor", "starter motor"},
		{"sensor sensor value", "sensor value"},
		{"pump oil pump", "pump oil"},
		{"disc disc brake", "disc brake"},
		{"pad pad set", "pad set"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			if got := normalizeTextSearchQuery(tc.input); got != tc.want {
				t.Errorf("normalizeTextSearchQuery(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeTextSearchQuery_Separators verifies that dashes, slashes, dots,
// and underscores are all treated as token separators by FieldsFunc.
// IMPORTANT: "cabin-air-filter" does NOT alias to "cabin filter" because the
// alias check runs on the raw lowercased string "cabin-air-filter" — which does
// not equal the alias key "cabin air filter". The result is "cabin air filter".
// 22 sub-tests.
func TestNormalizeTextSearchQuery_Separators(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// ── dash ─────────────────────────────────────────────────────────────
		{"oil-filter", "oil filter"},
		{"brake-pad", "brake pad"},
		{"BRAKE-PAD", "brake pad"},
		{"shock-absorber-front", "shock absorber front"},
		{"timing-chain-kit", "timing chain kit"},
		{"26300-35505", "26300 35505"},

		// ── slash ────────────────────────────────────────────────────────────
		{"oil/filter", "oil filter"},
		{"shock/absorber", "shock absorber"},
		{"ignition/coil", "ignition coil"},
		{"slashes/in/query", "slashes in query"},

		// ── dot ──────────────────────────────────────────────────────────────
		{"oil.filter", "oil filter"},
		{"brake.pad", "brake pad"},
		{"water.pump", "water pump"},
		{"dots.and.dashes-mixed", "dots and dashes mixed"},

		// ── underscore (not letter, not digit → separator) ────────────────────
		{"oil_filter", "oil filter"},
		{"underscore_and-mixed", "underscore and mixed"},

		// ── mixed separators ─────────────────────────────────────────────────
		{"oil filter.26300", "oil filter 26300"},

		// ── cabin-air-filter: alias NOT triggered (raw string ≠ alias key) ───
		// "cabin-air-filter" → ToLower → "cabin-air-filter"
		// alias["cabin-air-filter"] = miss
		// FieldsFunc → ["cabin","air","filter"] → "cabin air filter"
		{"cabin-air-filter", "cabin air filter"},

		// ── separators only → empty result ───────────────────────────────────
		{"---", ""},
		{"...", ""},
		{"-/-", ""},

		// ── multi-word with mixed separators ─────────────────────────────────
		{"multi-word/term.here", "multi word term here"},
		{"a-b_c.d/e", "a b c d e"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			if got := normalizeTextSearchQuery(tc.input); got != tc.want {
				t.Errorf("normalizeTextSearchQuery(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeTextSearchQuery_OEMSearchPatterns verifies that OEM-style inputs
// (bare numbers, hyphenated OEM codes, OEM mixed with keyword terms) behave
// predictably. Key: "cabin air filter 97133" is NOT aliased because the full
// lowercased string does not exactly match any alias key. 20 sub-tests.
func TestNormalizeTextSearchQuery_OEMSearchPatterns(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// ── bare OEM-like numbers (no separators) ────────────────────────────
		{"26300", "26300"},
		{"97133", "97133"},
		{"58101", "58101"},
		{"54651", "54651"},
		{"86511", "86511"},
		{"56820", "56820"},

		// ── hyphenated OEM: dash is separator → two tokens ───────────────────
		{"26300-35505", "26300 35505"},
		{"97133-D3000", "97133 d3000"},
		{"92101-D3100", "92101 d3100"},
		{"28113-D3100", "28113 d3100"},

		// ── OEM mixed with keyword terms ─────────────────────────────────────
		{"oil filter 26300", "oil filter 26300"},
		{"filter 97133", "filter 97133"},
		{"part 26300", "part 26300"},
		{"26300 oil", "26300 oil"},
		{"oil filter 58101", "oil filter 58101"},
		{"brake 58101-D3A70", "brake 58101 d3a70"},
		{"OEM 97133", "oem 97133"},
		{"filter OEM number", "filter oem number"},

		// ── "cabin air filter" + OEM: full string ≠ alias key → NOT aliased ──
		// FieldsFunc → ["cabin","air","filter","97133"] → join unchanged
		{"cabin air filter 97133", "cabin air filter 97133"},

		// ── OEM with no letters: stays numeric after lowercase ────────────────
		{"OEM26300", "oem26300"}, // no separator → single token
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			if got := normalizeTextSearchQuery(tc.input); got != tc.want {
				t.Errorf("normalizeTextSearchQuery(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeTextSearchQuery_AftMarketSearchTerms verifies that brand names
// and common aftermarket search patterns normalize correctly (lowercased,
// separators replaced, deduped). 22 sub-tests.
func TestNormalizeTextSearchQuery_AftMarketSearchTerms(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// ── brand names alone ────────────────────────────────────────────────
		{"MANN filter", "mann filter"},
		{"mahle", "mahle"},
		{"bosch", "bosch"},
		{"TRW", "trw"},
		{"brembo", "brembo"},
		{"bilstein", "bilstein"},
		{"KYB", "kyb"},
		{"sachs", "sachs"},
		{"Valeo", "valeo"},
		{"FEBI", "febi"},
		{"GATES", "gates"},
		{"SKF", "skf"},
		{"NTN", "ntn"},
		{"NSK", "nsk"},

		// ── brand + part type combinations ───────────────────────────────────
		{"MANN oil filter", "mann oil filter"},
		{"NGK spark plug", "ngk spark plug"},
		{"BOSCH oil filter", "bosch oil filter"},
		{"Mahle cabin filter", "mahle cabin filter"},
		{"valeo brake pad", "valeo brake pad"},
		{"SKF wheel bearing", "skf wheel bearing"},
		{"Gates timing belt", "gates timing belt"},

		// ── "DENSO cabin filter": full string ≠ any alias key → not aliased ──
		// ToLower → "denso cabin filter" → no alias → split → "denso cabin filter"
		{"DENSO cabin filter", "denso cabin filter"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			if got := normalizeTextSearchQuery(tc.input); got != tc.want {
				t.Errorf("normalizeTextSearchQuery(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

package service

import (
	"testing"

	"parts-engine/internal/model"
)

// TestBrandTier — canonical brands sort into their expected tier bucket.
func TestBrandTier(t *testing.T) {
	cases := []struct {
		brand string
		want  int
	}{
		{"Bosch", 1},
		{"MANN-FILTER", 1},
		{"MAHLE", 1},
		{"Denso", 1},
		{"Mobis", 1},
		{"Hyundai/Kia", 1},
		{"Textar", 1},
		{"Brembo", 1},
		{"Meyle", 2},
		{"Febi", 2},
		{"SKF", 2},
		{"Gates", 2},
		{"KYB", 2},
		{"Nissens", 2},
		{"Some Cheap Brand", 3},
		{"", 3},
	}
	for _, tc := range cases {
		t.Run(tc.brand, func(t *testing.T) {
			if got := BrandTier(tc.brand); got != tc.want {
				t.Errorf("BrandTier(%q) = %d, want %d", tc.brand, got, tc.want)
			}
		})
	}
}

// TestSortAftermarketByTier — mixed input sorts Tier 1 → 2 → 3, then
// alphabetically inside a tier.
func TestSortAftermarketByTier(t *testing.T) {
	parts := []model.AftermarketPart{
		{PartNumber: "P1", Brand: "Some Cheap Brand"}, // Tier 3
		{PartNumber: "P2", Brand: "Meyle"},            // Tier 2
		{PartNumber: "P3", Brand: "Bosch"},            // Tier 1
		{PartNumber: "P4", Brand: "MANN-FILTER"},      // Tier 1
		{PartNumber: "P5", Brand: "Febi"},             // Tier 2
	}
	SortAftermarketByTier(parts)

	// Tier 1 (Bosch, MANN-FILTER) before Tier 2 (Febi, Meyle) before Tier 3
	wantOrder := []string{"Bosch", "MANN-FILTER", "Febi", "Meyle", "Some Cheap Brand"}
	for i, want := range wantOrder {
		if parts[i].Brand != want {
			t.Errorf("parts[%d].Brand = %q, want %q (full order: %v)", i, parts[i].Brand, want, brandsOf(parts))
		}
	}
}

// TestCapAftermarketList — enforces max total + max per brand.
func TestCapAftermarketList(t *testing.T) {
	// 40 Bosch + 40 MANN + 10 Meyle. Cap at 20 total / 3 per brand.
	parts := make([]model.AftermarketPart, 0, 90)
	for i := 0; i < 40; i++ {
		parts = append(parts, model.AftermarketPart{PartNumber: sprintfN("BOSCH-%02d", i), Brand: "Bosch"})
	}
	for i := 0; i < 40; i++ {
		parts = append(parts, model.AftermarketPart{PartNumber: sprintfN("MANN-%02d", i), Brand: "MANN-FILTER"})
	}
	for i := 0; i < 10; i++ {
		parts = append(parts, model.AftermarketPart{PartNumber: sprintfN("MEYLE-%02d", i), Brand: "Meyle"})
	}

	out := CapAftermarketList(parts, 20, 3)

	if len(out) > 20 {
		t.Errorf("len(out) = %d, want <= 20", len(out))
	}

	brandCounts := map[string]int{}
	for _, p := range out {
		brandCounts[NormalizeBrand(p.Brand)]++
	}
	for b, c := range brandCounts {
		if c > 3 {
			t.Errorf("brand %q appears %d times, max is 3", b, c)
		}
	}

	// Should have at most 3 of each of the three brands = 9 items total
	// (3 brands × 3 max each). To hit the 20-cap you'd need 7+ brands.
	if len(out) != 9 {
		t.Errorf("len(out) = %d, expected 9 (3 brands × 3 max)", len(out))
	}
}

// TestCapAftermarketList_SmallInput — when input < maxTotal, all pass through.
func TestCapAftermarketList_SmallInput(t *testing.T) {
	parts := []model.AftermarketPart{
		{PartNumber: "A", Brand: "Bosch"},
		{PartNumber: "B", Brand: "MANN-FILTER"},
		{PartNumber: "C", Brand: "Meyle"},
	}
	out := CapAftermarketList(parts, 20, 3)
	if len(out) != 3 {
		t.Errorf("len(out) = %d, want 3", len(out))
	}
}

// TestCapAftermarketList_SingleBrandCapped — 5 Bosch entries capped at 3.
func TestCapAftermarketList_SingleBrandCapped(t *testing.T) {
	parts := []model.AftermarketPart{
		{PartNumber: "B1", Brand: "Bosch"},
		{PartNumber: "B2", Brand: "Bosch"},
		{PartNumber: "B3", Brand: "Bosch"},
		{PartNumber: "B4", Brand: "Bosch"},
		{PartNumber: "B5", Brand: "Bosch"},
	}
	out := CapAftermarketList(parts, 20, 3)
	if len(out) != 3 {
		t.Errorf("len(out) = %d, want 3 (single brand should be capped)", len(out))
	}
}

// helper: build a []string of brand values in slice order for error msgs.
func brandsOf(parts []model.AftermarketPart) []string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = p.Brand
	}
	return out
}

// sprintfN is a tiny helper to avoid the fmt import in tests.
func sprintfN(format string, i int) string {
	// Simple base-10 int formatter; sufficient for test PartNumber uniqueness.
	if i == 0 {
		return format[:len(format)-4] + "00"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	if len(digits) < 2 {
		digits = "0" + digits
	}
	return format[:len(format)-4] + digits
}

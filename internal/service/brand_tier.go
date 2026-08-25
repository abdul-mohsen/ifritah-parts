package service

import (
	"sort"
	"strings"

	"parts-engine/internal/model"
)

// BrandTier categorises an aftermarket brand into a quality tier so
// combined-mode results surface the best brands first. Tiers:
//
//	Tier 1 = OEM + top-10 aftermarket (household names)
//	Tier 2 = Reputable specialists (Meyle, Febi, Blue Print, SKF etc.)
//	Tier 3 = Everything else
//
// Returns 3 for unknown / non-canonical brands so they sort last.
// Introduced in M2.S2.T1 to give parts sellers a clear ranking within
// the aftermarket alternatives.
func BrandTier(canonical string) int {
	if _, ok := brandTier1[canonical]; ok {
		return 1
	}
	if _, ok := brandTier2[canonical]; ok {
		return 2
	}
	return 3
}

// brandTier1 - household names. Order-of-magnitude bigger install base
// than Tier 2. Parts sellers stock these first.
var brandTier1 = map[string]bool{
	"Mobis":       true,
	"Hyundai":     true,
	"Kia":         true,
	"Hyundai/Kia": true,
	"Bosch":       true,
	"MANN-FILTER": true,
	"MAHLE":       true,
	"Denso":       true,
	"NGK":         true,
	"Valeo":       true,
	"Hella":       true,
	"Textar":      true,
	"Ferodo":      true,
	"TRW":         true,
	"Brembo":      true,
}

// brandTier2 - reputable second-tier specialists. Well-known within the
// trade but not top-of-mind for retail buyers.
var brandTier2 = map[string]bool{
	"Febi":            true,
	"Meyle":           true,
	"Blue Print":      true,
	"Contitech":       true,
	"Gates":           true,
	"Dayco":           true,
	"SKF":             true,
	"FAG":             true,
	"INA":             true,
	"Koyo":            true,
	"NSK":             true,
	"NTN":             true,
	"KYB":             true,
	"Monroe":          true,
	"Sachs":           true,
	"Bilstein":        true,
	"Lemforder":       true,
	"Moog":            true,
	"ATE":             true,
	"Bendix":          true,
	"Champion":        true,
	"Beru":            true,
	"Delphi":          true,
	"Magneti Marelli": true,
	"Osram":           true,
	"Philips":         true,
	"Hengst":          true,
	"Filtron":         true,
	"WIX":             true,
	"Fram":            true,
	"NRF":             true,
	"Nissens":         true,
	"Triscan":         true,
	"Ruville":         true,
	"Master-Sport":    true,
	"Aisin":           true,
	"Hitachi":         true,
	"Mando":           true,
	"Hanon Systems":   true,
	"BorgWarner":      true,
	"Herth+Buss":      true,
}

// SortAftermarketByTier orders a slice of AftermarketPart:
//   - Tier ascending (1 first)
//   - then Brand ascending
//   - then PartNumber ascending
//
// Stable within tier so a caller that pre-sorted by relevance keeps
// intra-tier ordering.
func SortAftermarketByTier(parts []model.AftermarketPart) {
	sort.SliceStable(parts, func(i, j int) bool {
		ti := BrandTier(NormalizeBrand(parts[i].Brand))
		tj := BrandTier(NormalizeBrand(parts[j].Brand))
		if ti != tj {
			return ti < tj
		}
		if parts[i].Brand != parts[j].Brand {
			return parts[i].Brand < parts[j].Brand
		}
		return parts[i].PartNumber < parts[j].PartNumber
	})
}

// CapAftermarketList enforces two limits on a sorted aftermarket list:
//   - maxTotal: never return more than this many entries (default 20).
//   - maxPerBrand: prevent one brand from dominating (default 3).
//
// Preserves input order — expects the caller has already sorted by
// preferred priority (typically SortAftermarketByTier).
func CapAftermarketList(parts []model.AftermarketPart, maxTotal, maxPerBrand int) []model.AftermarketPart {
	if maxTotal <= 0 {
		maxTotal = 20
	}
	if maxPerBrand <= 0 {
		maxPerBrand = 3
	}
	perBrand := make(map[string]int, 16)
	out := make([]model.AftermarketPart, 0, maxTotal)
	for _, p := range parts {
		if len(out) >= maxTotal {
			break
		}
		key := NormalizeBrand(p.Brand)
		if perBrand[key] >= maxPerBrand {
			continue
		}
		perBrand[key]++
		out = append(out, p)
	}
	return out
}

// containsFold returns true when needle appears in haystack case-insensitively.
// Kept as a helper for tier tests that don't want the strings package import.
func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

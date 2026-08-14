package service

import (
	"regexp"
	"strings"
)

// HKScopeResult reports whether an OEM part number is inside the
// Hyundai/Kia scope this service supports. The result is used by the search
// cascade to decide whether it may fall back to online/dealer/supersession
// lookups.
//
// Design principle (README.md: "False positives cost more than false negatives"):
// when the format or prefix does not match the HK convention, return zero
// results with an honest, actionable message rather than a fabricated hit.
type HKScopeResult struct {
	IsHK           bool   // true when the number matches HK format AND has a known HK prefix
	Format         string // "hk_5_5" | "hk_5_5_dashed" | "hk_generic" | "unknown"
	SuggestedMake  string // best-effort guess for the make when the OEM is NOT HK (empty if unknown)
	Reason         string // one-line human-readable explanation for logging + user warning
}

// Strict HK format:
//   5 decimal digits, optional dash, 5 alphanumeric characters.
// Examples that MATCH: 26300-35505, 97133-D3000, 5452 8-4A100 (after
// whitespace strip), 26300-35505.
// Examples that DO NOT MATCH: 90915-YZZE1 (Toyota, 5-5 alphanumeric but wrong
// prefix), 11-42-7-509-125 (BMW), AL3Z-6584-A (Ford), 90111-06153 (Nissan).
var hkOEMFormatDashed = regexp.MustCompile(`^\d{5}-[A-Z0-9]{5}$`)
var hkOEMFormatFlat   = regexp.MustCompile(`^\d{5}[A-Z0-9]{5}$`)

// Non-HK make hints — high-signal starting fragments that let us suggest a
// specific competitor make when we reject the query. Keep this deny-list
// tight; a false positive here just means we lose the polite hint, not that
// we surface bad data.
var nonHKMakeHints = []struct {
	prefixNormalized string
	make             string
}{
	// Toyota part numbers: 90915-YZZE1 (filter), 44001-06110, 42431-08040, etc.
	// Toyota also uses 90xxx-YZZxx family for oil filters, and 43xxx / 44xxx
	// for suspension. Since 43xx and 44xx collide with HK "43" transmission,
	// we only surface Toyota when the pattern is 909xx-YZZ.
	{"90915", "Toyota"},
	{"9091", "Toyota"},
	{"9091514", "Toyota"},
	// BMW/Mini: 11-42-xxx patterns; prefixes 07, 11, 13, 17.
	{"11427", "BMW"},
	{"11421", "BMW"},
	{"07119", "BMW"},
	// Nissan: 15208-xxxxx (oil filter), 22448-xxxxx (coil), etc.
	{"15208", "Nissan"},
	{"22448", "Nissan"},
	// Honda: 15400-xxxxx (oil filter), 17220-xxxxx (air filter).
	{"15400", "Honda"},
	{"17220", "Honda"},
	// Mazda/Ford non-latin prefixes are covered by the format regex rejecting
	// alpha starts (Ford AL3Z-…, Mazda LF01-…).
}

// IsHKOEM classifies whether a query is a Hyundai/Kia OEM part number.
//
// It returns HKScopeResult with:
//   - IsHK=true if BOTH the format matches AND the prefix is in the HK
//     prefix map (see oem_prefix.go). This is the strict path the
//     Kia/Hyundai catalog uses.
//   - IsHK=false with a SuggestedMake / Reason when we recognize the OEM
//     as belonging to another marque. Reason is safe to surface to the user.
//   - IsHK=false with Reason="Unknown OEM format" when we cannot classify.
func IsHKOEM(rawOEM string) HKScopeResult {
	trimmed := strings.ToUpper(strings.TrimSpace(rawOEM))
	if trimmed == "" {
		return HKScopeResult{Format: "unknown", Reason: "Empty query"}
	}

	// Strip internal whitespace but keep the dash if present, since the
	// format regexes look at it explicitly. `26300 - 35505` → `26300-35505`.
	compact := strings.ReplaceAll(trimmed, " ", "")

	format := "unknown"
	switch {
	case hkOEMFormatDashed.MatchString(compact):
		format = "hk_5_5_dashed"
	case hkOEMFormatFlat.MatchString(compact):
		format = "hk_5_5"
	}

	// Not the HK 5-5 pattern — try to suggest a make from the deny-list.
	if format == "unknown" {
		normalized := NormalizeOEM(compact) // strips dashes, dots, slashes, spaces; lowercases
		normalizedUpper := strings.ToUpper(normalized)
		for _, hint := range nonHKMakeHints {
			if strings.HasPrefix(normalizedUpper, strings.ToUpper(hint.prefixNormalized)) {
				return HKScopeResult{
					IsHK:          false,
					Format:        "unknown",
					SuggestedMake: hint.make,
					Reason:        "This app searches Hyundai/Kia parts only. This OEM looks like a " + hint.make + " part.",
				}
			}
		}
		return HKScopeResult{
			IsHK:   false,
			Format: "unknown",
			Reason: "This app searches Hyundai/Kia parts only. This query does not match the HK OEM format (5 digits, dash, 5 characters).",
		}
	}

	// Format matches. Now confirm the prefix is in the HK map.
	// DecodeOEMPrefix strips non-digits and looks up the first 2-3 digits.
	// It returns nil for prefixes that Hyundai/Kia does not use (e.g. Toyota "90").
	prefixCat := DecodeOEMPrefix(compact)
	if prefixCat == nil {
		// Format looks HK-like but prefix is unknown to us. Try the
		// suggested-make deny-list once more in case the prefix collides
		// with a non-HK numbering scheme we know about.
		normalizedUpper := strings.ToUpper(NormalizeOEM(compact))
		for _, hint := range nonHKMakeHints {
			if strings.HasPrefix(normalizedUpper, strings.ToUpper(hint.prefixNormalized)) {
				return HKScopeResult{
					IsHK:          false,
					Format:        format,
					SuggestedMake: hint.make,
					Reason:        "This app searches Hyundai/Kia parts only. This OEM prefix belongs to " + hint.make + ".",
				}
			}
		}
		return HKScopeResult{
			IsHK:   false,
			Format: format,
			Reason: "This OEM format looks HK-shaped but the prefix is not in the Hyundai/Kia catalog.",
		}
	}

	return HKScopeResult{
		IsHK:   true,
		Format: format,
		Reason: "Matches Hyundai/Kia OEM format and prefix (" + prefixCat.System + " / " + prefixCat.Category + ").",
	}
}

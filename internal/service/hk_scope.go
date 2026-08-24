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
	IsHK          bool   // true when the number matches HK format AND has a known HK prefix
	Format        string // "hk_5_5" | "hk_5_5_dashed" | "hk_generic" | "unknown"
	SuggestedMake string // best-effort guess for the make when the OEM is NOT HK (empty if unknown)
	Reason        string // one-line human-readable explanation for logging + user warning
}

// Strict HK format:
//   5 decimal digits, optional dash, 5 alphanumeric characters.
// Examples that MATCH: 26300-35505, 97133-D3000, 5452 8-4A100 (after
// whitespace strip), 26300-35505.
// Examples that DO NOT MATCH: 90915-YZZE1 (Toyota, 5-5 alphanumeric but wrong
// prefix), 11-42-7-509-125 (BMW), AL3Z-6584-A (Ford), 90111-06153 (Nissan).
var hkOEMFormatDashed = regexp.MustCompile(`^\d{5}-[A-Z0-9]{5}$`)
var hkOEMFormatFlat = regexp.MustCompile(`^\d{5}[A-Z0-9]{5}$`)

// Non-HK make hints — high-signal starting fragments that let us reject the
// query with a specific competitor make suggestion. Widened in M1.S2 after
// the 2026-08-23 audit showed 38 of 100 non-HK OEMs leaked through the
// guard when their prefixes had no deny-list entry.
//
// Keep entries tight; a false positive here rejects a valid query. When a
// candidate prefix collides with a real HK prefix (e.g. 44xxx = HK
// Transmission Control), leave it out.
var nonHKMakeHints = []struct {
	prefixNormalized string
	make             string
}{
	// ─── Toyota ───────────────────────────────────────────────────────
	// 90915-YZZxx family = oil filters; 90080-* = fasteners; 90118-* =
	// bolts; 88310-* = A/C compressor. 43xxx/44xxx collide with HK
	// transmission — deliberately omitted.
	{"90915", "Toyota"},
	{"9091", "Toyota"},
	{"9091514", "Toyota"},
	{"90080", "Toyota"},
	{"90118", "Toyota"},
	{"88310", "Toyota"},
	{"87139", "Toyota"}, // cabin filter
	{"04152", "Toyota"}, // oil filter kit
	{"04466", "Toyota"}, // brake pad shim

	// ─── BMW / Mini ───────────────────────────────────────────────────
	// BMW OEMs are 11-42-* / 07-11-* / 13-71-* — full dash form matches
	// none of our HK regexes; the deny-list is the ONLY defence.
	{"11427", "BMW"},
	{"11421", "BMW"},
	{"11427634292", "BMW"}, // specific oil filter housing
	{"07119", "BMW"},
	{"07129", "BMW"},
	{"13717", "BMW"}, // air filter housing
	{"13718", "BMW"},
	{"34116", "BMW"}, // front brake pad
	{"34216", "BMW"}, // rear brake pad
	{"64119", "BMW"}, // cabin filter

	// ─── Nissan / Infiniti ────────────────────────────────────────────
	{"15208", "Nissan"},
	{"22448", "Nissan"},
	{"16546", "Nissan"}, // air filter
	{"27891", "Nissan"}, // cabin filter
	{"41060", "Nissan"}, // front brake pad
	{"D4060", "Nissan"}, // rear brake pad
	{"D1060", "Nissan"}, // brake pad variant

	// ─── Honda / Acura ────────────────────────────────────────────────
	{"15400", "Honda"},
	{"17220", "Honda"},
	{"80292", "Honda"}, // cabin filter
	{"45022", "Honda"}, // front brake pad
	{"43022", "Honda"}, // rear brake pad
	{"31500", "Honda"}, // battery

	// ─── Ford / Lincoln / Mercury ─────────────────────────────────────
	// Ford OEMs often start with alpha (AL3Z, BR3Z, EL3Z) — the format
	// regex rejects those. Numeric Ford prefixes covered here.
	{"1S7G", "Ford"},
	{"3S71", "Ford"},
	{"4F27", "Ford"},
	{"AL3Z", "Ford"},
	{"BR3Z", "Ford"},
	{"EL3Z", "Ford"},
	{"F5EX", "Ford"},

	// ─── GM / Chevrolet / Cadillac / GMC ──────────────────────────────
	{"12345678", "Chevrolet"}, // ACDelco pattern
	{"19", "Chevrolet"},
	{"88970", "Chevrolet"},
	{"22886", "Chevrolet"},
	{"25190", "Chevrolet"},

	// ─── Peugeot / Citroen (Stellantis PSA) ───────────────────────────
	{"9803", "Peugeot"},
	{"9804", "Peugeot"},
	{"1109", "Peugeot"}, // oil filter
	{"1613", "Peugeot"},

	// ─── Renault / Dacia / Nissan (RNA) ───────────────────────────────
	{"7700", "Renault"},
	{"8200", "Renault"},
	{"7701", "Renault"},

	// ─── Fiat / Alfa / Chrysler / Jeep (Stellantis) ──────────────────
	{"6803", "Chrysler"},
	{"6805", "Chrysler"},
	{"6810", "Chrysler"},
	{"6820", "Chrysler"},
	{"6830", "Chrysler"},
	{"5104", "Chrysler"},

	// ─── Mitsubishi ───────────────────────────────────────────────────
	{"MD", "Mitsubishi"},
	{"MR", "Mitsubishi"},
	{"MB", "Mitsubishi"},
	{"MN", "Mitsubishi"},

	// ─── Mazda ────────────────────────────────────────────────────────
	// Mazda uses alpha prefixes (LF01-*, KL01-*, RF7A-*) that the format
	// regex rejects; but LFY1 / KLY4 dashless forms may pass — deny.
	{"LF", "Mazda"},
	{"KL", "Mazda"},
	{"RF", "Mazda"},
	{"NF", "Mazda"},

	// ─── Volkswagen / Audi / Skoda / Seat (VAG) ──────────────────────
	{"06A", "Volkswagen"},
	{"06B", "Volkswagen"},
	{"06D", "Volkswagen"},
	{"06E", "Volkswagen"},
	{"06J", "Volkswagen"},
	{"03C", "Volkswagen"},
	{"03D", "Volkswagen"},
	{"03L", "Volkswagen"},

	// ─── Volvo ────────────────────────────────────────────────────────
	{"31261", "Volvo"},
	{"31267", "Volvo"},
	{"30637", "Volvo"},
	{"30748", "Volvo"},

	// ─── Land Rover / Jaguar ──────────────────────────────────────────
	{"LR0", "Land Rover"},
	{"LR1", "Land Rover"},
	{"C2Z", "Jaguar"},
	{"C2P", "Jaguar"},

	// ─── Mercedes-Benz ────────────────────────────────────────────────
	{"A000", "Mercedes-Benz"},
	{"A001", "Mercedes-Benz"},
	{"A002", "Mercedes-Benz"},
	{"A166", "Mercedes-Benz"},
	{"A278", "Mercedes-Benz"},
	{"0001", "Mercedes-Benz"},

	// ─── Subaru ───────────────────────────────────────────────────────
	{"15208AA", "Subaru"},
	{"16546AA", "Subaru"},
	{"26696", "Subaru"},

	// Note: Ford/Mazda alpha-first prefixes are ALSO covered by the
	// format regex rejection (hkOEMFormatDashed requires digits first).
	// This deny-list catches the dashless / partial forms that slip
	// through the regex.
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

	// M1.S2.T1: check the deny-list FIRST, before the format regex.
	// Prevents non-HK prefixes that HAPPEN to match an HK format from
	// getting classified as HK (e.g. Peugeot 9803* → "98" is an HK
	// Maintenance prefix, so pre-fix code took the HK branch; the
	// deny-list correctly identifies Peugeot but the branch never
	// reached it). Deny-list entries are canonical non-HK — always win.
	normalizedUpper := strings.ToUpper(NormalizeOEM(compact))
	for _, hint := range nonHKMakeHints {
		if strings.HasPrefix(normalizedUpper, strings.ToUpper(hint.prefixNormalized)) {
			// Set format for observability — some deny-listed OEMs match
			// the HK 5-5 form; recording that helps future debugging.
			format := "unknown"
			switch {
			case hkOEMFormatDashed.MatchString(compact):
				format = "hk_5_5_dashed"
			case hkOEMFormatFlat.MatchString(compact):
				format = "hk_5_5"
			}
			return HKScopeResult{
				IsHK:          false,
				Format:        format,
				SuggestedMake: hint.make,
				Reason:        "This app searches Hyundai/Kia parts only. This OEM looks like a " + hint.make + " part.",
			}
		}
	}

	format := "unknown"
	switch {
	case hkOEMFormatDashed.MatchString(compact):
		format = "hk_5_5_dashed"
	case hkOEMFormatFlat.MatchString(compact):
		format = "hk_5_5"
	}

	// Not the HK 5-5 pattern and not in the deny-list — return unknown.
	if format == "unknown" {
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

// HKOEMPrefix returns the 2-digit prefix of a normalised HK OEM number,
// or "" when the input does not match HK format. Provided for test compat.
func HKOEMPrefix(rawOEM string) string {
	compact := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(rawOEM, "-", ""), " ", ""))
	if len(compact) < 2 {
		return ""
	}
	return compact[:2]
}

// hkOEMPrefixes is the set of all known HK 2-digit prefixes.
// Built once from the prefix catalog; used by bulk tests.
var hkOEMPrefixes = func() map[string]bool {
	m := make(map[string]bool)
	for k := range prefixMap {
		if len(k) >= 2 {
			m[k[:2]] = true
		}
	}
	return m
}()

// hasVehicleContext reports whether any vehicle context (linkageTargetId, CC, or fuel type)
// is present. Used by confidence scoring to distinguish vehicle-scoped from universal searches.
func hasVehicleContext(linkageTargetId, vehicleCC int, fuelType string) bool {
	return linkageTargetId > 0 || vehicleCC > 0 || strings.TrimSpace(fuelType) != ""
}

// IsNonHKOEM is the logical complement of IsHKOEM: returns true when the
// given OEM number is explicitly not a Hyundai/Kia part (has a non-HK prefix).
func IsNonHKOEM(rawOEM string) bool {
	result := IsHKOEM(rawOEM)
	return !result.IsHK && result.SuggestedMake != ""
}

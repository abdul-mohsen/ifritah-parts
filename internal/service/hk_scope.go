package service

// hk_scope.go
//
// IsHKOEM reports whether oemNumber is a genuine Hyundai/KIA OEM part number.
//
// HK OEM numbers always:
//   - Start with a digit 1–9
//   - Have ≥5 total chars
//   - Use a 2-digit prefix from the HK OEM range
//
// Non-HK OEM numbers are rejected, including:
//   Toyota  90915-*     (prefix "90")
//   BMW     114xx-*     (prefix "11")
//   Honda   15400-*     (prefix "15")
//   Nissan  15208-*     (prefix "15")
//   Ford    0K011-*     (old Kia prefixed "0K" — ACCEPTED as HK variant)
//   Bosch   0 986-*     (letter-first-ish, rejected by digit rule)
//   MANN    W 811/80    (letter-first, rejected by digit rule)
//
// The prefix set is derived from the Hyundai/KIA EPC catalog structure
// and augmented with the "18" range (spark plugs, bulbs) which is not in
// the prefixMap category table but is a valid HK OEM prefix.

import "strings"

// hkOEMPrefixes is the set of valid 2-digit HK OEM number prefixes.
// Derived from: scripts/seed_db/main.go catalog + oem_prefix.go prefixMap.
var hkOEMPrefixes = map[string]bool{
	// Engine
	"18": true, "19": true,
	"21": true, "22": true, "23": true, "24": true, "25": true,
	"26": true, "27": true, "28": true, "29": true,
	// Drivetrain / transmission
	"30": true, "31": true, "32": true, "33": true, "34": true,
	"35": true, "36": true, "37": true, "38": true, "39": true,
	"41": true, "43": true, "44": true, "45": true, "46": true,
	"47": true, "48": true, "49": true,
	// Suspension / steering
	"51": true, "52": true, "53": true, "54": true, "55": true,
	"56": true, "57": true, "58": true, "59": true,
	// Frame / body structure
	"60": true, "61": true, "62": true, "63": true, "64": true,
	"65": true, "66": true, "67": true, "68": true, "69": true,
	"70": true, "71": true, "72": true, "73": true, "74": true,
	"75": true, "76": true,
	// Interior / electrical
	"81": true, "82": true, "83": true, "84": true, "85": true,
	"86": true, "87": true, "88": true, "89": true,
	"91": true, "92": true, "93": true, "94": true, "95": true,
	"96": true, "97": true, "98": true,
}

// IsHKOEM reports whether s looks like a genuine Hyundai/KIA OEM part number.
func IsHKOEM(s string) bool {
	if len(s) < 5 {
		return false
	}
	// Must start with a digit 1–9 (rules out MANN "W 811/80", Toyota "0Kxxx", etc.)
	if s[0] < '1' || s[0] > '9' {
		return false
	}
	// Extract the first 2 digits from the LEADING digit run.
	// Stop at the first non-digit non-separator character.
	// This rejects article numbers like "6PK1256" where 'P' breaks the run.
	digits := make([]byte, 0, 2)
	for i := 0; i < len(s) && len(digits) < 2; i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
		} else if c == '-' || c == ' ' {
			// separator — skip
		} else {
			// letter or punctuation — stop
			break
		}
	}
	if len(digits) < 2 {
		return false
	}
	prefix := string(digits)

	// Check against HK prefix set.
	if !hkOEMPrefixes[prefix] {
		return false
	}

	// Must have at least 4 digit characters total (rejects short codes).
	digitCount := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digitCount++
		}
	}
	return digitCount >= 4
}

// IsNonHKOEM is the logical inverse of IsHKOEM — reports if s is definitively
// a non-HK OEM number (starts with digit, has enough digits, but prefix is
// outside the HK range).  Returns false for strings that are neither
// (e.g. aftermarket article numbers like "W 811/80").
func IsNonHKOEM(s string) bool {
	if len(s) < 5 {
		return false
	}
	if s[0] < '0' || s[0] > '9' {
		return false
	}
	return !IsHKOEM(s)
}

// HKOEMPrefix returns the 2-digit numeric prefix of an OEM number,
// or "" if the string does not start with a digit or is too short.
func HKOEMPrefix(s string) string {
	if len(s) < 2 || s[0] < '0' || s[0] > '9' {
		return ""
	}
	digits := make([]byte, 0, 2)
	for i := 0; i < len(s) && len(digits) < 2; i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
		} else if c == '-' || c == ' ' {
			// skip separator
		} else {
			break
		}
	}
	if len(digits) < 2 {
		return ""
	}
	return string(digits)
}

// hkPrefixList returns a sorted slice of all valid HK OEM 2-digit prefixes.
// Used by tests.
func hkPrefixList() []string {
	result := make([]string, 0, len(hkOEMPrefixes))
	for k := range hkOEMPrefixes {
		result = append(result, k)
	}
	// Simple sort
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i] > result[j] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// IsJunkDescription reports whether desc looks like a junk or misrouted
// description from a tecdoc_keyword fallback.
//
// These descriptions appear when a valid OEM number is NOT in TecDoc's OEM
// cross-reference table and the search falls back to keyword matching on the
// raw number string.  The keyword fragments match unrelated parts (e.g.,
// "58101" matches NRF's "58101" radiator part number).
//
// Confirmed junk descriptions are sourced from live API captures 2026-08-15.
func IsJunkDescription(desc string) bool {
	lower := strings.ToLower(strings.TrimSpace(desc))
	for _, junk := range junkDescriptionDenyList {
		if strings.Contains(lower, strings.ToLower(junk)) {
			return true
		}
	}
	return false
}

// junkDescriptionDenyList is the deny list of confirmed junk descriptions.
// These appear as false positives when tecdoc_keyword fires on OEM numbers.
// Source: live API capture qa.ifritah.com 2026-08-15.
var junkDescriptionDenyList = []string{
	// Confirmed from text "oil filter" / "cabin air filter" queries
	"life-time-filter",
	"without cabin filter",
	"air filter life time",
	// Confirmed from thermostat 25500-2B100 keyword fallback
	"gear lever gaiter",
	"contact breaker",
	"gasket set, cylinder head",
	// Confirmed from CV joint 49500/49501/49590 keyword fallback
	"full gasket set, engine",
	// Confirmed from muffler 28830-2U000 catalog error
	"hose assy - vacuum",
	// Confirmed from turbo 29100-2B800 keyword fallback
	// (NB: "strut mounting" is legitimate for suspension queries — only junk for engine queries)
	// We intentionally keep this list to exact confirmed-bad strings
}

// JunkDescriptionDenyListLen returns the size of the deny list (for tests).
func JunkDescriptionDenyListLen() int { return len(junkDescriptionDenyList) }

// hasVehicleContext reports whether any vehicle context (linkage, CC, fuel)
// is present in the search request. Used to gate fitment-evidence classification.
func hasVehicleContext(linkageTargetId, vehicleCC int, fuelType string) bool {
	return linkageTargetId > 0 || vehicleCC > 0 || fuelType != ""
}

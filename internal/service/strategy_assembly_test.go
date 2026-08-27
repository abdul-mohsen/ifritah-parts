package service

// M0.T3 (2026-08) — VinAssemblyStrategy VIN-shape auto-detection.
//
// The 2026-08-24 per-strategy audit found `vin_assembly` returning zero
// hits for VIN queries (`?q=KMHDU4AD6DU100000&mode=vin_assembly`).
// Root cause: the strategy required req.LinkageTargetId > 0 and had no
// way to derive it from a VIN string. Callers had to pre-compute the
// linkageTargetId via /api/catalog/vehicles (which itself was broken —
// see M0.T4 sub-A + PR #29).
//
// The fix: when req.LinkageTargetId is unset AND req.Query looks like a
// VIN AND both VINDecoder + TecDoc are wired, decode the VIN and adopt
// the top linkageTargetId returned by TecDoc.LinkageTargetsForNHTSA.
//
// Because full end-to-end tests require a live MySQL TecDoc corpus, this
// unit test isolates resolveLinkageFromVIN — the pure helper responsible
// for shape detection + argument validation. It exhaustively covers the
// short-circuit paths that MUST NOT even attempt a database call.

import (
	"testing"
)

// TestResolveLinkageFromVIN_ShortCircuitPaths covers every path that
// MUST return 0 without invoking the VIN decoder or TecDoc lookup.
// Any of these paths hitting the DB would be a real regression.
func TestResolveLinkageFromVIN_ShortCircuitPaths(t *testing.T) {
	// A strategy with no attached SmartSearch — every helper call must
	// short-circuit long before it dereferences any nil pointer.
	strat := &VinAssemblyStrategy{search: nil}

	tests := []struct {
		name  string
		query string
	}{
		{"empty query", ""},
		{"whitespace only", "   "},
		{"too short (16 chars)", "KMHDU4AD6DU10000"},
		{"too long (18 chars)", "KMHDU4AD6DU1000000"},
		{"contains letter I", "KMIHDU4AD6DU10000"},
		{"contains letter O", "KMOHDU4AD6DU10000"},
		{"contains letter Q", "KMQHDU4AD6DU10000"},
		{"contains dash", "KMHDU4AD6-DU100000"},
		{"contains space in middle", "KMHDU4AD 6DU100000"},
		{"lowercase only — but wrong length", "abcdef"},
		{"OEM number shape", "26350-2J001"},
		{"free-text query", "oil filter"},
		{"correct length but wrong charset", "KMHDU4AD6DU10000$"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := strat.resolveLinkageFromVIN(tc.query); got != 0 {
				t.Errorf("resolveLinkageFromVIN(%q): got %d, want 0 (should short-circuit)", tc.query, got)
			}
		})
	}
}

// TestResolveLinkageFromVIN_NilSearchGuard confirms that a nil
// vinDecoder or nil tecdoc on the SmartSearch does NOT panic and does
// NOT return a linkage id even when the shape passes vinRegex.
func TestResolveLinkageFromVIN_NilSearchGuard(t *testing.T) {
	// SmartSearch present but with nil dependencies. Any real VIN goes
	// through vinRegex successfully, so the guard below is the only
	// thing preventing a nil dereference.
	stratMissingDeps := &VinAssemblyStrategy{search: &SmartSearch{}}
	validVIN := "KMHDU4AD6DU100000" // Hyundai Elantra MD 2013 (from vin_decoder.go WMI table)
	if got := stratMissingDeps.resolveLinkageFromVIN(validVIN); got != 0 {
		t.Errorf("resolveLinkageFromVIN with nil vinDecoder+nil tecdoc: got %d, want 0", got)
	}
}

// TestResolveLinkageFromVIN_ShapeAcceptance confirms the shape gate
// itself is correct: a valid 17-char alphanumeric ex-I/O/Q string
// passes the regex. This is the necessary condition for the strategy
// to proceed to the (mocked-out here) decoder + TecDoc lookup.
func TestResolveLinkageFromVIN_ShapeAcceptance(t *testing.T) {
	validShapes := []string{
		"KMHDU4AD6DU100000", // Hyundai (KMH make map)
		"5NPE24AF6FH000000", // Hyundai USA (5NP)
		"KNAFU4A28D5000000", // Kia (KNA)
		"KNDJT2A56D7000000", // Kia (KND)
		"5XYPG4A3XDG000000", // Kia USA (5XY)
	}
	for _, v := range validShapes {
		if !vinRegex.MatchString(v) {
			t.Errorf("vinRegex should accept %q as a VIN shape", v)
		}
	}
	// And the negation: shapes we do NOT want to accept.
	invalidShapes := []string{
		"KMHDU4AD6DU10000I", // ends in I
		"KMHDU4AD6DU10000O", // ends in O
		"KMHDU4AD6DU10000Q", // ends in Q
		"kmhdu4ad6du100000", // lowercase — regex is case-sensitive by design; the
		// resolveLinkageFromVIN caller ToUpper's first, so this is fine at that
		// entry point but the underlying regex correctly rejects raw lowercase.
	}
	for _, v := range invalidShapes {
		if vinRegex.MatchString(v) {
			t.Errorf("vinRegex should reject %q", v)
		}
	}
}

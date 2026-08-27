package service

import (
	"regexp"
	"strings"
)

// parsedVehicleDescription is the structured breakdown of a TecDoc
// linkagetargets.description string.
//
// The TecDoc linkagetargets table stores make/model/year/fuel/CC as separate
// columns; the description string carries the two facets that are NOT split
// out at the schema level: the platform/chassis code (e.g. "(TL)") and the
// engine-variant tag (e.g. "2.0 CRDi 4WD 136HP"). This parser therefore only
// pulls those two fields — make/model/year are joined from
// manufacturers/modelseries/linkagetargets columns directly in the SQL.
type parsedVehicleDescription struct {
	Chassis    string // e.g. "TL" from "HYUNDAI TUCSON (TL) 2.0 CRDi 4WD 136HP [08.2015-]"
	EngineSpec string // e.g. "2.0 CRDi 4WD 136HP" from the same input
}

// vehicleChassisRe matches a parenthesised 2-4 char token like "(TL)",
// "(PDE)", or "(DN8)". Callers must additionally verify the captured token
// contains at least one letter — pure-digit groups like "(191)" show up as
// horsepower ratings in some Hyundai/Kia rows and must not be treated as a
// chassis code.
var vehicleChassisRe = regexp.MustCompile(`\(([A-Za-z0-9]{2,4})\)`)

// vehicleEngineSpecRe finds the engine-specification tail. Captures:
//
//	"2.0 CRDi 4WD 136HP"   (displacement + engine code + variant)
//	"1.6 T-GDI 140HP"
//	"1.0 T-GDI 120HP MHEV"
//	"1.6"                   (bare displacement only)
//
// The match starts on a decimal displacement (e.g. "1.6", "2.0") preceded by
// a space or start-of-string — real TecDoc rows always write displacements
// with a decimal, so anchoring on `\d+\.\d+` prevents bare horsepower tokens
// like "120HP" from being misclassified as an engine spec on rows that only
// carry a fuel type (e.g. "IONIQ (AE) Electric 120HP"). After the anchor the
// pattern greedily consumes space-separated alphanumeric-or-hyphen tokens;
// non-ASCII separators (→, arrows, commas) and opening brackets naturally
// terminate the greedy repetition.
var vehicleEngineSpecRe = regexp.MustCompile(`(?:\s|^)(\d+\.\d+(?:\s+[A-Za-z0-9\-]+)*)`)

// hasLetter reports whether s contains at least one ASCII letter. Used to
// disambiguate parenthesised chassis codes from horsepower/CC groups that
// happen to fit the 2-4-char size window.
func hasLetter(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return true
		}
	}
	return false
}

// parseVehicleDescription extracts Chassis + EngineSpec from a TecDoc
// linkagetargets.description. Returns a zero-valued struct when nothing
// parses; never panics and never returns an error.
//
// Sample descriptions we support:
//
//	"HYUNDAI TUCSON (TL) 2.0 CRDi 4WD 136HP [08.2015-]"
//	    -> Chassis="TL"  EngineSpec="2.0 CRDi 4WD 136HP"
//
//	"KIA SORENTO (XM) 2.4 GDi AWD 189HP [05.2012-06.2020]"
//	    -> Chassis="XM"  EngineSpec="2.4 GDi AWD 189HP"
//
//	"HYUNDAI ELANTRA 1.6 [01.2011-]"
//	    -> Chassis=""    EngineSpec="1.6"
//
// The original description remains available to callers on the outer
// CompatibleVehicle.VehicleName field — this parser is additive only.
func parseVehicleDescription(desc string) parsedVehicleDescription {
	var out parsedVehicleDescription
	if desc == "" {
		return out
	}

	// Find the first parenthesised token that plausibly names a chassis:
	// 2-4 chars and containing at least one letter (so "(191)" — a power
	// rating some rows carry — is skipped).
	chassisEnd := -1 // byte index just past the closing ')' of the accepted chassis token
	for _, loc := range vehicleChassisRe.FindAllStringSubmatchIndex(desc, -1) {
		// loc = [matchStart, matchEnd, groupStart, groupEnd]
		if len(loc) < 4 {
			continue
		}
		token := desc[loc[2]:loc[3]]
		if !hasLetter(token) {
			continue
		}
		out.Chassis = strings.ToUpper(token)
		chassisEnd = loc[1]
		break
	}

	// Confine the engine-spec search to the tail after the accepted chassis
	// paren. If none was accepted, keep the whole description — the engine
	// regex is anchored on a numeric displacement so it still finds the spec.
	tail := desc
	if chassisEnd >= 0 && chassisEnd <= len(desc) {
		tail = desc[chassisEnd:]
	}

	// Strip the trailing bracketed year range so it doesn't get pulled into
	// the engine-spec match. TecDoc uses "[MM.YYYY-]" or "[MM.YYYY-MM.YYYY]".
	if idx := strings.Index(tail, "["); idx >= 0 {
		tail = tail[:idx]
	}
	// Some rows in other data sources use "→" or "->" as a year separator;
	// clip on those too so they don't leak into the engine spec.
	if idx := strings.Index(tail, "→"); idx >= 0 {
		tail = tail[:idx]
	}
	if idx := strings.Index(tail, "->"); idx >= 0 {
		tail = tail[:idx]
	}

	tail = strings.TrimSpace(tail)
	// Prefixing " " ensures the anchor `(?:\s|^)` fires when the tail
	// starts with a digit.
	if m := vehicleEngineSpecRe.FindStringSubmatch(" " + tail); len(m) > 1 {
		out.EngineSpec = strings.TrimSpace(m[1])
	}
	return out
}

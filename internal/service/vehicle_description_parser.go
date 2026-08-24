package service

import (
	"regexp"
	"strings"
)

// parsedVehicleDescription is the structured breakdown of a TecDoc
// linkagetargets.description string.
type parsedVehicleDescription struct {
	Chassis    string // e.g. "TL" from "(TL)"
	EngineSpec string // e.g. "2.0 CRDi 4WD 136HP"
}

// vehicleChassisRe matches the parenthesised chassis code common in
// TecDoc descriptions: "(TL)", "(GDe)", "(NC)". 2-4 chars.
var vehicleChassisRe = regexp.MustCompile(`\(([A-Za-z0-9]{2,4})\)`)

// vehicleEngineSpecRe finds the engine-specification tail. Captures:
//
//	"2.0 CRDi 4WD 136HP"   (displacement + engine code + variant)
//	"1.6 T-GDI"
//	"1.6"                   (bare displacement only)
//
// Trailing engine-code words are optional so short forms still match.
// Falls back to empty when no numeric displacement is found at all.
var vehicleEngineSpecRe = regexp.MustCompile(`(?:\s|^)(\d+(?:\.\d+)?(?:\s+[A-Za-z0-9\-]+)*)`)

// parseVehicleDescription extracts Chassis + EngineSpec from a TecDoc
// linkagetargets.description. Returns zero-valued struct when nothing
// parses; never returns error.
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
func parseVehicleDescription(desc string) parsedVehicleDescription {
	var out parsedVehicleDescription
	if desc == "" {
		return out
	}

	if m := vehicleChassisRe.FindStringSubmatch(desc); len(m) > 1 {
		out.Chassis = strings.ToUpper(m[1])
	}

	// Strip everything before the closing `)` of the chassis marker so
	// the engine-spec regex only looks at the tail.
	tail := desc
	if idx := strings.Index(tail, ")"); idx >= 0 {
		tail = tail[idx+1:]
	}
	// Strip the trailing bracketed year range so it doesn't get pulled
	// into the engine-spec match.
	if idx := strings.Index(tail, "["); idx >= 0 {
		tail = tail[:idx]
	}

	tail = strings.TrimSpace(tail)
	if m := vehicleEngineSpecRe.FindStringSubmatch(" " + tail); len(m) > 1 {
		out.EngineSpec = strings.TrimSpace(m[1])
	}
	return out
}

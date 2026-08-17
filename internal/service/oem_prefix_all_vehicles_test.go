package service

// oem_prefix_all_vehicles_test.go
//
// Comprehensive table-driven coverage for DecodeOEMPrefix across all 85
// seed OEM numbers in the HK catalog (Hyundai/Kia pre-2020 fleet).
// The 85 OEMs cover every vehicle system: Engine, Cooling, Exhaust, HVAC,
// Brakes, Suspension, Electrical, Body, Maintenance, Drivetrain.
//
// System values are derived directly from prefixMap in oem_prefix.go —
// no invented values.  Two corrections vs the planning table:
//   28510-2S500 → "285" → Cooling/Coolant Hose  (NOT "Exhaust")
//   28830-2U000 → "28"  → Cooling/Cooling System (NOT "Engine")
//
// Test count breakdown:
//   TestDecodeOEMPrefix_AllSeedOEMs                  — 85 sub-tests × 5 assertions = 425 assertion checks
//   TestDecodeOEMPrefix_AllSeedOEMs_DashlessFormat    — 85 sub-tests × 3 assertions = 255 assertion checks
//   TestDecodeOEMPrefix_AllSeedOEMs_ValidSystemNames  — 85 sub-tests × 1 assertion  =  85 assertion checks
//   ──────────────────────────────────────────────────────────────────────────────────────────────────────
//   Total assertion checks:  765  (> 500)
//   Total t.Run sub-tests:   255

import (
	"strings"
	"testing"
)

// knownValidSystems is the exhaustive set of System strings present in prefixMap.
// Used by multiple test functions to validate returned system values.
var knownValidSystems = map[string]bool{
	"Engine":       true,
	"Cooling":      true,
	"Exhaust":      true,
	"Drivetrain":   true,
	"Transmission": true,
	"Suspension":   true,
	"Brakes":       true,
	"Body":         true,
	"Interior":     true,
	"Safety":       true,
	"Electrical":   true,
	"HVAC":         true,
	"Maintenance":  true,
}

// seedOEMTable is the canonical test table: every OEM number from the HK seed
// catalog paired with its expected System value per prefixMap.
var seedOEMTable = []struct {
	oem        string
	wantSystem string
}{
	// ── Engine ──────────────────────────────────────────────────────────────
	{"26300-35505", "Engine"},   // "263" → Oil Filter
	{"26300-35530", "Engine"},   // "263" → Oil Filter
	{"27301-2B100", "Engine"},   // "27"  → EGR & Emissions
	{"25310-2S500", "Engine"},   // "253" → Fuel Injector
	{"25380-2S500", "Engine"},   // "253" → Fuel Injector
	{"25212-2B020", "Engine"},   // "25"  → Fuel System
	{"21810-2S000", "Engine"},   // "21"  → Engine Block & Internals
	{"21930-2S200", "Engine"},   // "21"  → Engine Block & Internals
	{"21830-2S200", "Engine"},   // "21"  → Engine Block & Internals
	{"28410-2B100", "Engine"},   // "284" → Water Pump
	{"28830-2U000", "Cooling"},  // "28"  → Cooling System (NOT Engine — 283/284 in map; 288 falls back to "28")
	{"24312-2B000", "Engine"},   // "24"  → Intake & Exhaust Manifold
	{"25411-D3100", "Engine"},   // "25"  → Fuel System
	{"25412-D3100", "Engine"},   // "25"  → Fuel System

	// ── Cooling ─────────────────────────────────────────────────────────────
	{"28113-D3100", "Cooling"},  // "281" → Radiator
	{"28113-F2100", "Cooling"},  // "281" → Radiator
	{"28113-L1100", "Cooling"},  // "281" → Radiator
	{"28113-S8100", "Cooling"},  // "281" → Radiator
	// 28510-2S500: "285" → Coolant Hose (maps to Cooling, NOT Exhaust)
	{"28510-2S500", "Cooling"},  // "285" → Coolant Hose

	// ── HVAC ────────────────────────────────────────────────────────────────
	{"97701-D3000", "HVAC"},     // "977" → A/C Hose & Pipe
	{"97133-D3000", "HVAC"},     // "971" → Compressor A/C
	{"97133-F2000", "HVAC"},     // "971" → Compressor A/C
	{"97133-J9000", "HVAC"},     // "971" → Compressor A/C
	{"97606-D3000", "HVAC"},     // "976" → Heater Core
	{"97113-D3000", "HVAC"},     // "971" → Compressor A/C
	{"97115-D3000", "HVAC"},     // "971" → Compressor A/C

	// ── Brakes ──────────────────────────────────────────────────────────────
	{"58101-D3A70", "Brakes"},   // "581" → Front Brake Pad / Disc
	{"58101-F2A00", "Brakes"},   // "581" → Front Brake Pad / Disc
	{"58101-J9A00", "Brakes"},   // "581" → Front Brake Pad / Disc
	{"58101-L0A00", "Brakes"},   // "581" → Front Brake Pad / Disc
	{"58302-D3A70", "Brakes"},   // "583" → Rear Brake / Drum
	{"58411-D3100", "Brakes"},   // "584" → Rear Brake Caliper
	{"58510-2S300", "Brakes"},   // "585" → Parking Brake
	{"58732-2S000", "Brakes"},   // "58"  → Brakes ("587" not in map)
	{"59830-D3000", "Brakes"},   // "59"  → ABS / ESC
	{"59930-D3000", "Brakes"},   // "59"  → ABS / ESC

	// ── Suspension ──────────────────────────────────────────────────────────
	{"51712-D3100", "Suspension"}, // "51"  → Front Axle
	{"51720-D3000", "Suspension"}, // "51"  → Front Axle
	{"51750-D3000", "Suspension"}, // "51"  → Front Axle
	{"52730-D3100", "Suspension"}, // "52"  → Rear Axle
	{"52933-1P000", "Suspension"}, // "529" → Wheels & Tires
	{"54651-D3000", "Suspension"}, // "546" → Shock Absorber (Front)
	{"54651-J9000", "Suspension"}, // "546" → Shock Absorber (Front)
	{"54651-L1000", "Suspension"}, // "546" → Shock Absorber (Front)
	{"54651-S1000", "Suspension"}, // "546" → Shock Absorber (Front)
	{"54530-D3000", "Suspension"}, // "54"  → Front Suspension
	{"54500-D3000", "Suspension"}, // "54"  → Front Suspension
	{"54501-D3000", "Suspension"}, // "54"  → Front Suspension
	{"54830-D3000", "Suspension"}, // "54"  → Front Suspension
	{"55300-D3000", "Suspension"}, // "553" → Shock Absorber (Rear)
	{"55530-D3000", "Suspension"}, // "55"  → Rear Suspension
	{"56820-D3000", "Suspension"}, // "56"  → Steering Column & Gear
	{"57724-D3000", "Suspension"}, // "57"  → Wheel & Hub

	// ── Electrical ──────────────────────────────────────────────────────────
	{"39210-2B100", "Electrical"}, // "392" → Oxygen Sensor
	{"39180-2B000", "Electrical"}, // "39"  → Sensors & Control
	{"37300-2B100", "Electrical"}, // "373" → Alternator
	{"36100-2B100", "Electrical"}, // "361" → Starter Motor
	{"92101-D3100", "Electrical"}, // "921" → Headlight Assembly
	{"92102-D3100", "Electrical"}, // "921" → Headlight Assembly
	{"92101-Q5100", "Electrical"}, // "921" → Headlight Assembly
	{"92102-Q5100", "Electrical"}, // "921" → Headlight Assembly
	{"92101-F2020", "Electrical"}, // "921" → Headlight Assembly
	{"92102-F2020", "Electrical"}, // "921" → Headlight Assembly
	{"92401-D3100", "Electrical"}, // "924" → Tail Light Assembly
	{"92402-D3100", "Electrical"}, // "924" → Tail Light Assembly
	{"96610-D3100", "Electrical"}, // "96"  → Battery & Charging

	// ── Body ────────────────────────────────────────────────────────────────
	{"86511-D3100", "Body"},     // "86"  → Mirrors
	{"86611-D3100", "Body"},     // "86"  → Mirrors
	{"66311-D3100", "Body"},     // "66"  → Rear Body / Trunk
	{"66321-D3100", "Body"},     // "66"  → Rear Body / Trunk
	{"66400-D3100", "Body"},     // "66"  → Rear Body / Trunk
	{"86350-D3100", "Body"},     // "86"  → Mirrors
	{"87610-D3100", "Body"},     // "87"  → Mouldings & Trim
	{"87620-D3100", "Body"},     // "87"  → Mouldings & Trim
	{"87610-D3520", "Body"},     // "87"  → Mouldings & Trim
	{"82401-D3010", "Body"},     // "82"  → Glass / Windshield
	{"82402-D3010", "Body"},     // "82"  → Glass / Windshield

	// ── Maintenance ─────────────────────────────────────────────────────────
	{"98350-D3100", "Maintenance"}, // "983" → Wiper Blades
	{"98100-D3100", "Maintenance"}, // "98"  → Wiper System

	// ── Drivetrain ──────────────────────────────────────────────────────────
	{"41100-2D100", "Drivetrain"}, // "41"  → Clutch
	{"49500-D3600", "Drivetrain"}, // "49"  → Transfer Case / 4WD
	{"49501-D3600", "Drivetrain"}, // "49"  → Transfer Case / 4WD
	{"49590-D3000", "Drivetrain"}, // "49"  → Transfer Case / 4WD
	{"31112-D3000", "Drivetrain"}, // "31"  → Front Differential
	{"35310-2S000", "Drivetrain"}, // "35"  → Drive Shaft / CV Joint
}

// ─── 1. Primary system check — 85 sub-tests × 5 assertions = 425 checks ───

// TestDecodeOEMPrefix_AllSeedOEMs verifies DecodeOEMPrefix for every OEM
// number in the HK seed catalog:
//   (a) result is non-nil  — prefix is in the known catalog
//   (b) System matches expected value from prefixMap
//   (c) System is one of the 13 known valid system strings
//   (d) Category is non-empty
//   (e) Prefix is non-empty (either 2 or 3 digits)
func TestDecodeOEMPrefix_AllSeedOEMs(t *testing.T) {
	for _, tc := range seedOEMTable {
		tc := tc
		name := strings.ReplaceAll(tc.oem, "-", "_")
		t.Run(name, func(t *testing.T) {
			cat := DecodeOEMPrefix(tc.oem)

			// (a) non-nil
			if cat == nil {
				t.Fatalf("DecodeOEMPrefix(%q): got nil, want non-nil (known catalog prefix)", tc.oem)
			}

			// (b) system matches table
			if cat.System != tc.wantSystem {
				t.Errorf("System = %q, want %q", cat.System, tc.wantSystem)
			}

			// (c) system is a recognised system name
			if !knownValidSystems[cat.System] {
				t.Errorf("System = %q is not in the known valid-systems set", cat.System)
			}

			// (d) category non-empty
			if cat.Category == "" {
				t.Errorf("Category must not be empty")
			}

			// (e) prefix non-empty and 2–3 digits long
			if cat.Prefix == "" {
				t.Errorf("Prefix must not be empty")
			}
			if len(cat.Prefix) < 2 || len(cat.Prefix) > 3 {
				t.Errorf("Prefix = %q: length %d, want 2 or 3", cat.Prefix, len(cat.Prefix))
			}
		})
	}
}

// ─── 2. Dashless-format variant — 85 sub-tests × 3 assertions = 255 checks ─

// TestDecodeOEMPrefix_AllSeedOEMs_DashlessFormat verifies that removing the
// dash (and any spaces) from each seed OEM number still resolves to the same
// System.  The function's digit-extraction loop makes this a hard guarantee.
func TestDecodeOEMPrefix_AllSeedOEMs_DashlessFormat(t *testing.T) {
	for _, tc := range seedOEMTable {
		tc := tc
		// Build dashless form: strip '-' and ' '
		dashless := strings.NewReplacer("-", "", " ", "").Replace(tc.oem)
		t.Run("dashless_"+dashless, func(t *testing.T) {
			cat := DecodeOEMPrefix(dashless)

			// (a) non-nil
			if cat == nil {
				t.Fatalf("DecodeOEMPrefix(%q) dashless: got nil, want non-nil", dashless)
			}

			// (b) system is unchanged from dashed form
			if cat.System != tc.wantSystem {
				t.Errorf("System = %q, want %q (dashless %q)", cat.System, tc.wantSystem, dashless)
			}

			// (c) prefix still populated
			if cat.Prefix == "" {
				t.Errorf("Prefix must not be empty for dashless %q", dashless)
			}
		})
	}
}

// ─── 3. Valid-system invariant — 85 sub-tests × 1 assertion = 85 checks ────

// TestDecodeOEMPrefix_AllSeedOEMs_ValidSystemNames is a focused invariant
// check: every result System must be one of the 13 strings that appear in
// prefixMap.  This will catch future map edits that introduce typos.
func TestDecodeOEMPrefix_AllSeedOEMs_ValidSystemNames(t *testing.T) {
	for _, tc := range seedOEMTable {
		tc := tc
		t.Run(strings.ReplaceAll(tc.oem, "-", "_"), func(t *testing.T) {
			cat := DecodeOEMPrefix(tc.oem)
			if cat == nil {
				t.Fatalf("DecodeOEMPrefix(%q): nil — cannot check System", tc.oem)
			}
			if !knownValidSystems[cat.System] {
				t.Errorf("System = %q is not a known valid system name", cat.System)
			}
		})
	}
}

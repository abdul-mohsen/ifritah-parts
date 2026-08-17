package service

import "testing"

// TestClassifyCategory_KnownCategoryRules verifies that well-known part descriptions
// resolve to the correct fitment driver. Each driver type is exercised.
func TestClassifyCategory_KnownCategoryRules(t *testing.T) {
	cases := []struct {
		description string
		wantDriver  FitmentDriver
	}{
		// FitEngine — engine-size-dependent
		{"Alternator", FitEngine},
		{"Starter Motor Assembly", FitEngine},
		{"Spark Plug", FitEngine},
		{"Ignition Coil Pack", FitEngine},
		{"Water Pump", FitEngine},
		{"Timing Belt Kit", FitEngine},
		{"Timing Chain", FitEngine},
		{"Turbocharger", FitEngine},
		{"Fuel Pump", FitEngine},
		{"Fuel Injector Set", FitEngine},
		{"Radiator", FitEngine},
		{"Serpentine Belt", FitEngine},
		{"Exhaust Manifold", FitEngine},
		{"Catalytic Converter", FitEngine},
		{"EGR Valve", FitEngine},
		{"Oxygen Sensor (pre-cat)", FitEngine},
		// FitBody — body-style-dependent
		{"Front Wiper Blade", FitBody},
		{"Rear Wiper Motor", FitBody},
		{"Side Mirror Assembly", FitBody},
		{"Headlight Assembly", FitBody},
		{"Tail Light Cluster", FitBody},
		{"Front Bumper", FitBody},
		{"Fog Light", FitBody},
		{"Hood Bonnet", FitBody},
		{"Trunk Lid", FitBody},
		// FitDrivetrain — drive-type-dependent
		{"CV Joint Boot Kit", FitDrivetrain},
		{"CV Axle Shaft", FitDrivetrain},
		{"Drive Shaft Assembly", FitDrivetrain},
		{"Clutch Disc Set", FitDrivetrain},
		{"Automatic Transmission", FitDrivetrain},
		// FitBrake — trim/sport-variant-dependent
		{"Front Brake Pad Set", FitBrake},
		{"Rear Brake Disc", FitBrake},
		{"Brake Rotor", FitBrake},
		{"Brake Caliper Rear", FitBrake},
		{"Brake Master Cylinder", FitBrake},
		// FitUniversal — fit by physical dimensions
		{"Oil Filter", FitUniversal},
		{"Cabin Filter", FitUniversal},
		{"Pollen Filter", FitUniversal},
		{"Air Filter Assembly", FitUniversal},
		{"Fuel Filter", FitUniversal},
		{"Wheel Bolt Set", FitUniversal},
		{"Bulb H4", FitUniversal},
		{"Fuse Set", FitUniversal},
	}

	for _, tc := range cases {
		rule := ClassifyCategory(tc.description)
		if rule.Driver != tc.wantDriver {
			t.Errorf("ClassifyCategory(%q): got driver %d, want %d",
				tc.description, rule.Driver, tc.wantDriver)
		}
	}
}

// TestClassifyCategory_FallsBackToFitUniversal checks that an unrecognised
// part description returns FitUniversal (safe default).
func TestClassifyCategory_FallsBackToFitUniversal(t *testing.T) {
	unknowns := []string{
		"Unknown Part Type XYZ 9999",
		"",
		// Note: "Door Sill Plate" matches the "Door" key (FitBody) — intentionally excluded.
		"Antenna Mast",
		"Cup Holder",
		"Sun Visor",
		"Cargo Mat",
	}
	for _, desc := range unknowns {
		rule := ClassifyCategory(desc)
		if rule.Driver != FitUniversal {
			t.Errorf("ClassifyCategory(%q): expected FitUniversal fallback, got %d", desc, rule.Driver)
		}
	}
}

// TestClassifyCategory_CaseInsensitive verifies the match is case-insensitive
// so database description variations (ALL CAPS, lower, mixed) all resolve.
func TestClassifyCategory_CaseInsensitive(t *testing.T) {
	cases := []struct {
		description string
		wantDriver  FitmentDriver
	}{
		{"ALTERNATOR ASSEMBLY", FitEngine},
		{"alternator assembly", FitEngine},
		{"AlTeRnAtOr", FitEngine},
		{"OIL FILTER", FitUniversal},
		{"oil filter", FitUniversal},
		{"BRAKE PAD SET", FitBrake},
		{"brake disc", FitBrake},
		{"WIPER BLADE", FitBody},
		{"wiper blade", FitBody},
	}
	for _, tc := range cases {
		rule := ClassifyCategory(tc.description)
		if rule.Driver != tc.wantDriver {
			t.Errorf("ClassifyCategory(%q): got driver %d, want %d",
				tc.description, rule.Driver, tc.wantDriver)
		}
	}
}

// TestClassifyCategory_LongestMatchWins verifies that when two rules could match,
// the longer (more specific) key wins.
func TestClassifyCategory_LongestMatchWins(t *testing.T) {
	// "Cabin Filter" (FitUniversal, 12 chars) should beat "Cabin" (not in map)
	// in all cases where both substrings appear in the description.
	rule := ClassifyCategory("Cabin Filter & Blower Motor")
	if rule.Driver != FitUniversal {
		t.Errorf("'Cabin Filter & Blower Motor': expected FitUniversal, got %d", rule.Driver)
	}

	// "Brake Pad" (FitBrake, 9 chars) description should resolve to FitBrake.
	rule2 := ClassifyCategory("Brake Pad Set (Front)")
	if rule2.Driver != FitBrake {
		t.Errorf("'Brake Pad Set (Front)': expected FitBrake, got %d", rule2.Driver)
	}

	// "Timing Belt" is more specific than "Belt" for the engine variant.
	rule3 := ClassifyCategory("Timing Belt Kit with Tensioner")
	if rule3.Driver != FitEngine {
		t.Errorf("'Timing Belt Kit': expected FitEngine, got %d", rule3.Driver)
	}
}

// TestClassifyCategory_CCMarginIsSet verifies that engine-sensitive categories
// carry a non-zero CCMargin field so confidence calculations can use it.
func TestClassifyCategory_CCMarginIsSet(t *testing.T) {
	strict := []string{"Spark Plug", "Timing Belt", "Cylinder Head", "Turbocharger"}
	for _, desc := range strict {
		rule := ClassifyCategory(desc)
		if rule.Driver != FitEngine {
			t.Errorf("ClassifyCategory(%q): expected FitEngine", desc)
		}
		if rule.CCMargin == 0 {
			t.Errorf("ClassifyCategory(%q): expected non-zero CCMargin for strict engine part", desc)
		}
	}
}

// TestContainsIgnoreCase covers the internal helper used by ClassifyCategory.
func TestContainsIgnoreCase(t *testing.T) {
	cases := []struct {
		s, sub string
		want   bool
	}{
		{"Alternator Assembly", "Alternator", true},
		{"alternator", "ALTERNATOR", true},
		{"BRAKE PAD SET", "brake pad", true},
		{"oil filter", "Oil Filter", true},
		{"Cabin Filter Assy", "Cabin Filter", true},
		// No match
		{"Unrelated description", "Alternator", false},
		{"", "Alternator", false},
		// Substring longer than string
		{"short", "too long substring that cannot match", false},
		// Empty substring matches everything (vacuously true)
		{"anything", "", true},
	}
	for _, tc := range cases {
		got := containsIgnoreCase(tc.s, tc.sub)
		if got != tc.want {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, want %v", tc.s, tc.sub, got, tc.want)
		}
	}
}

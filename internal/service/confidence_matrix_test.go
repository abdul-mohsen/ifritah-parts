//go:build quality_gates

// Package service — systematic confidence matrix test for computeConfidenceForVehicle.
//
// Sub-test totals
//  1. TestConfidenceMatrix_FitEngine_AllVehicleCC        16 × 25       =  400
//  2. TestConfidenceMatrix_FitEngine_AllMargins           6 × 4 × 10   =  240
//  3. TestConfidenceMatrix_FitEngine_NoCCContext          16 + 25       =   41
//  4. TestConfidenceMatrix_FitUniversal_AllVehicles      16 × 25       =  400
//  5. TestConfidenceMatrix_FitBody_AllVehicles            16 × 25       =  400
//  6. TestConfidenceMatrix_FitDrivetrain_AllVehicles      16 × 25       =  400
//  7. TestConfidenceMatrix_FitBrake_AllVehicles           16 × 25       =  400
//  8. TestConfidenceMatrix_RealVehiclePartCombinations    16            =   16
//                                                              TOTAL = 2,297
package service

import (
	"fmt"
	"math"
	"testing"
)

// ─── Seed data ────────────────────────────────────────────────────────────────

// cmxVehicleCCs lists the real engine displacements from seed_db (all pre-2020
// Hyundai/Kia vehicles). Exactly 16 entries.
var cmxVehicleCCs = []struct {
	name string
	cc   int
}{
	{"Tucson 2.0 MPI (TL)", 1999},
	{"Tucson 1.6 T-GDI (TL)", 1591},
	{"Tucson 2.0 CRDi (TL)", 1995},
	{"Tucson 2.5 GDI (NX4)", 2497},
	{"Sportage 2.0 MPI (QL)", 1999},
	{"Sportage 1.6 T-GDI (QL)", 1591},
	{"Sportage 2.0 CRDi (QL)", 1995},
	{"Elantra 2.0 MPI (AD)", 1999},
	{"Elantra 1.6 Turbo (AD)", 1591},
	{"Sonata 2.5 MPI (DN8)", 2497},
	{"Sonata 1.6 T-GDI (DN8)", 1598},
	{"Kona 2.0 MPI (OS)", 1999},
	{"Kona 1.6 T-GDI (OS)", 1591},
	{"K5 2.5 GDI (DL3)", 2497},
	{"Santa Fe 2.5 GDI (TM)", 2497},
	{"Santa Fe 2.2 CRDi (TM)", 2199},
}

// cmxPartCCs is the test-matrix part-displacement spread. Includes:
//   - cc-independent (0), engine-boundary values, TecDoc cross-ref values, outliers.
//
// Exactly 25 entries.
var cmxPartCCs = []int{
	0, 800, 1000, 1200, 1400, 1500,
	1591, 1595, 1598, 1599, 1600, 1700,
	1800, 1900, 1999, 2000, 2100, 2199,
	2200, 2300, 2400, 2497, 2500, 3000, 4000,
}

// cmxMargins holds all CCMargin values under test.
// 0 means the code defaults to 500 at runtime.
var cmxMargins = []int{0, 200, 300, 500, 800, 1000}

// ─── Pure helpers (no external deps) ─────────────────────────────────────────

// cmxAbsDiff returns |a - b|.
func cmxAbsDiff(a, b int) int {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

// cmxEffectiveMargin mirrors the runtime default: CCMargin=0 → 500.
func cmxEffectiveMargin(rawMargin int) int {
	if rawMargin == 0 {
		return 500
	}
	return rawMargin
}

// cmxExpectedFitEngineConf returns the confidence the function should produce
// for FitEngine given vehicleCC, partCC, and the raw (caller-supplied) CCMargin.
// Mirrors the logic in computeConfidenceForVehicle exactly.
func cmxExpectedFitEngineConf(vehicleCC, partCC, rawMargin int) float64 {
	if vehicleCC == 0 || partCC == 0 {
		return 0.7
	}
	eff := cmxEffectiveMargin(rawMargin)
	diff := cmxAbsDiff(vehicleCC, partCC)
	switch {
	case diff == 0:
		return 0.95
	case diff <= eff:
		return 0.85
	case diff <= eff*2:
		return 0.5
	default:
		return 0.2
	}
}

// cmxExpectedFitBrakeConf returns the expected FitBrake confidence.
// The runtime hard-codes the 1000 cc tolerance (CCMargin field is advisory there).
func cmxExpectedFitBrakeConf(vehicleCC, partCC int) float64 {
	if vehicleCC > 0 && partCC > 0 {
		if cmxAbsDiff(vehicleCC, partCC) <= 1000 {
			return 0.85
		}
		return 0.6
	}
	return 0.75
}

// ─── 1. FitEngine × all real vehicle CCs × all part CCs ──────────────────────

// TestConfidenceMatrix_FitEngine_AllVehicleCC runs the FitEngine scoring rule
// over the full 16 × 25 grid of (vehicleCC, partCC) with CCMargin=500.
// Each cell asserts:
//   - Confidence is in the valid range [0, 1].
//   - Confidence matches the expected value for the (vehicleCC, partCC) band:
//     partCC=0 or vehicleCC=0 → 0.7
//     diff=0                  → 0.95
//     0 < diff ≤ 500          → 0.85
//     500 < diff ≤ 1000       → 0.50
//     diff > 1000             → 0.20
//
// Total: 400 sub-tests.
func TestConfidenceMatrix_FitEngine_AllVehicleCC(t *testing.T) {
	s := &SmartSearch{}
	rule := CategoryRule{Driver: FitEngine, CCMargin: 500}

	for _, v := range cmxVehicleCCs {
		v := v
		for _, pcc := range cmxPartCCs {
			pcc := pcc
			name := fmt.Sprintf("%s/vCC=%d/pCC=%d", v.name, v.cc, pcc)
			t.Run(name, func(t *testing.T) {
				conf, _ := s.computeConfidenceForVehicle(rule, v.cc, pcc, "", "")

				// Range invariant — must always hold.
				if conf < 0 || conf > 1 {
					t.Fatalf("conf=%.4f is outside [0,1] for vehicleCC=%d partCC=%d",
						conf, v.cc, pcc)
				}

				want := cmxExpectedFitEngineConf(v.cc, pcc, 500)
				if math.Abs(conf-want) > 1e-9 {
					t.Errorf("FitEngine vehicleCC=%d partCC=%d: got %.4f, want %.4f",
						v.cc, pcc, conf, want)
				}
			})
		}
	}
}

// ─── 2. FitEngine × all margins × all confidence bands ───────────────────────

// TestConfidenceMatrix_FitEngine_AllMargins tests every combination of the 6
// CCMargin values against 4 confidence bands using 10 CC pairs per band.
//
//   Band     Expected   How diffs are chosen
//   exact    0.95       diff = 0 (10 identical vehicleCC pairs)
//   within   0.85       diffs evenly spaced in [step, eff]
//   marginal 0.50       diffs evenly spaced in [eff+step, eff×2]
//   mismatch 0.20       diffs = eff×2+50, eff×2+100, …, eff×2+500
//
// vehicleCC is fixed at 5000 so partCC is never ≤ 0 for any margin.
//
// Total: 6 × 4 × 10 = 240 sub-tests.
func TestConfidenceMatrix_FitEngine_AllMargins(t *testing.T) {
	s := &SmartSearch{}
	const vehicleCC = 5000

	for _, rawMargin := range cmxMargins {
		rawMargin := rawMargin
		eff := cmxEffectiveMargin(rawMargin)
		step := eff / 10
		if step < 1 {
			step = 1
		}

		type bandCase struct {
			label string
			pcc   int
			want  float64
		}
		var cases []bandCase

		// Band 1 — exact (diff = 0): 10 copies.
		for i := 0; i < 10; i++ {
			cases = append(cases, bandCase{
				label: fmt.Sprintf("exact/rep=%d", i),
				pcc:   vehicleCC,
				want:  0.95,
			})
		}

		// Band 2 — within (0 < diff ≤ eff): diffs at step*1 … step*10.
		for i := 1; i <= 10; i++ {
			diff := i * step
			if diff > eff {
				diff = eff
			}
			cases = append(cases, bandCase{
				label: fmt.Sprintf("within/i=%d/diff=%d", i, diff),
				pcc:   vehicleCC - diff,
				want:  0.85,
			})
		}

		// Band 3 — marginal (eff < diff ≤ eff×2): diffs at eff+step*1 … eff+step*10.
		for i := 1; i <= 10; i++ {
			diff := eff + i*step
			if diff > eff*2 {
				diff = eff * 2
			}
			cases = append(cases, bandCase{
				label: fmt.Sprintf("marginal/i=%d/diff=%d", i, diff),
				pcc:   vehicleCC - diff,
				want:  0.5,
			})
		}

		// Band 4 — mismatch (diff > eff×2): diffs at eff×2+50, +100, …, +500.
		for i := 1; i <= 10; i++ {
			diff := eff*2 + i*50
			cases = append(cases, bandCase{
				label: fmt.Sprintf("mismatch/i=%d/diff=%d", i, diff),
				pcc:   vehicleCC - diff,
				want:  0.2,
			})
		}

		for _, tc := range cases {
			tc := tc
			name := fmt.Sprintf("margin=%d/%s", rawMargin, tc.label)
			t.Run(name, func(t *testing.T) {
				rule := CategoryRule{Driver: FitEngine, CCMargin: rawMargin}
				conf, _ := s.computeConfidenceForVehicle(rule, vehicleCC, tc.pcc, "", "")
				if math.Abs(conf-tc.want) > 1e-9 {
					t.Errorf("margin=%d(eff=%d) pcc=%d: got %.4f, want %.4f",
						rawMargin, eff, tc.pcc, conf, tc.want)
				}
			})
		}
	}
}

// ─── 3. FitEngine — zero CC context ──────────────────────────────────────────

// TestConfidenceMatrix_FitEngine_NoCCContext verifies that any missing CC
// input always yields confidence 0.7 regardless of the other value.
//
//   Case A: partCC=0 × each real vehicle   → 16 sub-tests
//   Case B: vehicleCC=0 × each part CC     → 25 sub-tests
//
// Total: 41 sub-tests.
func TestConfidenceMatrix_FitEngine_NoCCContext(t *testing.T) {
	s := &SmartSearch{}
	rule := CategoryRule{Driver: FitEngine, CCMargin: 500}

	// Case A: part is CC-independent (e.g. oil filter 26300-35505); vehicleCC is known.
	for _, v := range cmxVehicleCCs {
		v := v
		name := fmt.Sprintf("partCC=0/%s/vCC=%d", v.name, v.cc)
		t.Run(name, func(t *testing.T) {
			conf, _ := s.computeConfidenceForVehicle(rule, v.cc, 0, "", "")
			if conf != 0.7 {
				t.Errorf("partCC=0 vehicleCC=%d: got %.4f, want 0.7", v.cc, conf)
			}
		})
	}

	// Case B: vehicle CC is unknown (VIN not provided).
	for _, pcc := range cmxPartCCs {
		pcc := pcc
		name := fmt.Sprintf("vehicleCC=0/partCC=%d", pcc)
		t.Run(name, func(t *testing.T) {
			conf, _ := s.computeConfidenceForVehicle(rule, 0, pcc, "", "")
			if conf != 0.7 {
				t.Errorf("vehicleCC=0 partCC=%d: got %.4f, want 0.7", pcc, conf)
			}
		})
	}
}

// ─── 4. FitUniversal × all vehicles × all part CCs ───────────────────────────

// TestConfidenceMatrix_FitUniversal_AllVehicles asserts that FitUniversal returns
// exactly 0.90 for every (vehicleCC, partCC) combination regardless of values.
//
// Total: 16 × 25 = 400 sub-tests.
func TestConfidenceMatrix_FitUniversal_AllVehicles(t *testing.T) {
	s := &SmartSearch{}
	rule := CategoryRule{Driver: FitUniversal}

	for _, v := range cmxVehicleCCs {
		v := v
		for _, pcc := range cmxPartCCs {
			pcc := pcc
			name := fmt.Sprintf("%s/vCC=%d/pCC=%d", v.name, v.cc, pcc)
			t.Run(name, func(t *testing.T) {
				conf, _ := s.computeConfidenceForVehicle(rule, v.cc, pcc, "", "")
				if conf != 0.90 {
					t.Errorf("FitUniversal vehicleCC=%d partCC=%d: got %.4f, want 0.90",
						v.cc, pcc, conf)
				}
			})
		}
	}
}

// ─── 5. FitBody × all vehicles × all part CCs ────────────────────────────────

// TestConfidenceMatrix_FitBody_AllVehicles asserts that FitBody returns exactly
// 0.85 for every (vehicleCC, partCC) combination — body fitment is determined by
// model/generation, not engine displacement.
//
// Total: 16 × 25 = 400 sub-tests.
func TestConfidenceMatrix_FitBody_AllVehicles(t *testing.T) {
	s := &SmartSearch{}
	rule := CategoryRule{Driver: FitBody}

	for _, v := range cmxVehicleCCs {
		v := v
		for _, pcc := range cmxPartCCs {
			pcc := pcc
			name := fmt.Sprintf("%s/vCC=%d/pCC=%d", v.name, v.cc, pcc)
			t.Run(name, func(t *testing.T) {
				conf, _ := s.computeConfidenceForVehicle(rule, v.cc, pcc, "", "")
				if conf != 0.85 {
					t.Errorf("FitBody vehicleCC=%d partCC=%d: got %.4f, want 0.85",
						v.cc, pcc, conf)
				}
			})
		}
	}
}

// ─── 6. FitDrivetrain × all vehicles × all part CCs ──────────────────────────

// TestConfidenceMatrix_FitDrivetrain_AllVehicles asserts that FitDrivetrain returns
// exactly 0.80 for every (vehicleCC, partCC) combination — drivetrain fitment
// depends on drive type (FWD/AWD/RWD), not engine displacement.
//
// Total: 16 × 25 = 400 sub-tests.
func TestConfidenceMatrix_FitDrivetrain_AllVehicles(t *testing.T) {
	s := &SmartSearch{}
	rule := CategoryRule{Driver: FitDrivetrain}

	for _, v := range cmxVehicleCCs {
		v := v
		for _, pcc := range cmxPartCCs {
			pcc := pcc
			name := fmt.Sprintf("%s/vCC=%d/pCC=%d", v.name, v.cc, pcc)
			t.Run(name, func(t *testing.T) {
				conf, _ := s.computeConfidenceForVehicle(rule, v.cc, pcc, "", "")
				if conf != 0.80 {
					t.Errorf("FitDrivetrain vehicleCC=%d partCC=%d: got %.4f, want 0.80",
						v.cc, pcc, conf)
				}
			})
		}
	}
}

// ─── 7. FitBrake × all vehicles × all part CCs ───────────────────────────────

// TestConfidenceMatrix_FitBrake_AllVehicles tests FitBrake (CCMargin=1000) across
// the full 16 × 25 CC grid. Expected values:
//   - vehicleCC > 0 AND partCC > 0 AND |diff| ≤ 1000 → 0.85 (CC match within tolerance)
//   - vehicleCC > 0 AND partCC > 0 AND |diff| > 1000 → 0.60 (CC differs — trim/sport)
//   - either CC = 0                                   → 0.75 (may vary by trim level)
//
// Total: 16 × 25 = 400 sub-tests.
func TestConfidenceMatrix_FitBrake_AllVehicles(t *testing.T) {
	s := &SmartSearch{}
	rule := CategoryRule{Driver: FitBrake, CCMargin: 1000}

	for _, v := range cmxVehicleCCs {
		v := v
		for _, pcc := range cmxPartCCs {
			pcc := pcc
			name := fmt.Sprintf("%s/vCC=%d/pCC=%d", v.name, v.cc, pcc)
			t.Run(name, func(t *testing.T) {
				conf, _ := s.computeConfidenceForVehicle(rule, v.cc, pcc, "", "")
				want := cmxExpectedFitBrakeConf(v.cc, pcc)
				if math.Abs(conf-want) > 1e-9 {
					t.Errorf("FitBrake vehicleCC=%d partCC=%d: got %.4f, want %.4f",
						v.cc, pcc, conf, want)
				}
			})
		}
	}
}

// ─── 8. Real vehicle × exact engine part CC match ────────────────────────────

// TestConfidenceMatrix_RealVehiclePartCombinations verifies that every real
// seed-DB vehicle, when paired with an engine part whose CC exactly matches
// the vehicle's displacement, yields confidence 0.95 and the canonical note
// "Engine CC exact match".
//
// This exercises the real cross-reference scenario: e.g. water pump 25100-2B000
// carries the engine's own displacement and should be an exact fit.
//
// Total: 16 sub-tests.
func TestConfidenceMatrix_RealVehiclePartCombinations(t *testing.T) {
	s := &SmartSearch{}
	rule := CategoryRule{Driver: FitEngine, CCMargin: 500}

	for _, v := range cmxVehicleCCs {
		v := v
		name := fmt.Sprintf("%s/exactMatch/vCC=%d/pCC=%d", v.name, v.cc, v.cc)
		t.Run(name, func(t *testing.T) {
			conf, note := s.computeConfidenceForVehicle(rule, v.cc, v.cc, "", "")

			if conf != 0.95 {
				t.Errorf("%s: vehicleCC=%d partCC=%d: got conf=%.4f, want 0.95",
					v.name, v.cc, v.cc, conf)
			}
			const wantNote = "Engine CC exact match"
			if note != wantNote {
				t.Errorf("%s: note=%q, want %q", v.name, note, wantNote)
			}
		})
	}
}

package service

import (
	"testing"
)

// TestSearchCombined_HKScopeGuard_RejectsNonHK verifies the guard added in
// the 2026-08-22 audit fix rejects non-HK OEMs at combined-mode entry
// (before the strategy fan-out) — the key user-facing regression the audit
// identified. Before the fix, combined mode bypassed the guard entirely
// (only searchByOEM called IsHKOEM) and non-HK queries ran the full
// 12-strategy fan-out, hitting 20s browser timeouts.
//
// This test uses a bare SmartSearch{} — no db, no TecDoc, no dealer lookup
// — so if the guard fails to fire, the strategy loop would try to run and
// panic on the nil db. That panic vs a clean "hk_scope_rejected" response
// is the signal.
func TestSearchCombined_HKScopeGuard_RejectsNonHK(t *testing.T) {
	cases := []struct {
		name          string
		oem           string
		wantMake      string
		wantReasonSub string
	}{
		{
			name:          "Toyota 5-5 dashed",
			oem:           "90915-YZZE1",
			wantMake:      "Toyota",
			wantReasonSub: "Toyota",
		},
		{
			name:          "Nissan 5-5 dashed",
			oem:           "15208-9F600",
			wantMake:      "Nissan",
			wantReasonSub: "Nissan",
		},
		{
			name:          "BMW multi-dash (was leaking pre-fix)",
			oem:           "11-42-7-521-353",
			wantMake:      "BMW",
			wantReasonSub: "BMW",
		},
		{
			name:          "Honda 5-5-dashed",
			oem:           "15400-PLM-A02",
			wantMake:      "Honda",
			wantReasonSub: "Honda",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &SmartSearch{}
			// Call searchCombined directly. If the guard fails, we panic
			// on nil db somewhere in the strategy fan-out — that's the
			// regression signal we're guarding against.
			resp, err := s.searchCombined(tc.oem, 0, 0, "", "", 1, 20, nil)
			if err != nil {
				t.Fatalf("searchCombined(%q) unexpected err: %v", tc.oem, err)
			}
			if resp == nil {
				t.Fatalf("searchCombined(%q) returned nil resp", tc.oem)
			}
			if resp.SearchStrategy != "hk_scope_rejected" {
				t.Errorf("searchCombined(%q).SearchStrategy = %q, want %q (guard must fire before fan-out)",
					tc.oem, resp.SearchStrategy, "hk_scope_rejected")
			}
			if resp.Total != 0 {
				t.Errorf("searchCombined(%q).Total = %d, want 0", tc.oem, resp.Total)
			}
			if len(resp.Warnings) == 0 {
				t.Errorf("searchCombined(%q) missing Warnings — user must see WHY it was rejected", tc.oem)
			}
			// Verify the SuggestedMake made it into the warnings so the
			// UI can render "Try [make] parts instead".
			joined := ""
			for _, w := range resp.Warnings {
				joined += w + " | "
			}
			if !contains(joined, tc.wantReasonSub) {
				t.Errorf("searchCombined(%q) warnings = %q, want containing %q",
					tc.oem, joined, tc.wantReasonSub)
			}
		})
	}
}

// TestSearchCombined_HKScopeGuard_AllowsPartialStem verifies the guard does
// NOT reject partial-OEM stems like "97133" (Hyundai Tucson cabin filter
// family). These are format="unknown" AND have no SuggestedMake, so the
// guard must pass them through to the strategy fan-out (where prefix
// inference and cache can potentially resolve them).
//
// Regression protection: if someone tightens the guard to reject ALL
// format="unknown" queries, partial-stem search breaks.
func TestSearchCombined_HKScopeGuard_AllowsPartialStem(t *testing.T) {
	// Not attempting to run the fan-out here (nil db would panic). We
	// verify the guard predicate directly instead — the same predicate
	// searchCombined applies at line 1.
	cases := []struct {
		oem  string
		name string
	}{
		{"97133", "5-digit prefix stem"},
		{"26350", "5-digit oil filter stem"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scope := IsHKOEM(tc.oem)
			// The searchCombined predicate is:
			//   !scope.IsHK && (scope.Format != "unknown" || scope.SuggestedMake != "")
			// For a bare 5-digit stem, all three of those SHOULD be false
			// (Format=unknown, SuggestedMake=""), so the predicate returns
			// false → guard does not fire → fan-out runs.
			shouldReject := !scope.IsHK && (scope.Format != "unknown" || scope.SuggestedMake != "")
			if shouldReject {
				t.Errorf("guard would REJECT partial stem %q — scope=%+v (partial stems must pass through)",
					tc.oem, scope)
			}
		})
	}
}

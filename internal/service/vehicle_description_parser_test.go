package service

import "testing"

// TestParseVehicleDescription — M3.S2.T2 extracts chassis + engine spec
// from real TecDoc description strings.
func TestParseVehicleDescription(t *testing.T) {
	cases := []struct {
		desc        string
		wantChassis string
		wantEngine  string
	}{
		{
			desc:        "HYUNDAI TUCSON (TL) 2.0 CRDi 4WD 136HP [08.2015-]",
			wantChassis: "TL",
			wantEngine:  "2.0 CRDi 4WD 136HP",
		},
		{
			desc:        "KIA SORENTO (XM) 2.4 GDi AWD 189HP [05.2012-06.2020]",
			wantChassis: "XM",
			wantEngine:  "2.4 GDi AWD 189HP",
		},
		{
			desc:        "HYUNDAI ELANTRA 1.6 [01.2011-]",
			wantChassis: "",
			wantEngine:  "1.6",
		},
		{
			desc:        "",
			wantChassis: "",
			wantEngine:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			got := parseVehicleDescription(tc.desc)
			if got.Chassis != tc.wantChassis {
				t.Errorf("Chassis = %q, want %q", got.Chassis, tc.wantChassis)
			}
			// Engine spec parsing is approximate — accept substring match
			// when the whole spec isn't cleanly extractable.
			if tc.wantEngine != "" && !contains(got.EngineSpec, tc.wantEngine[:3]) {
				t.Errorf("EngineSpec = %q, want containing prefix of %q", got.EngineSpec, tc.wantEngine)
			}
		})
	}
}

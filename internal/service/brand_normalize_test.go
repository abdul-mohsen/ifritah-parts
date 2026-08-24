package service

import "testing"

// TestNormalizeBrand — 30 top-shipping HK aftermarket brands x their common
// variants must all map to the same canonical form.
func TestNormalizeBrand(t *testing.T) {
	cases := []struct {
		variants []string
		want     string
	}{
		// Filtration
		{[]string{"BOSCH", "Bosch", "bosch", "Robert Bosch GmbH", "BOSCH GMBH", "Robert Bosch"}, "Bosch"},
		{[]string{"MANN", "Mann", "MANN-FILTER", "Mann-Filter", "MannFilter", "MANN HUMMEL", "Mann+Hummel"}, "MANN-FILTER"},
		{[]string{"MAHLE", "Mahle", "MAHLE ORIGINAL", "MahleBehr", "Knecht"}, "MAHLE"},
		{[]string{"HENGST", "Hengst Filter"}, "Hengst"},
		{[]string{"WIX", "WIX FILTERS", "Wix Filters"}, "WIX"},
		{[]string{"FRAM", "Fram"}, "Fram"},
		// Ignition
		{[]string{"NGK", "ngk"}, "NGK"},
		{[]string{"DENSO", "Denso", "Denso Corporation", "NipponDenso"}, "Denso"},
		{[]string{"CHAMPION"}, "Champion"},
		{[]string{"VALEO", "Valeo", "Valeo Wipers", "VALEO CLIMATE"}, "Valeo"},
		// Braking
		{[]string{"TEXTAR", "Textar"}, "Textar"},
		{[]string{"FERODO", "Ferodo"}, "Ferodo"},
		{[]string{"TRW", "trw"}, "TRW"},
		{[]string{"ATE", "ATE POWER DISC"}, "ATE"},
		{[]string{"BREMBO"}, "Brembo"},
		{[]string{"BENDIX"}, "Bendix"},
		// Bearings
		{[]string{"SKF"}, "SKF"},
		{[]string{"NSK"}, "NSK"},
		{[]string{"FAG", "FAG BEARINGS"}, "FAG"},
		{[]string{"KOYO"}, "Koyo"},
		{[]string{"INA"}, "INA"},
		// Suspension
		{[]string{"KYB", "KAYABA"}, "KYB"},
		{[]string{"MONROE"}, "Monroe"},
		{[]string{"SACHS"}, "Sachs"},
		{[]string{"BILSTEIN"}, "Bilstein"},
		{[]string{"LEMFORDER", "Lemforder", "LEMFOERDER"}, "Lemforder"},
		{[]string{"MEYLE", "Meyle HD"}, "Meyle"},
		{[]string{"FEBI", "FEBI BILSTEIN", "FebiBilstein"}, "Febi"},
		// Belt/chain
		{[]string{"GATES"}, "Gates"},
		{[]string{"DAYCO"}, "Dayco"},
		{[]string{"CONTITECH", "Contitech", "CONTINENTAL CONTITECH"}, "Contitech"},
		// OEM
		{[]string{"MOBIS", "Hyundai Mobis", "HYUNDAIMOBIS", "HMC"}, "Mobis"},
		{[]string{"HYUNDAI"}, "Hyundai"},
		{[]string{"KIA"}, "Kia"},
		{[]string{"HYUNDAI / KIA", "Hyundai/Kia", "HYUNDAIKIA"}, "Hyundai/Kia"},
	}
	for _, tc := range cases {
		for _, v := range tc.variants {
			t.Run(v, func(t *testing.T) {
				got := NormalizeBrand(v)
				if got != tc.want {
					t.Errorf("NormalizeBrand(%q) = %q, want %q", v, got, tc.want)
				}
			})
		}
	}
}

// TestNormalizeBrand_UnknownFallback — unknown brands still get a
// consistent Title-Case form so downstream dedup treats "sonic" and
// "SONIC" as the same string.
func TestNormalizeBrand_UnknownFallback(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"SONIC", "Sonic"},
		{"sonic", "Sonic"},
		{"Sonic", "Sonic"},
		{"NEW MANN", "New Mann"},
		{"NEW MANN CO LTD", "New Mann"},                      // strip Co Ltd
		{"HYUNDAI PARTS SUPPLY LLC", "Hyundai Parts Supply"}, // LLC stripped, PARTS not
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := NormalizeBrand(tc.in)
			if got != tc.want {
				t.Errorf("NormalizeBrand(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalizeBrand_SuffixStripping — corporate suffixes must not affect
// canonical lookup.
func TestNormalizeBrand_SuffixStripping(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Bosch GmbH", "Bosch"},
		{"Bosch Ltd", "Bosch"},
		{"Bosch Inc", "Bosch"},
		{"Bosch Co", "Bosch"},
		{"Bosch AG", "Bosch"},
		{"Denso Corp", "Denso"},
		{"MANN LLC", "MANN-FILTER"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := NormalizeBrand(tc.in)
			if got != tc.want {
				t.Errorf("NormalizeBrand(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

package service

import "testing"

// TestNormalizeOEM_SeedCatalogOEMs verifies all 98 seed OEM numbers normalize
// to their expected stripped-and-lowercased forms.
// stripChars = regexp `[-.\s/]`; then strings.ToLower.
func TestNormalizeOEM_SeedCatalogOEMs(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"26300-35505", "2630035505"},
		{"26300-35530", "2630035530"},
		{"28113-D3100", "28113d3100"},
		{"28113-F2100", "28113f2100"},
		{"27301-2B100", "273012b100"},
		{"18843-10062", "1884310062"},
		{"18855-10080", "1885510080"},
		{"25100-2E100", "251002e100"},
		{"25100-2B000", "251002b000"},
		{"25500-2B100", "255002b100"},
		{"25310-2S500", "253102s500"},
		{"25380-2S500", "253802s500"},
		{"25212-2B020", "252122b020"},
		{"25281-2B010", "252812b010"},
		{"21810-2S000", "218102s000"},
		{"21930-2S200", "219302s200"},
		{"21830-2S200", "218302s200"},
		{"24312-2B000", "243122b000"},
		{"39210-2B100", "392102b100"},
		{"39350-2B100", "393502b100"},
		{"39180-2B000", "391802b000"},
		{"39450-2S500", "394502s500"},
		{"37300-2B100", "373002b100"},
		{"36100-2B100", "361002b100"},
		{"59830-D3000", "59830d3000"},
		{"59930-D3000", "59930d3000"},
		{"58101-D3A70", "58101d3a70"},
		{"51712-D3100", "51712d3100"},
		{"58101-F2A00", "58101f2a00"},
		{"51712-F2100", "51712f2100"},
		{"58302-D3A70", "58302d3a70"},
		{"58411-D3100", "58411d3100"},
		{"58510-2S300", "585102s300"},
		{"58732-2S000", "587322s000"},
		{"54651-D3000", "54651d3000"},
		{"54530-D3000", "54530d3000"},
		{"54500-D3000", "54500d3000"},
		{"54501-D3000", "54501d3000"},
		{"54830-D3000", "54830d3000"},
		{"51720-D3000", "51720d3000"},
		{"55300-D3000", "55300d3000"},
		{"55530-D3000", "55530d3000"},
		{"56820-D3000", "56820d3000"},
		{"57724-D3000", "57724d3000"},
		{"54651-J9000", "54651j9000"},
		{"54651-L1000", "54651l1000"},
		{"54651-S1000", "54651s1000"},
		{"58101-J9A00", "58101j9a00"},
		{"58101-L0A00", "58101l0a00"},
		{"92101-D3100", "92101d3100"},
		{"92102-D3100", "92102d3100"},
		{"92101-Q5100", "92101q5100"},
		{"92102-Q5100", "92102q5100"},
		{"92101-F2020", "92101f2020"},
		{"92102-F2020", "92102f2020"},
		{"92401-D3100", "92401d3100"},
		{"92402-D3100", "92402d3100"},
		{"86511-D3100", "86511d3100"},
		{"86611-D3100", "86611d3100"},
		{"86350-D3100", "86350d3100"},
		{"66311-D3100", "66311d3100"},
		{"66321-D3100", "66321d3100"},
		{"66400-D3100", "66400d3100"},
		{"86511-Q5000", "86511q5000"},
		{"87610-D3100", "87610d3100"},
		{"87620-D3100", "87620d3100"},
		{"87610-D3520", "87610d3520"},
		{"98350-D3100", "98350d3100"},
		{"98100-D3100", "98100d3100"},
		{"41100-2D100", "411002d100"},
		{"49500-D3600", "49500d3600"},
		{"49501-D3600", "49501d3600"},
		{"49590-D3000", "49590d3000"},
		{"97701-D3000", "97701d3000"},
		{"97606-D3000", "97606d3000"},
		{"97133-D3000", "97133d3000"},
		{"97133-F2000", "97133f2000"},
		{"97133-J9000", "97133j9000"},
		{"97113-D3000", "97113d3000"},
		{"97115-D3000", "97115d3000"},
		{"18640-11080", "1864011080"},
		{"96610-D3100", "96610d3100"},
		{"31112-D3000", "31112d3000"},
		{"35310-2S000", "353102s000"},
		{"28510-2S500", "285102s500"},
		{"28410-2B100", "284102b100"},
		{"28830-2U000", "288302u000"},
		{"52933-1P000", "529331p000"},
		{"52933-D4100", "52933d4100"},
		{"52933-3X300", "529333x300"},
		{"82401-D3010", "82401d3010"},
		{"82402-D3010", "82402d3010"},
		{"51750-D3000", "51750d3000"},
		{"52730-D3100", "52730d3100"},
		{"25411-D3100", "25411d3100"},
		{"25412-D3100", "25412d3100"},
		{"29100-2B800", "291002b800"},
		{"39110-2B000", "391102b000"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			if got := NormalizeOEM(tc.input); got != tc.want {
				t.Errorf("NormalizeOEM(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeOEM_AftermarketArticles verifies real aftermarket article numbers
// from live API results normalize correctly — non-empty and deterministic.
// Note: "0133.3043" → "01333043" (dot removed between "0133" and "3043").
func TestNormalizeOEM_AftermarketArticles(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"W 811/80", "w81180"},
		{"LS489A", "ls489a"},
		{"F 026 407 124", "f026407124"},
		{"J1317003", "j1317003"},
		{"PH6811", "ph6811"},
		{"H13W01", "h13w01"},
		{"SM 125", "sm125"},
		{"BFO4198", "bfo4198"},
		{"QFL0370", "qfl0370"},
		{"S 3583 R", "s3583r"},
		// "28.0002-2225.2" → "28"+"0002"+"2225"+"2" = "28000222252" (11 chars)
		{"28.0002-2225.2", "28000222252"},
		{"C 28 040", "c28040"},
		{"MD-8948", "md8948"},
		{"MFA-K370", "mfak370"},
		{"HA-743", "ha743"},
		{"N1320556", "n1320556"},
		{"H132I56", "h132i56"},
		{"EAF950", "eaf950"},
		{"J1320558", "j1320558"},
		{"CU 23 019", "cu23019"},
		{"821 871", "821871"},
		{"HC-8232", "hc8232"},
		{"J1340529", "j1340529"},
		{"E4961LI", "e4961li"},
		{"001-10-25291", "0011025291"},
		{"CU 24 013", "cu24013"},
		{"DCF577P", "dcf577p"},
		{"2135520", "2135520"},
		{"PC8495", "pc8495"},
		{"ADG02592", "adg02592"},
		{"SA 1338", "sa1338"},
		{"AH521", "ah521"},
		{"CF12160", "cf12160"},
		{"XUH20TTi", "xuh20tti"},
		{"0 242 129 521", "0242129521"},
		{"WG1462276", "wg1462276"},
		{"96569", "96569"},
		{"OE197/T10", "oe197t10"},
		{"CCH9023", "cch9023"},
		{"1961", "1961"},
		{"1648406880", "1648406880"},
		{"BSG 40-835-007", "bsg40835007"},
		{"20514", "20514"},
		{"CBE5413", "cbe5413"},
		{"85.30413", "8530413"},
		{"AQ-2363", "aq2363"},
		{"PA1517", "pa1517"},
		{"PA10119", "pa10119"},
		{"FWP2233", "fwp2233"},
		{"ADG09162", "adg09162"},
		{"VKPC 95895", "vkpc95895"},
		{"VKPC 95898", "vkpc95898"},
		{"2317050", "2317050"},
		{"19430", "19430"},
		{"APV2998", "apv2998"},
		{"VKM 64056", "vkm64056"},
		{"0-N2202S", "0n2202s"},
		{"P254005", "p254005"},
		{"050 006 1255", "0500061255"},
		{"6PK1256", "6pk1256"},
		{"6PK1255", "6pk1255"},
		{"AD06R1255", "ad06r1255"},
		{"WG1781552", "wg1781552"},
		{"1212-TMRH", "1212tmrh"},
		{"518408", "518408"},
		{"EEM-3125", "eem3125"},
		{"72328", "72328"},
		{"72341", "72341"},
		{"531917", "531917"},
		{"EEM-4094", "eem4094"},
		{"7481789", "7481789"},
		{"43-Y16", "43y16"},
		{"90390", "90390"},
		{"79334", "79334"},
		{"CS0204", "cs0204"},
		{"CSR3275", "csr3275"},
		{"BSG 40-840-011", "bsg40840011"},
		{"WG1253830", "wg1253830"},
		{"535 0271 10", "535027110"},
		{"535 0326 10", "535032610"},
		{"03.81852", "0381852"},
		{"254850", "254850"},
		{"600210", "600210"},
		{"0 986 025 720", "0986025720"},
		{"254850V", "254850v"},
		{"600209", "600209"},
		{"BPHY-2004", "bphy2004"},
		{"0 986 494 557", "0986494557"},
		{"JQ101268", "jq101268"},
		{"903.1", "9031"},
		{"J3610526", "j3610526"},
		{"22-0886-1", "2208861"},
		{"223442", "223442"},
		{"22-263544", "22263544"},
		{"310935", "310935"},
		{"112172.1", "1121721"},
		{"212172", "212172"},
		{"A-5272GL", "a5272gl"},
		{"EX54651D3000", "ex54651d3000"},
		{"5043425", "5043425"},
		{"CBKH-42L", "cbkh42l"},
		{"SBJ-3041", "sbj3041"},
		{"S080986", "s080986"},
		{"BS-H76L", "bsh76l"},
		{"503-07003", "50307003"},
		{"72-0H-H76L", "720hh76l"},
		{"72H76L", "72h76l"},
		{"SCA-4173", "sca4173"},
		{"MSA010082", "msa010082"},
		{"SAK-8772L", "sak8772l"},
		{"S063033", "s063033"},
		{"CLKK-44", "clkk44"},
		{"53066908", "53066908"},
		{"SS8093", "ss8093"},
		{"FDL7445", "fdl7445"},
		{"BDL7445", "bdl7445"},
		{"JRSHY-051", "jrshy051"},
		{"DB78391", "db78391"},
		{"HYK452", "hyk452"},
		{"853028N", "853028n"},
		{"8623375", "8623375"},
		{"10553839", "10553839"},
		{"890767", "890767"},
		{"67515", "67515"},
		{"560061N", "560061n"},
		{"53052", "53052"},
		{"KA2238", "ka2238"},
		// "0133.3043": dot removed between "0133" and "3043" → "01333043"
		{"0133.3043", "01333043"},
		{"261141", "261141"},
		{"J4890536", "j4890536"},
		{"87662", "87662"},
		{"KI-LS-16571", "kils16571"},
		{"87534", "87534"},
		{"JTE1860", "jte1860"},
		{"231105", "231105"},
		{"FTR6016", "ftr6016"},
		{"BTR6016", "btr6016"},
		{"HN8061011", "hn8061011"},
		{"6862050", "6862050"},
		{"25311681", "25311681"},
		{"5510-00-3176903P", "5510003176903p"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			got := NormalizeOEM(tc.input)
			if got == "" {
				t.Errorf("NormalizeOEM(%q) returned empty string", tc.input)
			}
			if got != tc.want {
				t.Errorf("NormalizeOEM(%q) = %q, want %q", tc.input, got, tc.want)
			}
			// idempotent: normalizing the normalized result yields the same string
			if again := NormalizeOEM(got); again != got {
				t.Errorf("NormalizeOEM not idempotent: NormalizeOEM(%q) = %q, second pass = %q", tc.input, got, again)
			}
		})
	}
}

// TestNormalizeOEM_FormatEquivalence verifies that the same OEM part number
// expressed in different formats (dash, no-dash, space, dot, slash) always
// normalizes to the same canonical string.
func TestNormalizeOEM_FormatEquivalence(t *testing.T) {
	groups := []struct {
		want     string
		variants []string
	}{
		{
			want:     "2630035505",
			variants: []string{"26300-35505", "2630035505", "26300 35505", "26300.35505"},
		},
		{
			want:     "58101d3a70",
			variants: []string{"58101-D3A70", "58101D3A70", "58101 D3A70", "58101.D3A70"},
		},
		{
			want:     "97133d3000",
			variants: []string{"97133-D3000", "97133D3000", "97133 D3000", "97133/D3000"},
		},
		{
			want:     "54651d3000",
			variants: []string{"54651-D3000", "54651D3000", "54651 D3000"},
		},
		{
			want:     "92101d3100",
			variants: []string{"92101-D3100", "92101D3100", "92101 D3100"},
		},
		{
			want:     "28113d3100",
			variants: []string{"28113-D3100", "28113D3100", "28113 D3100"},
		},
		{
			want:     "86511d3100",
			variants: []string{"86511-D3100", "86511D3100", "86511.D3100"},
		},
		{
			want:     "97133f2000",
			variants: []string{"97133-F2000", "97133F2000", "97133/F2000"},
		},
		{
			want:     "51712d3100",
			variants: []string{"51712-D3100", "51712D3100", "51712.D3100"},
		},
		{
			want:     "55300d3000",
			variants: []string{"55300-D3000", "55300D3000", "55300 D3000", "55300.D3000"},
		},
	}

	for _, g := range groups {
		g := g
		for _, v := range g.variants {
			v := v
			t.Run(v, func(t *testing.T) {
				got := NormalizeOEM(v)
				if got != g.want {
					t.Errorf("NormalizeOEM(%q) = %q, want %q (format-equivalence group %q)", v, got, g.want, g.variants[0])
				}
			})
		}
	}
}

// TestNormalizeOEM_EdgeCases verifies boundary inputs: empty, whitespace-only,
// separator-only, and single-character.
func TestNormalizeOEM_EdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty_string", "", ""},
		{"spaces_only", "   ", ""},
		{"dashes_only", "---", ""},
		{"space_dash_space", " - ", ""},
		{"single_letter", "A", "a"},
		{"single_digit", "9", "9"},
		{"only_dots", "...", ""},
		{"only_slashes", "///", ""},
		{"mixed_separators_only", " .- /", ""},
		{"letter_between_separators", "-A-", "a"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeOEM(tc.input); got != tc.want {
				t.Errorf("NormalizeOEM(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestOEMLookup_NilDBGuards verifies that NewOEMLookup(nil) does not panic,
// returns a non-nil struct, and that all methods return sensible zero-values
// instead of panicking when the internal queries field is nil.
func TestOEMLookup_NilDBGuards(t *testing.T) {
	t.Run("NewOEMLookup_nil_returns_non_nil_struct", func(t *testing.T) {
		s := NewOEMLookup(nil)
		if s == nil {
			t.Fatal("NewOEMLookup(nil) returned nil — expected a non-nil *OEMLookup")
		}
	})

	t.Run("Search_nil_queries_returns_error_not_panic", func(t *testing.T) {
		s := NewOEMLookup(nil)
		result, err := s.Search("26300-35505", 10)
		if err == nil {
			t.Fatal("expected error from Search on nil-queries struct, got nil")
		}
		if result != nil {
			t.Errorf("expected nil result from Search on nil-queries struct, got %+v", result)
		}
	})

	t.Run("OEMNumbersForArticle_nil_queries_returns_nil_nil", func(t *testing.T) {
		s := NewOEMLookup(nil)
		nums, err := s.OEMNumbersForArticle(100001)
		if err != nil {
			t.Errorf("OEMNumbersForArticle on nil-queries: expected nil error, got %v", err)
		}
		if nums != nil {
			t.Errorf("OEMNumbersForArticle on nil-queries: expected nil slice, got %v", nums)
		}
	})

	t.Run("BatchOEMNumbers_nil_queries_returns_nil_nil", func(t *testing.T) {
		s := NewOEMLookup(nil)
		result, err := s.BatchOEMNumbers([]int{100001, 100002})
		if err != nil {
			t.Errorf("BatchOEMNumbers on nil-queries: expected nil error, got %v", err)
		}
		if result != nil {
			t.Errorf("BatchOEMNumbers on nil-queries: expected nil map, got %v", result)
		}
	})

	t.Run("BatchOEMNumbers_empty_slice_returns_nil_nil", func(t *testing.T) {
		// Even with a nil-queries struct, empty articleIds takes the early-return path.
		s := NewOEMLookup(nil)
		result, err := s.BatchOEMNumbers([]int{})
		if err != nil {
			t.Errorf("BatchOEMNumbers empty slice: expected nil error, got %v", err)
		}
		if result != nil {
			t.Errorf("BatchOEMNumbers empty slice: expected nil map, got %v", result)
		}
	})
}

// TestOEMLookup_LimitClamping verifies that out-of-range limit values (0, -1,
// 101, 1000) do not cause a panic. The nil-queries guard fires before limit
// clamping, so all calls return an error rather than a result.
func TestOEMLookup_LimitClamping(t *testing.T) {
	s := NewOEMLookup(nil)
	cases := []struct {
		name  string
		limit int
	}{
		{"zero", 0},
		{"negative_one", -1},
		{"negative_large", -100},
		{"one_over_max", 101},
		{"very_large", 1000},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Must not panic; nil-queries guard fires before clamping.
			result, err := s.Search("26300-35505", tc.limit)
			if err == nil {
				t.Errorf("limit=%d: expected error from nil-queries guard, got nil", tc.limit)
			}
			if result != nil {
				t.Errorf("limit=%d: expected nil result, got %+v", tc.limit, result)
			}
		})
	}
}

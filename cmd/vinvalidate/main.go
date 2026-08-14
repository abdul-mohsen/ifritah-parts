package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"parts-engine/internal/service"
)

// Ground truth from Wikibooks WMI database (authoritative source)
// https://en.wikibooks.org/wiki/Vehicle_Identification_Numbers_(VIN_codes)/World_Manufacturer_Identifier_(WMI)
var groundTruth = map[string]string{
	// Hyundai
	"KMH": "HYUNDAI", "KM8": "HYUNDAI", "5NP": "HYUNDAI", "5NM": "HYUNDAI",
	"KMJ": "HYUNDAI", "TMD": "HYUNDAI", "NLH": "HYUNDAI", "NLJ": "HYUNDAI",
	"MAL": "HYUNDAI", "TMA": "HYUNDAI", "TMC": "HYUNDAI", "KME": "HYUNDAI",
	"MF3": "HYUNDAI", "LBE": "HYUNDAI", "MB2": "HYUNDAI", "Z94": "HYUNDAI",
	"AC5": "HYUNDAI", "5NT": "HYUNDAI", "7YA": "HYUNDAI", "2HM": "HYUNDAI",
	// Kia
	"KNA": "KIA", "KND": "KIA", "KNE": "KIA", "KNH": "KIA",
	"5XY": "KIA", "5XX": "KIA", "U5Y": "KIA", "U6Y": "KIA",
	"LJD": "KIA", "KNB": "KIA", "KNC": "KIA",
	"3KP": "KIA", "3KM": "KIA", "MZB": "KIA",
	// Genesis
	"KMT": "GENESIS", "KMU": "GENESIS",
	// Toyota
	"JTD": "TOYOTA", "JTE": "TOYOTA", "JTN": "TOYOTA", "JTK": "TOYOTA",
	"JTL": "TOYOTA", "JTM": "TOYOTA", "JTB": "TOYOTA", "JTF": "TOYOTA",
	"JTP": "TOYOTA", "JT2": "TOYOTA", "JT3": "TOYOTA", "JT4": "TOYOTA",
	"2T1": "TOYOTA", "2T3": "TOYOTA",
	"4T1": "TOYOTA", "4T3": "TOYOTA", "4T4": "TOYOTA", "4TA": "TOYOTA",
	"5TD": "TOYOTA", "5TF": "TOYOTA", "5TB": "TOYOTA", "5TE": "TOYOTA", "5YF": "TOYOTA",
	"MR0": "TOYOTA", "MR1": "TOYOTA", "MR2": "TOYOTA", "MR3": "TOYOTA",
	"MHF": "TOYOTA", "MBJ": "TOYOTA", "MHK": "TOYOTA",
	"LFM": "TOYOTA", "LTV": "TOYOTA", "LVG": "TOYOTA",
	"AHT": "TOYOTA", "NMT": "TOYOTA", "SB1": "TOYOTA", "VNK": "TOYOTA",
	"1NX": "TOYOTA", "3TM": "TOYOTA", "3TY": "TOYOTA", "3MY": "TOYOTA",
	"7MU": "TOYOTA", "7SV": "TOYOTA", "WZ1": "TOYOTA",
	// Lexus
	"JTJ": "LEXUS", "JTH": "LEXUS", "JT6": "LEXUS", "JT8": "LEXUS",
	"2T2": "LEXUS", "58A": "LEXUS",
	// Honda
	"JHM": "HONDA", "JHL": "HONDA", "SHH": "HONDA", "93H": "HONDA",
	"1HG": "HONDA", "2HG": "HONDA", "2HK": "HONDA", "2HJ": "HONDA",
	"5FN": "HONDA", "5FP": "HONDA", "5J6": "HONDA", "5JR": "HONDA", "5KB": "HONDA",
	"MHR": "HONDA", "MRH": "HONDA", "MLH": "HONDA",
	"LHG": "HONDA", "LUC": "HONDA", "LVH": "HONDA",
	"19X": "HONDA", "7FA": "HONDA", "3CZ": "HONDA", "3HG": "HONDA",
	"NLA": "HONDA", "MAK": "HONDA", "PAD": "HONDA", "PMH": "HONDA",
	"NFB": "HONDA", "RLH": "HONDA", "8C3": "HONDA",
	// Acura
	"19U": "ACURA", "JH4": "ACURA", "19V": "ACURA",
	"5J8": "ACURA", "5FR": "ACURA", "2HH": "ACURA", "2HN": "ACURA", "3HD": "ACURA",
	// Nissan
	"JN1": "NISSAN", "JN3": "NISSAN", "JN6": "NISSAN", "JN8": "NISSAN",
	"1N4": "NISSAN", "1N6": "NISSAN", "3N1": "NISSAN", "3N6": "NISSAN", "3N8": "NISSAN",
	"5N1": "NISSAN", "MNT": "NISSAN", "LGB": "NISSAN", "LJN": "NISSAN",
	"VSK": "NISSAN", "Z8N": "NISSAN", "MDH": "NISSAN",
	// Infiniti
	"JNK": "INFINITI", "5N3": "INFINITI", "JNR": "INFINITI", "SJK": "INFINITI", "3PC": "INFINITI",
	// Mazda
	"JM1": "MAZDA", "JM3": "MAZDA", "JM5": "MAZDA", "JM6": "MAZDA", "JM7": "MAZDA",
	"JMZ": "MAZDA", "JM0": "MAZDA", "JM2": "MAZDA", "JM4": "MAZDA",
	"3MZ": "MAZDA", "3MD": "MAZDA", "3MJ": "MAZDA", "3MV": "MAZDA", "7MM": "MAZDA",
	// Subaru
	"JF1": "SUBARU", "JF2": "SUBARU", "JF3": "SUBARU", "4S3": "SUBARU", "4S4": "SUBARU",
	// Mitsubishi
	"JA3": "MITSUBISHI", "JA4": "MITSUBISHI", "JA7": "MITSUBISHI",
	"JMB": "MITSUBISHI", "JMY": "MITSUBISHI", "JMA": "MITSUBISHI", "JMF": "MITSUBISHI",
	"4A3": "MITSUBISHI", "4A4": "MITSUBISHI", "4MB": "MITSUBISHI",
	"MMA": "MITSUBISHI", "MMB": "MITSUBISHI", "MMC": "MITSUBISHI", "MMT": "MITSUBISHI",
	"MMD": "MITSUBISHI", "MME": "MITSUBISHI", "ML3": "MITSUBISHI", "MK2": "MITSUBISHI",
	// Suzuki
	"JS1": "SUZUKI", "JS2": "SUZUKI", "JS3": "SUZUKI", "JSA": "SUZUKI", "JS4": "SUZUKI",
	"TSM": "SUZUKI", "MMS": "SUZUKI", "MHD": "SUZUKI", "MHY": "SUZUKI", "MBH": "SUZUKI",
	// Isuzu
	"JAA": "ISUZU", "JAL": "ISUZU", "JAB": "ISUZU", "JAC": "ISUZU",
	"MP1": "ISUZU", "MPA": "ISUZU",
	// Daihatsu
	"JDA": "DAIHATSU",
	// Ford
	"1FA": "FORD", "1FB": "FORD", "1FC": "FORD", "1FD": "FORD",
	"1FM": "FORD", "1FT": "FORD", "1FV": "FORD",
	"2FA": "FORD", "2FM": "FORD", "2FT": "FORD",
	"3FA": "FORD", "3FM": "FORD", "3FT": "FORD",
	"MAJ": "FORD", "MNB": "FORD", "WF0": "FORD", "VS6": "FORD",
	"NM0": "FORD", "MPB": "FORD",
	// Lincoln
	"1LN": "LINCOLN", "2LN": "LINCOLN", "3LN": "LINCOLN", "5LM": "LINCOLN", "2LM": "LINCOLN",
	// Chevrolet
	"1G1": "CHEVROLET", "1GC": "CHEVROLET", "1GN": "CHEVROLET",
	"2G1": "CHEVROLET", "2GC": "CHEVROLET", "2GN": "CHEVROLET",
	"3G1": "CHEVROLET", "3GC": "CHEVROLET", "3GN": "CHEVROLET",
	"1GB": "CHEVROLET", "1GA": "CHEVROLET",
	// GMC
	"1GK": "GMC", "1GT": "GMC", "2GK": "GMC", "2GT": "GMC", "3GK": "GMC", "3GT": "GMC",
	// Cadillac
	"1GY": "CADILLAC", "1G6": "CADILLAC", "3GY": "CADILLAC",
	// Buick
	"1G4": "BUICK", "2G4": "BUICK", "5GA": "BUICK",
	// Chrysler
	"1C3": "CHRYSLER", "2C3": "CHRYSLER", "2C4": "CHRYSLER", "3C4": "CHRYSLER",
	// RAM
	"1C6": "RAM", "3C6": "RAM",
	// Dodge
	"1B3": "DODGE", "1B7": "DODGE", "2B3": "DODGE", "2B5": "DODGE",
	"3B4": "DODGE", "1D7": "DODGE", "3D7": "DODGE",
	"1D3": "DODGE", "1D4": "DODGE",
	// Jeep
	"1J4": "JEEP", "1J8": "JEEP", "1C4": "JEEP", "ZAC": "JEEP",
	// Tesla
	"5YJ": "TESLA", "7SA": "TESLA", "LRW": "TESLA", "XP7": "TESLA", "7G2": "TESLA",
	// Rivian
	"7FC": "RIVIAN", "7PD": "RIVIAN",
	// Lucid
	"50E": "LUCID", "7UU": "LUCID",
	// BMW
	"WBA": "BMW", "WBS": "BMW", "WBY": "BMW", "WBX": "BMW",
	"5UX": "BMW", "5UM": "BMW", "5YM": "BMW", "WB5": "BMW",
	"4US": "BMW", "3MW": "BMW", "3MF": "BMW", "LBV": "BMW",
	// MINI
	"WMW": "MINI", "WMZ": "MINI",
	// Mercedes-Benz
	"WDB": "MERCEDES-BENZ", "WDC": "MERCEDES-BENZ", "WDD": "MERCEDES-BENZ",
	"WDF": "MERCEDES-BENZ", "WMX": "MERCEDES-BENZ",
	"W1K": "MERCEDES-BENZ", "W1N": "MERCEDES-BENZ", "W1V": "MERCEDES-BENZ",
	"4JG": "MERCEDES-BENZ", "VSA": "MERCEDES-BENZ", "55S": "MERCEDES-BENZ",
	"NLE": "MERCEDES-BENZ", "NMB": "MERCEDES-BENZ", "MBR": "MERCEDES-BENZ",
	// Smart
	"WME": "SMART", "W1A": "SMART",
	// Audi
	"WAU": "AUDI", "WA1": "AUDI", "WAP": "AUDI", "WUA": "AUDI", "WU1": "AUDI",
	// Volkswagen
	"WVW": "VOLKSWAGEN", "WVG": "VOLKSWAGEN", "WV2": "VOLKSWAGEN", "WV1": "VOLKSWAGEN",
	"3VW": "VOLKSWAGEN", "3VV": "VOLKSWAGEN", "1VW": "VOLKSWAGEN",
	"LFV": "VOLKSWAGEN", "LSV": "VOLKSWAGEN",
	// Porsche
	"WP0": "PORSCHE", "WP1": "PORSCHE",
	// Volvo
	"YV1": "VOLVO", "YV4": "VOLVO", "YV2": "VOLVO", "YV3": "VOLVO",
	"7JR": "VOLVO", "7JD": "VOLVO", "LYV": "VOLVO",
	// Polestar
	"LPS": "POLESTAR", "YSM": "POLESTAR", "YSR": "POLESTAR", "7SY": "POLESTAR",
	// Saab
	"YS3": "SAAB",
	// Fiat
	"ZFA": "FIAT", "ZFB": "FIAT", "ZFC": "FIAT",
	// Alfa Romeo
	"ZAR": "ALFA ROMEO", "ZAS": "ALFA ROMEO",
	// Ferrari
	"ZFF": "FERRARI",
	// Maserati
	"ZAM": "MASERATI", "ZN6": "MASERATI",
	// Lamborghini
	"ZHW": "LAMBORGHINI", "ZPB": "LAMBORGHINI",
	// Jaguar
	"SAJ": "JAGUAR", "SAD": "JAGUAR",
	// Land Rover
	"SAL": "LAND ROVER",
	// Aston Martin
	"SCF": "ASTON MARTIN", "SD7": "ASTON MARTIN",
	// Bentley
	"SCB": "BENTLEY", "SJA": "BENTLEY",
	// Rolls-Royce
	"SCA": "ROLLS-ROYCE", "SLA": "ROLLS-ROYCE",
	// McLaren
	"SBM": "MCLAREN",
	// Lotus
	"SCC": "LOTUS",
	// INEOS
	"SC6": "INEOS",
	// Peugeot
	"VF3": "PEUGEOT", "VR3": "PEUGEOT",
	// Citroen
	"VF7": "CITROEN", "VR7": "CITROEN",
	// DS
	"VR1": "DS",
	// Renault
	"VF1": "RENAULT", "VF2": "RENAULT", "VF6": "RENAULT", "UU1": "RENAULT",
	// Opel
	"W0L": "OPEL", "W0V": "OPEL",
	// SEAT
	"VSS": "SEAT",
	// Skoda
	"TMB": "SKODA",
	// BYD
	"LGX": "BYD", "LC0": "BYD", "LPE": "BYD",
	// Geely
	"L6T": "GEELY", "LB3": "GEELY",
	// NIO
	"LJ1": "NIO",
	// XPeng
	"L1N": "XPENG",
	// Li Auto
	"LW4": "LI AUTO",
	// Great Wall
	"LGW": "GREAT WALL",
	// Chery
	"LNN": "CHERY",
	// VinFast
	"RLL": "VINFAST", "RLN": "VINFAST",
	// Mahindra
	"MA1": "MAHINDRA", "MAB": "MAHINDRA",
	// Tata
	"MAT": "TATA",
	// Maruti Suzuki
	"MA3": "MARUTI SUZUKI",
}

// Year code chars for VIN position 10
var yearChars = []byte{
	'1', '2', '3', '4', '5', '6', '7', '8', '9', // 2001-2009
	'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'J', // 2010-2018
	'K', 'L', 'M', 'N', 'P', 'R', 'S', // 2019-2025
}

// VDS filler patterns to vary position 4 for model detection
var vdsFills = []byte{'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'J', 'K', 'L', 'M', 'N', 'P', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z'}

// buildVIN creates a synthetic 17-char VIN from WMI + pos4 char + year code
func buildVIN(wmi string, pos4 byte, yearChar byte) string {
	// Format: WMI(3) + pos4(1) + AAAA(4) + check(1) + year(1) + plant(1) + seq(5)
	// We use 'A' filler for positions 5-8, '0' for check digit, 'A' for plant
	return fmt.Sprintf("%s%cAAAA0%cA00001", wmi, pos4, yearChar)
}

type nhtsaResult struct {
	Make      string
	Model     string
	ModelYear string
	ErrorCode string
}

func nhtsaBatchDecode(vins []string) (map[string]nhtsaResult, error) {
	results := make(map[string]nhtsaResult)

	// NHTSA batch API: 50 VINs max per call
	const batchSize = 50
	for i := 0; i < len(vins); i += batchSize {
		end := i + batchSize
		if end > len(vins) {
			end = len(vins)
		}
		batch := vins[i:end]

		vinList := strings.Join(batch, ";")
		data := url.Values{"DATA": {vinList}, "format": {"json"}}

		resp, err := http.PostForm("https://vpic.nhtsa.dot.gov/api/vehicles/DecodeVINValuesBatch/", data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "NHTSA batch %d-%d error: %v\n", i, end, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var payload struct {
			Results []struct {
				VIN       string `json:"VIN"`
				Make      string `json:"Make"`
				Model     string `json:"Model"`
				ModelYear string `json:"ModelYear"`
				ErrorCode string `json:"ErrorCode"`
			} `json:"Results"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			fmt.Fprintf(os.Stderr, "NHTSA parse error batch %d-%d: %v\n", i, end, err)
			continue
		}
		for _, r := range payload.Results {
			results[r.VIN] = nhtsaResult{
				Make:      r.Make,
				Model:     r.Model,
				ModelYear: r.ModelYear,
				ErrorCode: r.ErrorCode,
			}
		}

		if end < len(vins) {
			time.Sleep(600 * time.Millisecond)
		}
	}
	return results, nil
}

func main() {
	useNHTSA := false
	for _, arg := range os.Args[1:] {
		if arg == "--nhtsa" {
			useNHTSA = true
		}
	}

	decoder := service.NewVINDecoder("", nil, nil)

	// Collect all WMIs from ground truth
	wmis := make([]string, 0, len(groundTruth))
	for wmi := range groundTruth {
		wmis = append(wmis, wmi)
	}

	// Generate VINs: each WMI × each year code × a few pos4 variants
	type testCase struct {
		VIN          string
		WMI          string
		ExpectedMake string
		YearChar     byte
		Pos4         byte
	}

	var cases []testCase
	for _, wmi := range wmis {
		expected := groundTruth[wmi]
		for _, yc := range yearChars {
			// Use 3 different pos4 chars for model diversity
			for _, p4 := range []byte{'A', 'D', 'K'} {
				vin := buildVIN(wmi, p4, yc)
				cases = append(cases, testCase{
					VIN:          vin,
					WMI:          wmi,
					ExpectedMake: expected,
					YearChar:     yc,
					Pos4:         p4,
				})
			}
		}
	}

	fmt.Printf("Generated %d test VINs from %d WMIs\n", len(cases), len(wmis))

	// Run local decode on all
	type result struct {
		testCase
		LocalMake  string
		LocalModel string
		LocalYear  string
		MakeMatch  bool
		NHTSAMake  string
		NHTSAModel string
		NHTSAYear  string
		NHTSAMatch string // "match", "mismatch", "empty", "skipped"
	}

	var results []result
	passCount := 0
	failCount := 0
	localFails := []result{}

	for _, tc := range cases {
		v, _ := decoder.DecodeVIN(tc.VIN)
		r := result{
			testCase:   tc,
			LocalMake:  v.Make,
			LocalModel: v.Model,
			LocalYear:  v.ModelYear,
			MakeMatch:  strings.EqualFold(v.Make, tc.ExpectedMake),
			NHTSAMatch: "skipped",
		}
		if r.MakeMatch {
			passCount++
		} else {
			failCount++
			localFails = append(localFails, r)
		}
		results = append(results, r)
	}

	fmt.Printf("\nLocal Decode Results:\n")
	fmt.Printf("  PASS: %d / %d\n", passCount, len(cases))
	fmt.Printf("  FAIL: %d / %d\n", failCount, len(cases))

	if failCount > 0 {
		fmt.Printf("\nLocal Mismatches (make):\n")
		seen := map[string]bool{}
		for _, f := range localFails {
			key := f.WMI
			if seen[key] {
				continue
			}
			seen[key] = true
			fmt.Printf("  WMI=%s  expected=%s  got=%s\n", f.WMI, f.ExpectedMake, f.LocalMake)
		}
	}

	// Optional NHTSA cross-check
	if useNHTSA {
		fmt.Printf("\nRunning NHTSA cross-check (this takes ~3 minutes)...\n")

		allVINs := make([]string, len(results))
		for i, r := range results {
			allVINs[i] = r.VIN
		}

		nhtsaMap, err := nhtsaBatchDecode(allVINs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "NHTSA error: %v\n", err)
		}

		nhtsaHits := 0
		nhtsaMismatches := 0
		nhtsaEmpty := 0

		for i := range results {
			vin := results[i].VIN
			if nr, ok := nhtsaMap[vin]; ok {
				results[i].NHTSAMake = nr.Make
				results[i].NHTSAModel = nr.Model
				results[i].NHTSAYear = nr.ModelYear
				if nr.Make == "" {
					results[i].NHTSAMatch = "empty"
					nhtsaEmpty++
				} else if strings.EqualFold(nr.Make, results[i].LocalMake) {
					results[i].NHTSAMatch = "match"
					nhtsaHits++
				} else {
					results[i].NHTSAMatch = "mismatch"
					nhtsaMismatches++
				}
			}
		}

		fmt.Printf("\nNHTSA Cross-Check:\n")
		fmt.Printf("  Matches:    %d\n", nhtsaHits)
		fmt.Printf("  Mismatches: %d\n", nhtsaMismatches)
		fmt.Printf("  Empty/NA:   %d\n", nhtsaEmpty)

		if nhtsaMismatches > 0 {
			fmt.Printf("\nNHTSA Mismatches:\n")
			seen := map[string]bool{}
			for _, r := range results {
				if r.NHTSAMatch == "mismatch" {
					key := r.WMI
					if seen[key] {
						continue
					}
					seen[key] = true
					fmt.Printf("  WMI=%s  local=%s  nhtsa=%s  vin=%s\n", r.WMI, r.LocalMake, r.NHTSAMake, r.VIN)
				}
			}
		}
	}

	// Write CSV report
	csvFile, err := os.Create("vin_validation_report.csv")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create CSV: %v\n", err)
		os.Exit(1)
	}
	defer csvFile.Close()

	w := csv.NewWriter(csvFile)
	w.Write([]string{"VIN", "WMI", "ExpectedMake", "LocalMake", "LocalModel", "LocalYear", "MakeMatch", "NHTSAMake", "NHTSAModel", "NHTSAYear", "NHTSAMatch"})

	for _, r := range results {
		match := "PASS"
		if !r.MakeMatch {
			match = "FAIL"
		}
		w.Write([]string{
			r.VIN, r.WMI, r.ExpectedMake, r.LocalMake, r.LocalModel, r.LocalYear,
			match, r.NHTSAMake, r.NHTSAModel, r.NHTSAYear, r.NHTSAMatch,
		})
	}
	w.Flush()
	fmt.Printf("\nCSV report: vin_validation_report.csv\n")

	// Summary by brand
	brandCounts := map[string]int{}
	for _, r := range results {
		brandCounts[r.ExpectedMake]++
	}
	fmt.Printf("\nVINs per brand:\n")
	for brand, count := range brandCounts {
		fmt.Printf("  %-20s %d\n", brand, count)
	}

	if failCount > 0 {
		os.Exit(1)
	}
}

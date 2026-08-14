package service

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"parts-engine/internal/enrich"
	"parts-engine/internal/model"
	"parts-engine/internal/nhtsa"
)

var vinRegex = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)

var wmiMake = map[string]string{
	// Hyundai (Korea, Turkey, India, Indonesia, China, South Africa, Czech, Russia)
	"KMH": "HYUNDAI", "KM8": "HYUNDAI", "5NP": "HYUNDAI", "5NM": "HYUNDAI",
	"KMJ": "HYUNDAI", "TMD": "HYUNDAI", "NLH": "HYUNDAI", "NLJ": "HYUNDAI",
	"MAL": "HYUNDAI", "TMA": "HYUNDAI", "TMC": "HYUNDAI", "KME": "HYUNDAI",
	"MF3": "HYUNDAI", "LBE": "HYUNDAI", "MB2": "HYUNDAI", "Z94": "HYUNDAI",
	"AC5": "HYUNDAI", "5NT": "HYUNDAI", "7YA": "HYUNDAI", "2HM": "HYUNDAI",
	// Kia (Korea, USA, Slovakia, India, Mexico, China)
	"KNA": "KIA", "KND": "KIA", "KNE": "KIA", "KNH": "KIA",
	"5XY": "KIA", "5XX": "KIA", "U5Y": "KIA", "U6Y": "KIA",
	"LJD": "KIA", "KNB": "KIA", "KNC": "KIA",
	"3KP": "KIA", "3KM": "KIA", "MZB": "KIA",
	// Genesis
	"KMT": "GENESIS", "KMU": "GENESIS",
	// Toyota (Japan, USA, Canada, Thailand, Turkey, Indonesia, India, China, UK, France, S.Africa)
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
	// Lexus (Japan, USA, Canada)
	"JTJ": "LEXUS", "JTH": "LEXUS", "JT6": "LEXUS", "JT8": "LEXUS",
	"2T2": "LEXUS", "58A": "LEXUS",
	// Honda (Japan, USA, Canada, UK, Thailand, Indonesia, India, China, Turkey, Brazil, Mexico)
	"JHM": "HONDA", "JHL": "HONDA", "SHH": "HONDA", "93H": "HONDA",
	"1HG": "HONDA", "2HG": "HONDA", "2HK": "HONDA", "2HJ": "HONDA",
	"5FN": "HONDA", "5FP": "HONDA", "5J6": "HONDA", "5JR": "HONDA", "5KB": "HONDA",
	"MHR": "HONDA", "MRH": "HONDA", "MLH": "HONDA",
	"LHG": "HONDA", "LUC": "HONDA", "LVH": "HONDA",
	"19X": "HONDA", "7FA": "HONDA", "3CZ": "HONDA", "3HG": "HONDA",
	"NLA": "HONDA", "MAK": "HONDA", "PAD": "HONDA", "PMH": "HONDA",
	"NFB": "HONDA", "RLH": "HONDA", "8C3": "HONDA",
	// Acura (USA, Canada, Mexico)
	"19U": "ACURA", "JH4": "ACURA", "19V": "ACURA",
	"5J8": "ACURA", "5FR": "ACURA", "2HH": "ACURA", "2HN": "ACURA", "3HD": "ACURA",
	// Nissan (Japan, USA, Mexico, Thailand, China, Russia)
	"JN1": "NISSAN", "JN3": "NISSAN", "JN6": "NISSAN", "JN8": "NISSAN",
	"1N4": "NISSAN", "1N6": "NISSAN", "3N1": "NISSAN", "3N6": "NISSAN", "3N8": "NISSAN",
	"5N1": "NISSAN", "MNT": "NISSAN", "LGB": "NISSAN", "LJN": "NISSAN",
	"VSK": "NISSAN", "Z8N": "NISSAN", "MDH": "NISSAN",
	// Infiniti
	"JNK": "INFINITI", "5N3": "INFINITI", "JNR": "INFINITI", "SJK": "INFINITI", "3PC": "INFINITI",
	// Mazda (Japan, Mexico, USA, Europe, Thailand)
	"JM1": "MAZDA", "JM3": "MAZDA", "JM5": "MAZDA", "JM6": "MAZDA", "JM7": "MAZDA",
	"JMZ": "MAZDA", "JM0": "MAZDA", "JM2": "MAZDA", "JM4": "MAZDA",
	"3MZ": "MAZDA", "3MD": "MAZDA", "3MJ": "MAZDA", "3MV": "MAZDA", "7MM": "MAZDA",
	// Subaru (Japan, USA)
	"JF1": "SUBARU", "JF2": "SUBARU", "JF3": "SUBARU", "4S3": "SUBARU", "4S4": "SUBARU",
	// Mitsubishi (Japan, USA, Thailand, Indonesia)
	"JA3": "MITSUBISHI", "JA4": "MITSUBISHI", "JA7": "MITSUBISHI",
	"JMB": "MITSUBISHI", "JMY": "MITSUBISHI", "JMA": "MITSUBISHI", "JMF": "MITSUBISHI",
	"4A3": "MITSUBISHI", "4A4": "MITSUBISHI", "4MB": "MITSUBISHI",
	"MMA": "MITSUBISHI", "MMB": "MITSUBISHI", "MMC": "MITSUBISHI", "MMT": "MITSUBISHI",
	"MMD": "MITSUBISHI", "MME": "MITSUBISHI", "ML3": "MITSUBISHI", "MK2": "MITSUBISHI",
	// Suzuki (Japan, Hungary, Thailand, Indonesia, India)
	"JS1": "SUZUKI", "JS2": "SUZUKI", "JS3": "SUZUKI", "JSA": "SUZUKI", "JS4": "SUZUKI",
	"TSM": "SUZUKI", "MMS": "SUZUKI", "MHD": "SUZUKI", "MHY": "SUZUKI", "MBH": "SUZUKI",
	// Isuzu (Japan, Thailand)
	"JAA": "ISUZU", "JAL": "ISUZU", "JAB": "ISUZU", "JAC": "ISUZU",
	"MP1": "ISUZU", "MPA": "ISUZU",
	// Daihatsu (Japan, Indonesia)
	"JDA": "DAIHATSU",
	// Ford (USA, Canada, Mexico, Germany, Spain, Turkey, Thailand, India)
	"1FA": "FORD", "1FB": "FORD", "1FC": "FORD", "1FD": "FORD",
	"1FM": "FORD", "1FT": "FORD", "1FV": "FORD",
	"2FA": "FORD", "2FM": "FORD", "2FT": "FORD",
	"3FA": "FORD", "3FM": "FORD", "3FT": "FORD",
	"MAJ": "FORD", "MNB": "FORD", "WF0": "FORD", "VS6": "FORD",
	"NM0": "FORD", "MPB": "FORD",
	// Lincoln (USA, Canada)
	"1LN": "LINCOLN", "2LN": "LINCOLN", "3LN": "LINCOLN", "5LM": "LINCOLN", "2LM": "LINCOLN",
	// Chevrolet (USA, Canada, Mexico)
	"1G1": "CHEVROLET", "1GC": "CHEVROLET", "1GN": "CHEVROLET",
	"2G1": "CHEVROLET", "2GC": "CHEVROLET", "2GN": "CHEVROLET",
	"3G1": "CHEVROLET", "3GC": "CHEVROLET", "3GN": "CHEVROLET",
	"1GB": "CHEVROLET", "1GA": "CHEVROLET",
	// GMC (USA, Canada, Mexico)
	"1GK": "GMC", "1GT": "GMC", "2GK": "GMC", "2GT": "GMC", "3GK": "GMC", "3GT": "GMC",
	// Cadillac (USA, Mexico)
	"1GY": "CADILLAC", "1G6": "CADILLAC", "3GY": "CADILLAC",
	// Buick (USA)
	"1G4": "BUICK", "2G4": "BUICK", "5GA": "BUICK",
	// Chrysler
	"1C3": "CHRYSLER", "2C3": "CHRYSLER", "2C4": "CHRYSLER", "3C4": "CHRYSLER",
	// RAM
	"1C6": "RAM", "3C6": "RAM",
	// Dodge
	"1B3": "DODGE", "1B7": "DODGE", "2B3": "DODGE", "2B5": "DODGE",
	"3B4": "DODGE", "1D7": "DODGE", "3D7": "DODGE",
	"1D3": "DODGE", "1D4": "DODGE",
	// Jeep (USA, Italy)
	"1J4": "JEEP", "1J8": "JEEP", "1C4": "JEEP", "ZAC": "JEEP",
	// Tesla (USA, China, Germany)
	"5YJ": "TESLA", "7SA": "TESLA", "LRW": "TESLA", "XP7": "TESLA", "7G2": "TESLA",
	// Rivian
	"7FC": "RIVIAN", "7PD": "RIVIAN",
	// Lucid
	"50E": "LUCID", "7UU": "LUCID",
	// BMW (Germany, USA, Mexico, China)
	"WBA": "BMW", "WBS": "BMW", "WBY": "BMW", "WBX": "BMW",
	"5UX": "BMW", "5UM": "BMW", "5YM": "BMW", "WB5": "BMW",
	"4US": "BMW", "3MW": "BMW", "3MF": "BMW", "LBV": "BMW",
	// MINI (UK)
	"WMW": "MINI", "WMZ": "MINI",
	// Mercedes-Benz (Germany, USA, Spain, Turkey)
	"WDB": "MERCEDES-BENZ", "WDC": "MERCEDES-BENZ", "WDD": "MERCEDES-BENZ",
	"WDF": "MERCEDES-BENZ", "WMX": "MERCEDES-BENZ",
	"W1K": "MERCEDES-BENZ", "W1N": "MERCEDES-BENZ", "W1V": "MERCEDES-BENZ",
	"4JG": "MERCEDES-BENZ", "VSA": "MERCEDES-BENZ", "55S": "MERCEDES-BENZ",
	"NLE": "MERCEDES-BENZ", "NMB": "MERCEDES-BENZ", "MBR": "MERCEDES-BENZ",
	// Smart
	"WME": "SMART", "W1A": "SMART",
	// Audi (Germany)
	"WAU": "AUDI", "WA1": "AUDI", "WAP": "AUDI", "WUA": "AUDI", "WU1": "AUDI",
	// Volkswagen (Germany, Mexico, USA, Spain, China)
	"WVW": "VOLKSWAGEN", "WVG": "VOLKSWAGEN", "WV2": "VOLKSWAGEN", "WV1": "VOLKSWAGEN",
	"3VW": "VOLKSWAGEN", "3VV": "VOLKSWAGEN", "1VW": "VOLKSWAGEN",
	"LFV": "VOLKSWAGEN", "LSV": "VOLKSWAGEN",
	// Porsche (Germany)
	"WP0": "PORSCHE", "WP1": "PORSCHE",
	// Volvo (Sweden, China, USA)
	"YV1": "VOLVO", "YV4": "VOLVO", "YV2": "VOLVO", "YV3": "VOLVO",
	"7JR": "VOLVO", "7JD": "VOLVO", "LYV": "VOLVO",
	// Polestar (Sweden, China)
	"LPS": "POLESTAR", "YSM": "POLESTAR", "YSR": "POLESTAR", "7SY": "POLESTAR",
	// Saab
	"YS3": "SAAB",
	// Fiat (Italy)
	"ZFA": "FIAT", "ZFB": "FIAT", "ZFC": "FIAT",
	// Alfa Romeo (Italy)
	"ZAR": "ALFA ROMEO", "ZAS": "ALFA ROMEO",
	// Ferrari (Italy)
	"ZFF": "FERRARI",
	// Maserati (Italy)
	"ZAM": "MASERATI", "ZN6": "MASERATI",
	// Lamborghini (Italy)
	"ZHW": "LAMBORGHINI", "ZPB": "LAMBORGHINI",
	// Jaguar (UK)
	"SAJ": "JAGUAR", "SAD": "JAGUAR",
	// Land Rover (UK)
	"SAL": "LAND ROVER",
	// Aston Martin (UK)
	"SCF": "ASTON MARTIN", "SD7": "ASTON MARTIN",
	// Bentley (UK)
	"SCB": "BENTLEY", "SJA": "BENTLEY",
	// Rolls-Royce (UK)
	"SCA": "ROLLS-ROYCE", "SLA": "ROLLS-ROYCE",
	// McLaren (UK)
	"SBM": "MCLAREN",
	// Lotus (UK)
	"SCC": "LOTUS",
	// INEOS (UK)
	"SC6": "INEOS",
	// Peugeot (France)
	"VF3": "PEUGEOT", "VR3": "PEUGEOT",
	// Citroen (France)
	"VF7": "CITROEN", "VR7": "CITROEN",
	// DS (France)
	"VR1": "DS",
	// Renault (France, Romania)
	"VF1": "RENAULT", "VF2": "RENAULT", "VF6": "RENAULT", "UU1": "RENAULT",
	// Opel/Vauxhall (Germany)
	"W0L": "OPEL", "W0V": "OPEL",
	// SEAT/Cupra (Spain)
	"VSS": "SEAT",
	// Skoda (Czech Republic)
	"TMB": "SKODA",
	// BYD (China)
	"LGX": "BYD", "LC0": "BYD", "LPE": "BYD",
	// Geely / Lynk & Co / Zeekr (China)
	"L6T": "GEELY", "LB3": "GEELY",
	// NIO (China)
	"LJ1": "NIO",
	// XPeng (China)
	"L1N": "XPENG",
	// Li Auto (China)
	"LW4": "LI AUTO",
	// Great Wall / Haval (China)
	"LGW": "GREAT WALL",
	// Chery (China)
	"LNN": "CHERY",
	// VinFast (Vietnam)
	"RLL": "VINFAST", "RLN": "VINFAST",
	// Indian brands
	"MA1": "MAHINDRA", "MAB": "MAHINDRA",
	"MAT": "TATA",
	"MA3": "MARUTI SUZUKI",
}

var modelLookup = map[string]map[byte]string{
	"HYUNDAI": {
		'A': "Accent", 'B': "Santa Fe", 'C': "Sonata", 'D': "Elantra",
		'E': "Equus", 'F': "Azera", 'G': "Genesis", 'H': "Veloster",
		'J': "Tucson", 'K': "Kona", 'L': "Palisade", 'M': "Santa Cruz",
		'N': "Nexo", 'P': "Genesis Coupe", 'R': "Accent", 'S': "Sonata",
		'T': "Tiburon", 'U': "Tucson", 'V': "Venue", 'W': "Santa Fe",
		'X': "Veracruz", 'Y': "Elantra", 'Z': "IONIQ",
	},
	"KIA": {
		'A': "Forte", 'B': "Soul", 'C': "Optima", 'D': "Sportage",
		'E': "Sorento", 'F': "Carnival", 'G': "K5", 'H': "Cadenza",
		'J': "Stinger", 'K': "Niro", 'L': "Telluride", 'M': "Seltos",
		'N': "EV6", 'P': "Sportage", 'R': "Rio", 'S': "Sedona",
		'T': "Optima", 'U': "Sorento", 'V': "Amanti", 'W': "Spectra",
		'X': "Forte", 'Y': "K8",
	},
}

var wmiModelOverride = map[string]string{
	"2T1": "Corolla", "4T1": "Camry", "4T3": "Sequoia", "4T4": "Avalon",
	"5TD": "Sienna", "5TF": "Tundra", "5TB": "Tundra/Tacoma",
	"MR0": "Hilux/Fortuner", "MR2": "Yaris/Corolla", "MHF": "Avanza/Yaris",
	"1HG": "Civic/Accord", "5FN": "Odyssey", "5J6": "CR-V", "5JR": "Passport",
	"5FP": "Ridgeline/Passport", "5KB": "Civic/Accord", "JHL": "CR-V/Pilot/HR-V",
	"1N4": "Altima/Sentra", "1N6": "Titan/Frontier", "5N1": "Pathfinder/Murano",
	"JN8": "Rogue/X-Trail", "3N1": "Versa/Sentra",
	"JF1": "Impreza/Legacy", "JF2": "Forester/Outback",
	"4S3": "Impreza/WRX", "4S4": "Forester/Outback",
	"JM1": "Mazda3/Mazda6", "JM3": "CX-5/CX-9", "JM5": "CX-9/CX-50",
	"5YJ": "Model S/Model 3", "7SA": "Model X/Model Y",
	"LRW": "Model 3/Model Y", "XP7": "Model Y",
	"5UX": "X5/X3", "5UM": "M3/M4/M5",
	"4JG": "GLE/GLS", "W1N": "GLC/GLE/GLS",
	"JTJ": "RX/NX/LX", "JTH": "ES/LS", "2T2": "RX/NX",
	"1FM": "Explorer/Expedition", "1FT": "F-150/Super Duty", "1FA": "Mustang/Fusion",
	"3FM": "Explorer/Escape", "3FT": "F-150/Maverick",
	"1GC": "Silverado", "1GN": "Tahoe/Suburban", "1G1": "Malibu/Camaro",
	"1GK": "Acadia/Yukon", "1GT": "Sierra",
	"1J4": "Wrangler/Cherokee", "1C4": "Grand Cherokee/Pacifica",
	"SAL": "Range Rover/Defender",
	"WP0": "911/Cayman", "WP1": "Cayenne/Macan",
	"5LM": "Navigator/Aviator", "2LM": "Navigator/Aviator",
	"1GY": "Escalade", "3GY": "Escalade",
	"7FC": "R1T", "7PD": "R1S",
	"WVG": "Tiguan/Atlas", "3VV": "Taos/Atlas",
	"LGX": "Han/Seal/Dolphin",
}

var yearCodes = map[byte]int{
	'A': 2010, 'B': 2011, 'C': 2012, 'D': 2013, 'E': 2014,
	'F': 2015, 'G': 2016, 'H': 2017, 'J': 2018, 'K': 2019,
	'L': 2020, 'M': 2021, 'N': 2022, 'P': 2023, 'R': 2024,
	'S': 2025, 'T': 2026, 'V': 2027, 'W': 2028, 'X': 2029,
	'Y': 2030,
	'1': 2001, '2': 2002, '3': 2003, '4': 2004, '5': 2005,
	'6': 2006, '7': 2007, '8': 2008, '9': 2009,
}

var countryCode = map[byte]string{
	'1': "UNITED STATES", '4': "UNITED STATES", '5': "UNITED STATES",
	'2': "CANADA", '3': "MEXICO",
	'6': "AUSTRALIA", '7': "NEW ZEALAND",
	'8': "ARGENTINA", '9': "BRAZIL",
	'A': "SOUTH AFRICA",
	'J': "JAPAN", 'K': "SOUTH KOREA",
	'L': "CHINA", 'M': "THAILAND",
	'N': "TURKEY",
	'P': "PHILIPPINES",
	'R': "TAIWAN",
	'S': "UNITED KINGDOM",
	'T': "CZECH REPUBLIC",
	'U': "SLOVAKIA",
	'V': "FRANCE",
	'W': "GERMANY",
	'X': "RUSSIA",
	'Y': "SWEDEN",
	'Z': "ITALY",
}

type VINDecoder struct {
	nhtsa    *nhtsa.Decoder
	nhtsaAPI *nhtsa.APIClient
	enricher *enrich.Enricher
}

func NewVINDecoder(_ string, nhtsaDecoder *nhtsa.Decoder, enricher *enrich.Enricher) *VINDecoder {
	return &VINDecoder{
		nhtsa:    nhtsaDecoder,
		nhtsaAPI: nhtsa.NewAPIClient(),
		enricher: enricher,
	}
}

func (d *VINDecoder) ValidateVIN(vin string) error {
	vin = strings.ToUpper(strings.TrimSpace(vin))
	if len(vin) != 17 {
		return fmt.Errorf("VIN must be 17 characters, got %d", len(vin))
	}
	if !vinRegex.MatchString(vin) {
		return fmt.Errorf("VIN contains invalid characters (I, O, Q not allowed)")
	}
	return nil
}

func (d *VINDecoder) DecodeVIN(vin string) (*model.NHTSAVehicle, error) {
	vin = strings.ToUpper(strings.TrimSpace(vin))

	// Try NHTSA SQLite database first (exact model identification)
	if d.nhtsa != nil {
		nr, err := d.nhtsa.Decode(vin)
		if err != nil {
			log.Printf("nhtsa decode warning: %v", err)
		}
		if nr != nil && nr.Model != "" {
			// Use our curated WMI→Make mapping (more reliable for brand assignment)
			// but take everything else from NHTSA (exact model, body, engine, etc.)
			makeName := strings.ToUpper(nr.Make)
			wmi3 := vin[0:3]
			if m, ok := wmiMake[wmi3]; ok {
				makeName = m
			}

			v := &model.NHTSAVehicle{
				Make:         makeName,
				Model:        nr.Model,
				ModelYear:    strconv.Itoa(nr.ModelYear),
				BodyClass:    nr.BodyClass,
				DriveType:    nr.DriveType,
				FuelType:     nr.FuelType,
				EngineCC:     nr.EngineCC,
				EngineCyl:    nr.EngineCyl,
				PlantCountry: nr.PlantCountry,
			}
			d.applyEnrichment(v)
			return v, nil
		}
	}

	// Fallback 1: NHTSA online API (for VINs not in the local DB)
	if d.nhtsaAPI != nil {
		nr, err := d.nhtsaAPI.Decode(vin)
		if err != nil {
			log.Printf("nhtsa api warning: %v", err)
		}
		if nr != nil && nr.Model != "" {
			makeName := strings.ToUpper(nr.Make)
			wmi3 := vin[0:3]
			if m, ok := wmiMake[wmi3]; ok {
				makeName = m
			}
			v := &model.NHTSAVehicle{
				Make:         makeName,
				Model:        nr.Model,
				ModelYear:    strconv.Itoa(nr.ModelYear),
				BodyClass:    nr.BodyClass,
				DriveType:    nr.DriveType,
				FuelType:     nr.FuelType,
				EngineCC:     nr.EngineCC,
				EngineCyl:    nr.EngineCyl,
				PlantCountry: nr.PlantCountry,
			}
			d.applyEnrichment(v)
			return v, nil
		}
	}

	// Fallback 2: WMI-based local decoder
	v, err := d.decodeWMI(vin)
	if err == nil {
		d.applyEnrichment(v)
	}
	return v, err
}

// applyEnrichment queries all enrichment sources and merges data into the result.
func (d *VINDecoder) applyEnrichment(v *model.NHTSAVehicle) {
	if d.enricher == nil {
		return
	}
	year, _ := strconv.Atoi(v.ModelYear)
	if year == 0 {
		return
	}
	er := d.enricher.Enrich(v.Make, v.Model, year)
	if er == nil {
		return
	}
	if er.Transmission != "" && v.Transmission == "" {
		v.Transmission = er.Transmission
	}
	if er.VehicleClass != "" && v.VehicleClass == "" {
		v.VehicleClass = er.VehicleClass
	}
	if er.CityMPG > 0 && v.CityMPG == 0 {
		v.CityMPG = er.CityMPG
	}
	if er.HighwayMPG > 0 && v.HighwayMPG == 0 {
		v.HighwayMPG = er.HighwayMPG
	}
	if er.CombinedMPG > 0 && v.CombinedMPG == 0 {
		v.CombinedMPG = er.CombinedMPG
	}
	if er.CO2Gpm > 0 && v.CO2Gpm == 0 {
		v.CO2Gpm = er.CO2Gpm
	}
	if er.EngineDescr != "" && v.EngineDescr == "" {
		v.EngineDescr = er.EngineDescr
	}
	if er.VehicleType != "" && v.VehicleType == "" {
		v.VehicleType = er.VehicleType
	}
	if er.Trim != "" && v.Trim == "" {
		v.Trim = er.Trim
	}
	v.DataSources = er.Sources
}

func (d *VINDecoder) decodeWMI(vin string) (*model.NHTSAVehicle, error) {
	wmi3 := vin[0:3]
	pos4 := vin[3]

	makeName := ""
	if m, ok := wmiMake[wmi3]; ok {
		makeName = m
	} else {
		makeName = "UNKNOWN (" + wmi3 + ")"
	}

	year := 0
	if y, ok := yearCodes[vin[9]]; ok {
		year = y
	}

	modelName := ""
	if models, ok := modelLookup[makeName]; ok {
		if m, ok := models[pos4]; ok {
			modelName = m
		}
	}
	if modelName == "" {
		if m, ok := wmiModelOverride[wmi3]; ok {
			modelName = m
		}
	}
	if modelName == "" {
		modelName = "Unknown"
	}

	plantCountry := ""
	if c, ok := countryCode[vin[0]]; ok {
		plantCountry = c
	}

	return &model.NHTSAVehicle{
		Make:         makeName,
		Model:        modelName,
		ModelYear:    strconv.Itoa(year),
		PlantCountry: plantCountry,
	}, nil
}

func ParseModelYear(s string) int {
	y, _ := strconv.Atoi(s)
	return y
}

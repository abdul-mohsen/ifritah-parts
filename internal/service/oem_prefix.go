package service

// OEMPrefix decodes Hyundai/Kia OEM part number prefixes into part categories.
// Hyundai/Kia OEM numbers follow the pattern XXXXX-XXXXX where the first 2-3 digits
// indicate the vehicle subsystem.

// OEMCategory contains decoded category info from an OEM prefix.
type OEMCategory struct {
	System   string // e.g. "Engine", "Brakes", "Cooling"
	Category string // e.g. "Oil Filter", "Brake Pad Set"
	Prefix   string // The matched prefix
}

// prefixMap maps Hyundai/Kia OEM number prefixes → subsystem/category.
// Source: Hyundai/Kia parts catalog structure (EPC).
var prefixMap = map[string]OEMCategory{
	// ═══ Engine ═══
	"21":  {System: "Engine", Category: "Engine Block & Internals"},
	"211": {System: "Engine", Category: "Cylinder Block"},
	"213": {System: "Engine", Category: "Crankshaft & Bearings"},
	"22":  {System: "Engine", Category: "Engine Mounting"},
	"23":  {System: "Engine", Category: "Cylinder Head & Valvetrain"},
	"231": {System: "Engine", Category: "Cylinder Head"},
	"233": {System: "Engine", Category: "Camshaft & Timing"},
	"24":  {System: "Engine", Category: "Intake & Exhaust Manifold"},
	"25":  {System: "Engine", Category: "Fuel System"},
	"251": {System: "Engine", Category: "Fuel Pump"},
	"253": {System: "Engine", Category: "Fuel Injector"},
	"26":  {System: "Engine", Category: "Oil System / Filters"},
	"263": {System: "Engine", Category: "Oil Filter"},
	"27":  {System: "Engine", Category: "EGR & Emissions"},
	"283": {System: "Engine", Category: "Intake Manifold"},
	"284": {System: "Engine", Category: "Water Pump"},
	"29":  {System: "Engine", Category: "Turbo / Supercharger"},

	// ═══ Cooling ═══
	"28":  {System: "Cooling", Category: "Cooling System"},
	"281": {System: "Cooling", Category: "Radiator"},
	"282": {System: "Cooling", Category: "Thermostat & Housing"},
	"285": {System: "Cooling", Category: "Coolant Hose"},

	// ═══ Exhaust ═══
	"286": {System: "Exhaust", Category: "Exhaust System"},
	"287": {System: "Exhaust", Category: "Exhaust Manifold"},
	"289": {System: "Exhaust", Category: "Catalytic Converter"},

	// ═══ Drivetrain ═══
	"30":  {System: "Drivetrain", Category: "Propeller Shaft"},
	"31":  {System: "Drivetrain", Category: "Front Differential"},
	"32":  {System: "Drivetrain", Category: "Front Drive Shaft"},
	"33":  {System: "Drivetrain", Category: "Rear Axle & Differential"},
	"34":  {System: "Drivetrain", Category: "Rear Drive Shaft"},
	"35":  {System: "Drivetrain", Category: "Drive Shaft / CV Joint"},
	"36":  {System: "Electrical", Category: "Starter & Charging"},
	"361": {System: "Electrical", Category: "Starter Motor"},
	"373": {System: "Electrical", Category: "Alternator"},
	"37":  {System: "Electrical", Category: "Ignition System"},
	"38":  {System: "Engine", Category: "Engine Control Unit (ECU)"},
	"39":  {System: "Electrical", Category: "Sensors & Control"},
	"392": {System: "Electrical", Category: "Oxygen Sensor"},

	// ═══ Clutch & Transmission ═══
	"41": {System: "Drivetrain", Category: "Clutch"},
	"43": {System: "Transmission", Category: "Automatic Transmission"},
	"44": {System: "Transmission", Category: "Transmission Control"},
	"45": {System: "Transmission", Category: "Manual Transmission"},
	"46": {System: "Transmission", Category: "Auto Transmission Control"},
	"47": {System: "Drivetrain", Category: "Transfer Case"},
	"48": {System: "Drivetrain", Category: "Transaxle"},
	"49": {System: "Drivetrain", Category: "Transfer Case / 4WD"},

	// ═══ Suspension & Steering ═══
	"51":  {System: "Suspension", Category: "Front Axle"},
	"52":  {System: "Suspension", Category: "Rear Axle"},
	"529": {System: "Suspension", Category: "Wheels & Tires"},
	"53":  {System: "Suspension", Category: "Power Steering"},
	"54":  {System: "Suspension", Category: "Front Suspension"},
	"546": {System: "Suspension", Category: "Shock Absorber (Front)"},
	"55":  {System: "Suspension", Category: "Rear Suspension"},
	"553": {System: "Suspension", Category: "Shock Absorber (Rear)"},
	"56":  {System: "Suspension", Category: "Steering Column & Gear"},
	"563": {System: "Suspension", Category: "Tie Rod"},
	"57":  {System: "Suspension", Category: "Wheel & Hub"},

	// ═══ Brakes ═══
	"58":  {System: "Brakes", Category: "Brakes"},
	"581": {System: "Brakes", Category: "Front Brake Pad / Disc"},
	"582": {System: "Brakes", Category: "Front Brake Caliper"},
	"583": {System: "Brakes", Category: "Rear Brake / Drum"},
	"584": {System: "Brakes", Category: "Rear Brake Caliper"},
	"585": {System: "Brakes", Category: "Parking Brake"},
	"586": {System: "Brakes", Category: "Brake Master Cylinder"},
	"59":  {System: "Brakes", Category: "ABS / ESC"},
	"589": {System: "Brakes", Category: "Brake Fluid Reservoir"},

	// ═══ Frame & Body Structure ═══
	"60": {System: "Body", Category: "Frame & Cross Members"},
	"61": {System: "Body", Category: "Sub-Frame"},
	"62": {System: "Body", Category: "Front Structure"},
	"63": {System: "Body", Category: "Rear Structure"},
	"64": {System: "Body", Category: "Front Body / Hood"},
	"65": {System: "Body", Category: "Fender & Side Body"},
	"66": {System: "Body", Category: "Rear Body / Trunk"},
	"67": {System: "Body", Category: "Floor & Underbody"},
	"68": {System: "Body", Category: "Roof Panel"},
	"69": {System: "Body", Category: "Quarter Panel"},
	"70": {System: "Body", Category: "Tailgate / Liftgate"},
	"71": {System: "Body", Category: "Bumper"},

	// ═══ Doors & Glass ═══
	"72": {System: "Body", Category: "Front Door"},
	"73": {System: "Body", Category: "Rear Door"},
	"74": {System: "Body", Category: "Back Door / Hatch"},
	"75": {System: "Body", Category: "Door Lock & Handle"},
	"76": {System: "Body", Category: "Window Regulator"},
	"81": {System: "Body", Category: "Weatherstrip & Seal"},
	"82": {System: "Body", Category: "Glass / Windshield"},
	"83": {System: "Body", Category: "Sunroof"},

	// ═══ Interior ═══
	"84": {System: "Interior", Category: "Interior Trim"},
	"85": {System: "Interior", Category: "Seats"},
	"86": {System: "Body", Category: "Mirrors"},
	"87": {System: "Body", Category: "Mouldings & Trim"},
	"88": {System: "Interior", Category: "Instrument Panel / Dashboard"},
	"89": {System: "Safety", Category: "Air Bag System"},

	// ═══ Electrical ═══
	"91":  {System: "Electrical", Category: "Wiring Harness"},
	"92":  {System: "Electrical", Category: "Lighting - Headlights"},
	"921": {System: "Electrical", Category: "Headlight Assembly"},
	"922": {System: "Electrical", Category: "Fog Light"},
	"923": {System: "Electrical", Category: "Turn Signal"},
	"924": {System: "Electrical", Category: "Tail Light Assembly"},
	"93":  {System: "Electrical", Category: "Lighting - Interior"},
	"94":  {System: "Electrical", Category: "Audio & Display"},
	"95":  {System: "Electrical", Category: "Sensors & Modules"},
	"96":  {System: "Electrical", Category: "Battery & Charging"},
	"961": {System: "Electrical", Category: "Battery"},

	// ═══ HVAC ═══
	"97":  {System: "HVAC", Category: "Air Conditioning & Heating"},
	"971": {System: "HVAC", Category: "Compressor A/C"},
	"972": {System: "HVAC", Category: "Condenser"},
	"973": {System: "HVAC", Category: "Evaporator"},
	"976": {System: "HVAC", Category: "Heater Core"},
	"977": {System: "HVAC", Category: "A/C Hose & Pipe"},

	// ═══ Wipers & Maintenance ═══
	"98":  {System: "Maintenance", Category: "Wiper System"},
	"983": {System: "Maintenance", Category: "Wiper Blades"},
	"984": {System: "Maintenance", Category: "Washer System"},
}

// DecodeOEMPrefix attempts to classify a Hyundai/Kia OEM part number by its prefix.
// Tries longest prefix match first (3 digits, then 2 digits).
// Returns nil if no match.
func DecodeOEMPrefix(oemNumber string) *OEMCategory {
	// Strip formatting
	clean := ""
	for _, c := range oemNumber {
		if c >= '0' && c <= '9' {
			clean += string(c)
		}
		if len(clean) >= 5 {
			break
		}
	}
	if len(clean) < 2 {
		return nil
	}

	// Try 3-digit prefix first (more specific)
	if len(clean) >= 3 {
		if cat, ok := prefixMap[clean[:3]]; ok {
			cat.Prefix = clean[:3]
			return &cat
		}
	}

	// Try 2-digit prefix
	if cat, ok := prefixMap[clean[:2]]; ok {
		cat.Prefix = clean[:2]
		return &cat
	}

	return nil
}

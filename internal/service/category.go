package service

// FitmentDriver specifies which vehicle attribute(s) determine part compatibility.
type FitmentDriver int

const (
	// FitEngine means the part depends on engine displacement, fuel type, cylinders.
	FitEngine FitmentDriver = iota
	// FitBody means the part depends on body style / dimensions.
	FitBody
	// FitDrivetrain means the part depends on drive type (FWD/AWD/RWD).
	FitDrivetrain
	// FitBrake means brakes may vary by trim/sport package; moderate CC correlation.
	FitBrake
	// FitUniversal means the part fits by physical dimensions, not engine/body.
	FitUniversal
)

// CategoryRule defines how to filter parts for a specific genericArticleDescription.
type CategoryRule struct {
	Driver   FitmentDriver
	CCMargin int  // for FitEngine: acceptable ±cc difference (0 = use default 500)
	Strict   bool // if true, reject cross-refs outside the margin
}

// categoryRules maps genericArticleDescription keywords to fitment strategies.
// The key is a substring matched against the part's genericArticleDescription.
var categoryRules = map[string]CategoryRule{
	// Engine-dependent: must match engine CC ±500
	"Alternator":        {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Starter":           {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Spark Plug":        {Driver: FitEngine, CCMargin: 300, Strict: true},
	"Ignition Coil":     {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Exhaust":           {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Turbocharger":      {Driver: FitEngine, CCMargin: 300, Strict: true},
	"Fuel Injector":     {Driver: FitEngine, CCMargin: 300, Strict: true},
	"Fuel Pump":         {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Water Pump":        {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Thermostat":        {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Timing Belt":       {Driver: FitEngine, CCMargin: 300, Strict: true},
	"Timing Chain":      {Driver: FitEngine, CCMargin: 300, Strict: true},
	"Cylinder Head":     {Driver: FitEngine, CCMargin: 200, Strict: true},
	"Piston":            {Driver: FitEngine, CCMargin: 200, Strict: true},
	"Camshaft":          {Driver: FitEngine, CCMargin: 200, Strict: true},
	"Crankshaft":        {Driver: FitEngine, CCMargin: 200, Strict: true},
	"Valve":             {Driver: FitEngine, CCMargin: 300, Strict: true},
	"Engine Mount":      {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Radiator":          {Driver: FitEngine, CCMargin: 800, Strict: false},
	"Radiator Fan":      {Driver: FitEngine, CCMargin: 800, Strict: false},
	"Cooling":           {Driver: FitEngine, CCMargin: 800, Strict: false},
	"Coolant":           {Driver: FitEngine, CCMargin: 800, Strict: false},
	"Expansion Tank":    {Driver: FitEngine, CCMargin: 800, Strict: false},
	"Serpentine Belt":   {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Drive Belt":        {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Intake Manifold":   {Driver: FitEngine, CCMargin: 300, Strict: true},
	"Exhaust Manifold":  {Driver: FitEngine, CCMargin: 300, Strict: true},
	"Catalytic Convert": {Driver: FitEngine, CCMargin: 500, Strict: true},
	"EGR":               {Driver: FitEngine, CCMargin: 500, Strict: true},
	"Oxygen Sensor":     {Driver: FitEngine, CCMargin: 800, Strict: false},
	"Lambda Sensor":     {Driver: FitEngine, CCMargin: 800, Strict: false},
	"Injector":          {Driver: FitEngine, CCMargin: 300, Strict: true},
	"Glow Plug":         {Driver: FitEngine, CCMargin: 300, Strict: true},

	// Body-dependent: match by body style / model generation
	"Wiper Motor": {Driver: FitBody},
	"Wiper Blade": {Driver: FitBody},
	"Washer Pump": {Driver: FitBody},
	"Wiper":       {Driver: FitBody},
	"Door":        {Driver: FitBody},
	"Window":      {Driver: FitBody},
	"Mirror":      {Driver: FitBody},
	"Bumper":      {Driver: FitBody},
	"Headlight":   {Driver: FitBody},
	"Tail Light":  {Driver: FitBody},
	"Rear Light":  {Driver: FitBody},
	"Indicator":   {Driver: FitBody},
	"Fog Light":   {Driver: FitBody},
	"Grille":      {Driver: FitBody},
	"Fender":      {Driver: FitBody},
	"Bonnet":      {Driver: FitBody},
	"Hood":        {Driver: FitBody},
	"Trunk":       {Driver: FitBody},
	"Tailgate":    {Driver: FitBody},

	// Drivetrain-dependent: match by FWD/AWD/RWD
	"CV Joint":      {Driver: FitDrivetrain},
	"CV Axle":       {Driver: FitDrivetrain},
	"Drive Shaft":   {Driver: FitDrivetrain},
	"Transfer Case": {Driver: FitDrivetrain},
	"Differential":  {Driver: FitDrivetrain},
	"Propshaft":     {Driver: FitDrivetrain},
	"Axle":          {Driver: FitDrivetrain},
	"Clutch":        {Driver: FitDrivetrain},
	"Transmission":  {Driver: FitDrivetrain},

	// Brakes: may vary by trim, moderate CC correlation
	"Brake Pad":     {Driver: FitBrake, CCMargin: 1000},
	"Brake Disc":    {Driver: FitBrake, CCMargin: 1000},
	"Brake Rotor":   {Driver: FitBrake, CCMargin: 1000},
	"Brake Caliper": {Driver: FitBrake, CCMargin: 1000},
	"Brake Shoe":    {Driver: FitBrake, CCMargin: 1000},
	"Brake Drum":    {Driver: FitBrake, CCMargin: 1000},
	"Brake Hose":    {Driver: FitBrake, CCMargin: 1000},
	"Brake Master":  {Driver: FitBrake, CCMargin: 1000},

	// Universal: fit by physical dimensions, not engine
	"Oil Filter":    {Driver: FitUniversal},
	"Cabin Filter":  {Driver: FitUniversal},
	"Pollen Filter": {Driver: FitUniversal},
	"Air Filter":    {Driver: FitUniversal},
	"Fuel Filter":   {Driver: FitUniversal},
	"Wheel Bolt":    {Driver: FitUniversal},
	"Wheel Nut":     {Driver: FitUniversal},
	"Wiper Fluid":   {Driver: FitUniversal},
	"Bulb":          {Driver: FitUniversal},
	"Fuse":          {Driver: FitUniversal},
	"Gas Strut":     {Driver: FitUniversal},
}

// ClassifyCategory returns the fitment rule for a given part description.
// Falls back to FitUniversal if no rule matches.
func ClassifyCategory(description string) CategoryRule {
	// Try longest match first (more specific wins)
	bestKey := ""
	bestRule := CategoryRule{Driver: FitUniversal}
	for key, rule := range categoryRules {
		if len(key) > len(bestKey) && containsIgnoreCase(description, key) {
			bestKey = key
			bestRule = rule
		}
	}
	return bestRule
}

func containsIgnoreCase(s, substr string) bool {
	sLen := len(s)
	subLen := len(substr)
	if subLen > sLen {
		return false
	}
	for i := 0; i <= sLen-subLen; i++ {
		match := true
		for j := 0; j < subLen; j++ {
			a := s[i+j]
			b := substr[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

package model

// Vehicle represents a decoded vehicle from VIN or TecDoc lookup.
type Vehicle struct {
	LinkageTargetId int    `json:"linkageTargetId"`
	Make            string `json:"make"`
	Model           string `json:"model"`
	ModelYear       int    `json:"modelYear,omitempty"`
	Description     string `json:"description,omitempty"`
	FuelType        string `json:"fuelType,omitempty"`
	CapacityCC      int    `json:"capacityCC,omitempty"`
	HorsePower      int    `json:"horsePower,omitempty"`
	BeginYearMonth  int    `json:"beginYearMonth,omitempty"`
	EndYearMonth    int    `json:"endYearMonth,omitempty"`
}

// EngineDetail describes a resolved TecDoc motor code for the vehicle.
type EngineDetail struct {
	MotorCode  string `json:"motorCode"`
	CC         int    `json:"cc"`
	FuelType   string `json:"fuelType,omitempty"`
	Cylinders  int    `json:"cylinders,omitempty"`
	PowerHP    int    `json:"powerHP,omitempty"`
	PowerKW    int    `json:"powerKW,omitempty"`
	EngineType string `json:"engineType,omitempty"`
}

// VINDecodeResult is the response from the VIN decode endpoint.
type VINDecodeResult struct {
	VIN        string          `json:"vin"`
	Vehicle    *Vehicle        `json:"vehicle"`
	Parts      []Part          `json:"parts,omitempty"`
	CrossBrand []CrossBrandHit `json:"crossBrand,omitempty"`
	Recalls    []Recall        `json:"recalls,omitempty"`
	TotalParts int             `json:"totalParts"`
	NHTSARaw   *NHTSAVehicle   `json:"nhtsaRaw,omitempty"`
	MotorCodes []string        `json:"motorCodes,omitempty"`
	Engines    []EngineDetail  `json:"engines,omitempty"`
}

// NHTSAVehicle is the vehicle info decoded from NHTSA vPIC API.
type NHTSAVehicle struct {
	Make         string `json:"make"`
	Model        string `json:"model"`
	ModelYear    string `json:"modelYear"`
	BodyClass    string `json:"bodyClass,omitempty"`
	DriveType    string `json:"driveType,omitempty"`
	FuelType     string `json:"fuelType,omitempty"`
	EngineCC     string `json:"engineDisplacementCC,omitempty"`
	EngineCyl    string `json:"engineNumberOfCylinders,omitempty"`
	PlantCountry string `json:"plantCountry,omitempty"`
	// Enrichment fields (from EPA, OpenVehicleDB, CarQuery, etc.)
	Trim         string   `json:"trim,omitempty"`
	Transmission string   `json:"transmission,omitempty"`
	VehicleClass string   `json:"vehicleClass,omitempty"`
	CityMPG      int      `json:"cityMPG,omitempty"`
	HighwayMPG   int      `json:"highwayMPG,omitempty"`
	CombinedMPG  int      `json:"combinedMPG,omitempty"`
	CO2Gpm       float64  `json:"co2GramsPerMile,omitempty"`
	EngineDescr  string   `json:"engineDescription,omitempty"`
	VehicleType  string   `json:"vehicleType,omitempty"`
	DataSources  []string `json:"dataSources,omitempty"`
}

// CrossBrandHit suggests a sibling vehicle that shares parts.
type CrossBrandHit struct {
	SiblingMake  string `json:"siblingMake"`
	SiblingModel string `json:"siblingModel"`
	Platform     string `json:"platform"`
	SharedParts  int    `json:"sharedParts,omitempty"`
}

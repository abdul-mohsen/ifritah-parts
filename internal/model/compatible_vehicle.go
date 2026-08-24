package model

// CompatibleVehicle describes a vehicle a part fits, sourced from the
// TecDoc articlesvehicletrees table (651M rows).
//
// This type is intentionally distinct from model.Vehicle: Vehicle represents
// a resolved vehicle from a VIN or user selection; CompatibleVehicle is the
// inverse — a part-first fitment link. VehicleName is the display label
// (e.g. "TUCSON 2.0 CRDi 4WD (2015-2020)"); Make/Model/YearFrom/YearTo carry
// the structured facets.
//
// FitmentDriver mirrors the ClassifyCategory driver (e.g. "engine",
// "chassis", "electrical") so consumers can render category-appropriate
// evidence without re-classifying.
//
// Chassis and EngineSpec are populated by M3.S2.T2 from the raw
// linkagetargets.description (e.g. "(TL)" -> Chassis, "2.0 CRDi 4WD 136HP" ->
// EngineSpec). Empty when the description doesn't include them.
type CompatibleVehicle struct {
	LegacyArticleId int    `json:"legacyArticleId"`
	LinkageTargetId int    `json:"linkageTargetId"`
	VehicleName     string `json:"vehicleName"`
	Make            string `json:"make,omitempty"`
	Model           string `json:"model,omitempty"`
	Chassis         string `json:"chassis,omitempty"`
	EngineSpec      string `json:"engineSpec,omitempty"`
	YearFrom        int    `json:"yearFrom,omitempty"`
	YearTo          int    `json:"yearTo,omitempty"`
	FuelType        string `json:"fuelType,omitempty"`
	CapacityCC      int    `json:"capacityCC,omitempty"`
	HorsePower      int    `json:"horsePower,omitempty"`
	FitmentDriver   string `json:"fitmentDriver,omitempty"`
}

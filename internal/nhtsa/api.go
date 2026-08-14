package nhtsa

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIClient queries the NHTSA vPIC online API as a last-resort fallback.
type APIClient struct {
	client *http.Client
}

// NewAPIClient creates an NHTSA API client with sensible timeouts.
func NewAPIClient() *APIClient {
	return &APIClient{
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

type apiResponse struct {
	Results []apiResult `json:"Results"`
}

type apiResult struct {
	Variable string `json:"Variable"`
	Value    string `json:"Value"`
}

// Decode calls the NHTSA vPIC DecodeVinValues endpoint.
// Returns nil if the API call fails (non-fatal).
func (a *APIClient) Decode(vin string) (*DecodeResult, error) {
	u := fmt.Sprintf("https://vpic.nhtsa.dot.gov/api/vehicles/DecodeVinValues/%s?format=json",
		url.PathEscape(vin))

	resp, err := a.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("nhtsa api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nhtsa api: status %d", resp.StatusCode)
	}

	var data apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("nhtsa api: decode: %w", err)
	}

	result := &DecodeResult{}
	for _, r := range data.Results {
		v := strings.TrimSpace(r.Value)
		if v == "" || v == "Not Applicable" {
			continue
		}
		switch r.Variable {
		case "Make":
			result.Make = v
		case "Model":
			result.Model = v
		case "Model Year":
			fmt.Sscanf(v, "%d", &result.ModelYear)
		case "Body Class":
			result.BodyClass = v
		case "Drive Type":
			result.DriveType = v
		case "Fuel Type - Primary":
			result.FuelType = v
		case "Displacement (CC)":
			result.EngineCC = v
		case "Engine Number of Cylinders":
			result.EngineCyl = v
		case "Plant Country":
			result.PlantCountry = v
		case "Trim":
			result.Trim = v
		case "Series":
			result.Series = v
		case "Vehicle Type":
			result.VehicleType = v
		}
	}

	if result.Model == "" {
		return nil, nil
	}
	return result, nil
}

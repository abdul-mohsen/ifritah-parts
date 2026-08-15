package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"parts-engine/internal/model"
)

const recallMatchWarning = "NHTSA recall results are matched by make, model, and model year. They do not confirm that this exact VIN is affected or that a remedy remains open."

type RecallsClient struct {
	baseURL string
	client  *http.Client
}

func NewRecallsClient(baseURL string) *RecallsClient {
	return newRecallsClient(baseURL, &http.Client{Timeout: 8 * time.Second})
}

func newRecallsClient(baseURL string, client *http.Client) *RecallsClient {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &RecallsClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  client,
	}
}

type nhtsaRecallsResponse struct {
	Count   int `json:"Count"`
	Results []struct {
		NHTSACampaignNumber string `json:"NHTSACampaignNumber"`
		Component           string `json:"Component"`
		Summary             string `json:"Summary"`
		Consequence         string `json:"Consequence"`
		Remedy              string `json:"Remedy"`
		ReportReceivedDate  string `json:"ReportReceivedDate"`
	} `json:"results"`
}

func (c *RecallsClient) GetRecalls(makeName, modelName string, year int) ([]model.Recall, error) {
	makeName = strings.ToUpper(strings.TrimSpace(makeName))
	modelName = strings.ToUpper(strings.TrimSpace(modelName))
	if makeName == "" || modelName == "" || year < 1886 || year > time.Now().Year()+1 {
		return nil, fmt.Errorf("valid make, model, and model year are required")
	}
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("NHTSA recall client is not configured")
	}

	endpoint, err := url.Parse(c.baseURL + "/recallsByVehicle")
	if err != nil {
		return nil, fmt.Errorf("build NHTSA recall URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("make", makeName)
	query.Set("model", modelName)
	query.Set("modelYear", strconv.Itoa(year))
	endpoint.RawQuery = query.Encode()

	resp, err := c.client.Get(endpoint.String())
	if err != nil {
		return nil, fmt.Errorf("request NHTSA recalls: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NHTSA recalls returned status %d", resp.StatusCode)
	}

	var payload nhtsaRecallsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode NHTSA recalls: %w", err)
	}

	recalls := make([]model.Recall, 0, len(payload.Results))
	for _, result := range payload.Results {
		if result.NHTSACampaignNumber == "" || result.Summary == "" {
			continue
		}
		recalls = append(recalls, model.Recall{
			NHTSACampaignNumber: result.NHTSACampaignNumber,
			Component:           result.Component,
			Summary:             result.Summary,
			Consequence:         result.Consequence,
			Remedy:              result.Remedy,
			ReportDate:          result.ReportReceivedDate,
			SourceLabel:         "NHTSA vehicle recall API",
			SourceURL:           endpoint.String(),
			Warning:             recallMatchWarning,
		})
	}
	return recalls, nil
}

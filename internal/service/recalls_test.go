package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecallsClientMapsNHTSAVehicleResultsWithScopeWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/recallsByVehicle" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("make") != "HYUNDAI" || r.URL.Query().Get("model") != "TUCSON" || r.URL.Query().Get("modelYear") != "2016" {
			t.Fatalf("unexpected query %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Count": 1,
			"results": [{
				"NHTSACampaignNumber": "20V543000",
				"Component": "SERVICE BRAKES",
				"Summary": "HECU could cause an engine compartment fire.",
				"Consequence": "Fire risk.",
				"Remedy": "Replace the fuse.",
				"ReportReceivedDate": "04/09/2020"
			}]
		}`))
	}))
	defer server.Close()

	client := newRecallsClient(server.URL, &http.Client{Timeout: time.Second})
	recalls, err := client.GetRecalls("hyundai", "tucson", 2016)
	if err != nil {
		t.Fatalf("GetRecalls returned error: %v", err)
	}
	if len(recalls) != 1 {
		t.Fatalf("expected one recall, got %d", len(recalls))
	}
	if recalls[0].NHTSACampaignNumber != "20V543000" || recalls[0].SourceLabel != "NHTSA vehicle recall API" {
		t.Fatalf("unexpected recall: %+v", recalls[0])
	}
	if !strings.Contains(recalls[0].Warning, "do not confirm") {
		t.Fatalf("missing vehicle-level scope warning: %q", recalls[0].Warning)
	}
}

func TestRecallsClientRejectsInvalidVehicleInput(t *testing.T) {
	client := newRecallsClient("https://example.test/recalls", &http.Client{Timeout: time.Second})
	if _, err := client.GetRecalls("", "TUCSON", 2016); err == nil {
		t.Fatal("expected validation error")
	}
}

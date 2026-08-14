package service

import (
	"testing"
	"time"

	"parts-engine/internal/model"
)

func TestVINCacheRetainsConfirmationAndRecallData(t *testing.T) {
	cache := NewVINCache(time.Minute)
	expected := &model.VINDecodeResult{
		VIN: "KM8J33A46GU123456",
		AllVariants: []model.Vehicle{
			{LinkageTargetId: 10001, Make: "HYUNDAI", Model: "TUCSON"},
			{LinkageTargetId: 10002, Make: "HYUNDAI", Model: "TUCSON"},
		},
		NeedsConfirmation: true,
		Recalls: []model.Recall{
			{NHTSACampaignNumber: "20V543000", SourceLabel: "NHTSA vehicle recall API"},
		},
	}

	cache.Set(expected.VIN, expected)
	got, ok := cache.Get(expected.VIN)
	if !ok {
		t.Fatal("expected cached VIN result")
	}
	if !got.NeedsConfirmation || len(got.AllVariants) != 2 || len(got.Recalls) != 1 || got.Recalls[0].NHTSACampaignNumber != "20V543000" {
		t.Fatalf("cached confirmation data was lost: %+v", got)
	}
}

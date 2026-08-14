package service

import (
	"parts-engine/internal/model"
)

// RecallsClient is a placeholder — no external API calls.
type RecallsClient struct{}

func NewRecallsClient() *RecallsClient {
	return &RecallsClient{}
}

// GetRecalls returns empty — no external API dependency.
func (c *RecallsClient) GetRecalls(make, modelName string, year int) ([]model.Recall, error) {
	return nil, nil
}

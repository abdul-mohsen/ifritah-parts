package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// CostHandler serves the M6.S2.T2 process-wide cost aggregate.
// Sourced from service.DefaultAggregate, which is merged into by
// SmartSearch.SearchWithProgress on every request completion. Read-only.
//
// Response shape (see cost_meter.go AggregateSnapshot):
//
//	{
//	  "requestsServed": 12345,
//	  "totalDbQueries": 456789,
//	  "totalDbBytes": 12345678,
//	  "totalExternalCalls": 12000,
//	  "totalExternalBytes": 5678901,
//	  "totalCacheHits": 456,
//	  "totalSlowQueries": 12,
//	  "totalCostUsd": 3.2,
//	  "totalCostUsdCents": 320.0,
//	  "avgCostUsdCentsPerRequest": 0.026,
//	  "sinceProcessStart": "2h13m10s",
//	  "sinceProcessStartMs": 8000000,
//	  "rates": { ... current CostRates ... }
//	}
//
// No auth gate — the payload is aggregate-only (no PII, no per-request
// leak). If deploy policy tightens later, gate at the router layer.
type CostHandler struct{}

// NewCostHandler returns a fresh CostHandler.
// The handler is stateless — the aggregate lives in service.DefaultAggregate.
func NewCostHandler() *CostHandler {
	return &CostHandler{}
}

// Aggregate handles GET /api/debug/cost.
func (h *CostHandler) Aggregate(c *gin.Context) {
	snap := service.DefaultAggregate.Snapshot()
	c.JSON(http.StatusOK, snap)
}

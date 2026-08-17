package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// SupersessionHandler handles part chain requests.
type SupersessionHandler struct {
	svc *service.Supersession
}

func NewSupersessionHandler(svc *service.Supersession) *SupersessionHandler {
	return &SupersessionHandler{svc: svc}
}

// GetChain handles GET /api/part/:id/chain.
func (h *SupersessionHandler) GetChain(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article ID"})
		return
	}

	chain, err := h.svc.GetChain(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"legacyArticleId": id,
		"chain":           chain,
		"total":           len(chain),
	})
}

// RecallsHandler handles recall lookup requests.
type RecallsHandler struct {
	svc *service.RecallsClient
}

func NewRecallsHandler(svc *service.RecallsClient) *RecallsHandler {
	return &RecallsHandler{svc: svc}
}

// ByVIN handles GET /api/recalls?make=&model=&year=.
// Returns 200 with an empty recalls list when the NHTSA API is unavailable
// (rate-limited, network error, or empty results) so callers can treat
// recalls as best-effort without treating the absence as a server error.
//
// Observability: the X-NHTSA-Available response header signals whether the
// upstream call succeeded. Values:
//   - "true"  — NHTSA returned successfully (may still have zero recalls)
//   - "false" — NHTSA request failed; recalls list is empty because of the
//              upstream error, NOT because there are no recalls
// Callers that must distinguish these cases can key on this header rather
// than on HTTP status (both are 200 by design).
func (h *RecallsHandler) ByVIN(c *gin.Context) {
	vehicleMake := c.Query("make")
	model := c.Query("model")
	yearStr := c.Query("year")

	if vehicleMake == "" || model == "" || yearStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "make, model, year query params required"})
		return
	}

	year, _ := strconv.Atoi(yearStr)

	recalls, err := h.svc.GetRecalls(vehicleMake, model, year)
	if err != nil {
		// NHTSA API failures are non-fatal: return 200 with empty list and a warning.
		// Callers (QA gate, frontend) treat empty recalls gracefully.
		c.Header("X-NHTSA-Available", "false")
		c.JSON(http.StatusOK, gin.H{
			"make":    vehicleMake,
			"model":   model,
			"year":    year,
			"recalls": []interface{}{},
			"total":   0,
			"warning": "NHTSA recalls API unavailable: " + err.Error(),
		})
		return
	}

	c.Header("X-NHTSA-Available", "true")
	c.JSON(http.StatusOK, gin.H{
		"make":    vehicleMake,
		"model":   model,
		"year":    year,
		"recalls": recalls,
		"total":   len(recalls),
	})
}

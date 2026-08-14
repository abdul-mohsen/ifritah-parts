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

// ByVIN handles GET /api/recalls/:vin.
func (h *RecallsHandler) ByVIN(c *gin.Context) {
	// For now, require make/model/year as query params
	// (full VIN decode integration is in the VIN handler)
	make := c.Query("make")
	model := c.Query("model")
	yearStr := c.Query("year")

	if make == "" || model == "" || yearStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "make, model, year query params required"})
		return
	}

	year, _ := strconv.Atoi(yearStr)

	recalls, err := h.svc.GetRecalls(make, model, year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"make":    make,
		"model":   model,
		"year":    year,
		"recalls": recalls,
		"total":   len(recalls),
	})
}

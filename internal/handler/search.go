package handler

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// SearchHandler handles smart search requests.
type SearchHandler struct {
	search *service.SmartSearch
}

func NewSearchHandler(search *service.SmartSearch) *SearchHandler {
	return &SearchHandler{search: search}
}

// validEnrichmentLevels is the set of allowed enrichmentLevel query values.
// Any value outside this set returns 400 rather than silently falling back.
var validEnrichmentLevels = map[string]bool{
	"":      true, // empty → default (basic)
	"none":  true,
	"basic": true,
	"full":  true,
}

// Search handles GET /api/search?q=&linkageTargetId=&vehicleCC=&fuelType=&category=&page=&limit=&mode=&enrichmentLevel=
func (h *SearchHandler) Search(c *gin.Context) {
	start := time.Now()
	q := c.Query("q")
	category := c.Query("category")
	fuelType := c.Query("fuelType")
	mode := c.Query("mode")
	enrichmentLevel := c.DefaultQuery("enrichmentLevel", "basic")

	linkageTargetId, _ := strconv.Atoi(c.Query("linkageTargetId"))
	vehicleCC, _ := strconv.Atoi(c.Query("vehicleCC"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	log.Printf("[SearchHandler] >>> GET /api/search q=%q vehicle=%d cc=%d fuel=%q cat=%q mode=%q enrichment=%q page=%d limit=%d",
		q, linkageTargetId, vehicleCC, fuelType, category, mode, enrichmentLevel, page, limit)

	if q == "" && linkageTargetId == 0 && category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide 'q' (search text), 'linkageTargetId', or 'category'"})
		return
	}

	// Input validation: 400 on invalid mode / enrichmentLevel rather than silent fallback.
	if mode != "" && !h.isValidMode(mode) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":       "unknown mode",
			"mode":        mode,
			"validModes":  h.validModeKeys(),
			"hint":        "Call GET /api/search/modes for the current list, or omit the mode param to use the default cascade",
		})
		return
	}
	if !validEnrichmentLevels[enrichmentLevel] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":            "unknown enrichmentLevel",
			"enrichmentLevel":  enrichmentLevel,
			"validValues":      []string{"none", "basic", "full"},
		})
		return
	}

	result, err := h.search.SearchWithOptions(q, linkageTargetId, vehicleCC, fuelType, category, page, limit, mode, enrichmentLevel)
	if err != nil {
		log.Printf("[SearchHandler] <<< ERROR after %v: %v", time.Since(start), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusOK, gin.H{"query": q, "results": []interface{}{}, "total": 0, "searchStrategy": "none"})
		return
	}

	// X-Search-Strategy response header — tells the caller which strategy produced
	// the result without having to parse the JSON body (useful for logs / observability).
	if result.SearchStrategy != "" {
		c.Header("X-Search-Strategy", result.SearchStrategy)
	}
	if result.Mode != "" {
		c.Header("X-Search-Mode", result.Mode)
	}

	log.Printf("[SearchHandler] <<< OK strategy=%q mode=%q results=%d elapsed=%v",
		result.SearchStrategy, result.Mode, result.Total, time.Since(start))
	c.JSON(http.StatusOK, result)
}

// isValidMode returns true when the requested mode is registered in
// SmartSearch.AvailableModes(). The set is dynamic — modes only appear when
// their backing TecDoc services are wired up, so we consult the live registry
// rather than a hardcoded list.
func (h *SearchHandler) isValidMode(mode string) bool {
	for _, m := range h.search.AvailableModes() {
		if m.Key == mode {
			return true
		}
	}
	return false
}

func (h *SearchHandler) validModeKeys() []string {
	modes := h.search.AvailableModes()
	keys := make([]string, 0, len(modes))
	for _, m := range modes {
		keys = append(keys, m.Key)
	}
	return keys
}

// Modes handles GET /api/search/modes — returns all available search strategy descriptors.
func (h *SearchHandler) Modes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"modes": h.search.AvailableModes(),
	})
}

// Categories handles GET /api/vehicle/:id/categories
func (h *SearchHandler) Categories(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vehicle id"})
		return
	}

	cats, err := h.search.GetCategories(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"linkageTargetId": id, "categories": cats, "total": len(cats)})
}

// CrossRef handles GET /api/part/:id/crossref
func (h *SearchHandler) CrossRef(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid part id"})
		return
	}

	vehicleCC, _ := strconv.Atoi(c.Query("vehicleCC"))
	category := c.Query("category")

	// Get OEM numbers
	crossRef := h.search
	oems, oerr := crossRef.GetOEMNumbers(id)

	// Get vehicles this part fits
	vehicles, verr := crossRef.GetVehiclesForArticle(id, vehicleCC, category, 50)

	if oerr != nil && verr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": oerr.Error()})
		return
	}
	if verr != nil {
		log.Printf("[CrossRef] GetVehiclesForArticle id=%d err=%v", id, verr)
	}

	c.JSON(http.StatusOK, gin.H{
		"legacyArticleId": id,
		"oemNumbers":      oems,
		"fitsVehicles":    vehicles,
	})
}

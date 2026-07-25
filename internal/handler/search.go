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

// Search handles GET /api/search?q=&linkageTargetId=&vehicleCC=&fuelType=&category=&page=&limit=
func (h *SearchHandler) Search(c *gin.Context) {
	start := time.Now()
	q := c.Query("q")
	category := c.Query("category")
	fuelType := c.Query("fuelType")

	linkageTargetId, _ := strconv.Atoi(c.Query("linkageTargetId"))
	vehicleCC, _ := strconv.Atoi(c.Query("vehicleCC"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	log.Printf("[SearchHandler] >>> GET /api/search q=%q vehicle=%d cc=%d fuel=%q cat=%q page=%d limit=%d",
		q, linkageTargetId, vehicleCC, fuelType, category, page, limit)

	if q == "" && linkageTargetId == 0 && category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide 'q' (search text), 'linkageTargetId', or 'category'"})
		return
	}

	result, err := h.search.Search(q, linkageTargetId, vehicleCC, fuelType, category, page, limit)
	if err != nil {
		log.Printf("[SearchHandler] <<< ERROR after %v: %v", time.Since(start), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[SearchHandler] <<< OK strategy=%q results=%d elapsed=%v",
		result.SearchStrategy, result.Total, time.Since(start))
	c.JSON(http.StatusOK, result)
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

	c.JSON(http.StatusOK, gin.H{
		"legacyArticleId": id,
		"oemNumbers":      oems,
		"fitsVehicles":    vehicles,
	})
}

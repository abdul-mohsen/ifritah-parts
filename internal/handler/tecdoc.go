package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// TecDocHandler exposes direct TecDoc queries when MySQL is connected.
type TecDocHandler struct {
	td *service.TecDoc
}

func NewTecDocHandler(td *service.TecDoc) *TecDocHandler {
	return &TecDocHandler{td: td}
}

// Specs returns technical specifications for a part.
// GET /api/tecdoc/specs/:id
func (h *TecDocHandler) Specs(c *gin.Context) {
	if h.td == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TecDoc not connected"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article id"})
		return
	}
	specs, err := h.td.ArticleSpecs(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"legacyArticleId": id, "specs": specs})
}

// Fitment checks if a part fits a vehicle.
// GET /api/tecdoc/fitment?article=123&vehicle=456
func (h *TecDocHandler) Fitment(c *gin.Context) {
	if h.td == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TecDoc not connected"})
		return
	}
	articleId, _ := strconv.Atoi(c.Query("article"))
	vehicleId, _ := strconv.Atoi(c.Query("vehicle"))
	if articleId == 0 || vehicleId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "article and vehicle params required"})
		return
	}
	fits, err := h.td.CheckFitment(articleId, vehicleId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"article": articleId, "vehicle": vehicleId, "fits": fits})
}

// Groups returns the assembly group hierarchy.
// GET /api/tecdoc/groups
func (h *TecDocHandler) Groups(c *gin.Context) {
	if h.td == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TecDoc not connected"})
		return
	}
	groups, err := h.td.GenericArticleGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"groups": groups, "total": len(groups)})
}

// Replacements returns supersession chain for a part.
// GET /api/tecdoc/replacements/:id
func (h *TecDocHandler) Replacements(c *gin.Context) {
	if h.td == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TecDoc not connected"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article id"})
		return
	}
	chain, err := h.td.FindReplacements(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"legacyArticleId": id, "replacements": chain, "total": len(chain)})
}

// VehicleParts returns parts for a vehicle from the full TecDoc database.
// GET /api/tecdoc/vehicle/:id/parts?category=&page=1&limit=30
func (h *TecDocHandler) VehicleParts(c *gin.Context) {
	if h.td == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TecDoc not connected"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vehicle id"})
		return
	}
	category := c.Query("category")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))

	results, total, err := h.td.PartsForVehicle(id, category, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"vehicleId": id, "results": results, "total": total, "page": page})
}

// VehicleGroups returns assembly group categories for a vehicle.
// GET /api/tecdoc/vehicle/:id/groups
func (h *TecDocHandler) VehicleGroups(c *gin.Context) {
	if h.td == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TecDoc not connected"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vehicle id"})
		return
	}
	groups, err := h.td.AssemblyGroups(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"vehicleId": id, "groups": groups, "total": len(groups)})
}

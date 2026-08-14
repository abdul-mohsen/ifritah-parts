package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// CatalogHandler serves the catalog browsing endpoints.
type CatalogHandler struct {
	parts *service.PartsLookup
	cross *service.CrossRef
}

func NewCatalogHandler(parts *service.PartsLookup, cross *service.CrossRef) *CatalogHandler {
	return &CatalogHandler{parts: parts, cross: cross}
}

// Models handles GET /api/catalog/models?make=HYUNDAI
func (h *CatalogHandler) Models(c *gin.Context) {
	make := c.Query("make")
	if make == "" {
		c.JSON(http.StatusOK, gin.H{
			"makes": []string{"HYUNDAI", "KIA"},
		})
		return
	}

	models, err := h.parts.ListModels(make)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"make": make, "models": models})
}

// Vehicles handles GET /api/catalog/vehicles?make=HYUNDAI&model=TUCSON
func (h *CatalogHandler) Vehicles(c *gin.Context) {
	make := c.Query("make")
	model := c.Query("model")
	if make == "" || model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide make and model"})
		return
	}

	vehicles, err := h.parts.ListVehicleVariants(make, model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"make": make, "model": model, "vehicles": vehicles, "total": len(vehicles)})
}

// Groups handles GET /api/catalog/groups?vehicleId=10001
func (h *CatalogHandler) Groups(c *gin.Context) {
	vid, err := strconv.Atoi(c.Query("vehicleId"))
	if err != nil || vid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide valid vehicleId"})
		return
	}

	groups, err := h.parts.ListAssemblyGroups(vid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"vehicleId": vid, "groups": groups, "total": len(groups)})
}

// GroupParts handles GET /api/catalog/parts?vehicleId=10001&groupId=10100
func (h *CatalogHandler) GroupParts(c *gin.Context) {
	vid, err := strconv.Atoi(c.Query("vehicleId"))
	if err != nil || vid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide valid vehicleId"})
		return
	}

	groupId, _ := strconv.Atoi(c.Query("groupId"))

	parts, err := h.parts.ListPartsByGroup(vid, groupId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Enrich with OEM cross-refs
	type enrichedPart struct {
		LegacyArticleId int      `json:"legacyArticleId"`
		ArticleNumber   string   `json:"articleNumber"`
		Description     string   `json:"description"`
		BrandName       string   `json:"brandName"`
		Category        string   `json:"category"`
		AssemblyGroupId int      `json:"assemblyGroupId"`
		OEMNumbers      []string `json:"oemNumbers,omitempty"`
	}

	var enriched []enrichedPart
	for _, p := range parts {
		ep := enrichedPart{
			LegacyArticleId: p.LegacyArticleId,
			ArticleNumber:   p.ArticleNumber,
			Description:     p.Description,
			BrandName:       p.BrandName,
			Category:        p.Category,
			AssemblyGroupId: p.AssemblyGroupId,
		}
		if oems, err := h.cross.FindOEMNumbers(p.LegacyArticleId); err == nil {
			for _, o := range oems {
				ep.OEMNumbers = append(ep.OEMNumbers, o.RawNumber)
			}
		}
		enriched = append(enriched, ep)
	}

	c.JSON(http.StatusOK, gin.H{"vehicleId": vid, "groupId": groupId, "parts": enriched, "total": len(enriched)})
}

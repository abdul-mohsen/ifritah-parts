package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// PartsHandler handles parts lookup requests.
type PartsHandler struct {
	parts        *service.PartsLookup
	oem          *service.OEMLookup
	engine       *service.EngineResolver
	alternatives *service.Alternatives
	categoryTree *service.CategoryTree
	tecdoc       *service.TecDoc
}

func NewPartsHandler(parts *service.PartsLookup, oem *service.OEMLookup) *PartsHandler {
	return &PartsHandler{parts: parts, oem: oem}
}

// SetEngineResolver attaches the engine resolver (only available with MySQL).
func (h *PartsHandler) SetEngineResolver(er *service.EngineResolver) {
	h.engine = er
}

// SetAlternatives attaches the alternatives service.
func (h *PartsHandler) SetAlternatives(a *service.Alternatives) {
	h.alternatives = a
}

// SetCategoryTree attaches the category tree service.
func (h *PartsHandler) SetCategoryTree(ct *service.CategoryTree) {
	h.categoryTree = ct
}

// SetTecDoc attaches the TecDoc service for criteria enrichment.
func (h *PartsHandler) SetTecDoc(td *service.TecDoc) {
	h.tecdoc = td
}

// ByVehicle handles GET /api/vehicle/:id/parts.
// TecDoc's articlesvehicletrees is the authoritative fitment source — parts returned
// for a linkageTargetId are already verified to fit that exact vehicle variant.
func (h *PartsHandler) ByVehicle(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid linkageTargetId"})
		return
	}

	category := c.Query("category")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	enrich := c.DefaultQuery("enrich", "false") == "true"

	rawParts, total, ferr := h.parts.FindByLinkageTarget(id, category, page, limit)
	if ferr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": ferr.Error()})
		return
	}

	var parts []service.PartWithOEM
	for _, p := range rawParts {
		parts = append(parts, service.PartWithOEM{Part: p})
	}

	// Enrich with OEM numbers if requested
	if enrich && h.oem != nil && len(parts) > 0 {
		ids := make([]int, len(parts))
		for i, p := range parts {
			ids[i] = p.LegacyArticleId
		}
		oemMap, _ := h.oem.BatchOEMNumbers(ids)
		if oemMap != nil {
			for i := range parts {
				if nums, ok := oemMap[parts[i].LegacyArticleId]; ok {
					parts[i].OEMNumbers = nums
				}
			}
		}
	}

	// Enrich with criteria specs if TecDoc is available
	var criteriaMap map[int]map[string]string
	if enrich && h.tecdoc != nil && len(parts) > 0 {
		criteriaMap = make(map[int]map[string]string)
		for _, p := range parts {
			specs, err := h.tecdoc.ArticleSpecs(p.LegacyArticleId)
			if err == nil && len(specs) > 0 {
				criteriaMap[p.LegacyArticleId] = specs
			}
		}
	}

	// Build response with optional criteria
	type enrichedPart struct {
		service.PartWithOEM
		Criteria map[string]string `json:"criteria,omitempty"`
	}
	resp := make([]enrichedPart, len(parts))
	for i, p := range parts {
		resp[i] = enrichedPart{PartWithOEM: p}
		if criteriaMap != nil {
			if specs, ok := criteriaMap[p.LegacyArticleId]; ok {
				resp[i].Criteria = specs
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"linkageTargetId": id,
		"page":            page,
		"limit":           limit,
		"total":           total,
		"parts":           resp,
	})
}

// ReverseByArticle handles GET /api/part/:id/vehicles.
func (h *PartsHandler) ReverseByArticle(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid legacyArticleId"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	vehicles, err := h.parts.ReverseByArticle(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"legacyArticleId": id,
		"total":           len(vehicles),
		"vehicles":        vehicles,
	})
}

// Engine handles GET /api/vehicle/:id/engine — returns resolved engine info (display only).
func (h *PartsHandler) Engine(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vehicle id"})
		return
	}
	if h.engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "engine resolver not available"})
		return
	}

	engines, err := h.engine.ResolveByLinkageTarget(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	codes := service.MotorCodes(engines)
	c.JSON(http.StatusOK, gin.H{
		"linkageTargetId": id,
		"motorCodes":      codes,
		"engines":         engines,
		"total":           len(engines),
	})
}

// CategoryTree handles GET /api/vehicle/:id/tree — returns hierarchical category tree.
func (h *PartsHandler) CategoryTree(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vehicle id"})
		return
	}
	if h.categoryTree == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "category tree not available"})
		return
	}

	tree, err := h.categoryTree.GetTreeForVehicle(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	totalParts := 0
	for _, g := range tree {
		totalParts += g.TotalParts
	}

	c.JSON(http.StatusOK, gin.H{
		"linkageTargetId": id,
		"tree":            tree,
		"totalGroups":     len(tree),
		"totalParts":      totalParts,
	})
}

// Alternatives handles GET /api/part/:id/alternatives — returns functionally equivalent parts.
func (h *PartsHandler) Alternatives(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid part id"})
		return
	}
	vehicleId, _ := strconv.Atoi(c.Query("vehicleId"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if h.alternatives == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "alternatives service not available"})
		return
	}

	alts, err := h.alternatives.FindForArticle(id, vehicleId, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"legacyArticleId": id,
		"alternatives":    alts,
		"total":           len(alts),
		"label":           "Also Compatible",
	})
}

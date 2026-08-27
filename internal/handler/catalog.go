package handler

import (
	"net/http"
	"strconv"
	"strings"

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

// normalizeCatalogArg trims whitespace and forces uppercase on user-supplied
// make/model query parameters. `vehicle_lookup` stores nhtsa_make and
// nhtsa_model as UPPERCASE (populated by scripts/derive_hk_maps: `'HYUNDAI'`,
// `'KIA'`, `'TUCSON'`, `'ELANTRA'`, etc.) so the underlying SQL query filters
// with a case-sensitive `nhtsa_make = $1` predicate. Before this normaliser
// existed, any browser request with mixed case (e.g. `?make=Hyundai&model=Elantra`)
// silently matched zero rows — the exact user-visible bug that M0.T4 fixes.
//
// Uppercasing in the handler keeps the DB query simple + the
// idx_vehicle_lookup_model B-tree usable (no functional index needed).
func normalizeCatalogArg(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}

// Models handles GET /api/catalog/models?make=HYUNDAI
func (h *CatalogHandler) Models(c *gin.Context) {
	make := normalizeCatalogArg(c.Query("make"))
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
//
// M0.T4: make + model are uppercased before the DB query so mixed-case
// input (`?make=Hyundai&model=Elantra`) resolves against the UPPERCASE
// values stored in vehicle_lookup. Case-insensitive by handler
// normalisation, not by SQL-side UPPER() calls — preserves index use.
func (h *CatalogHandler) Vehicles(c *gin.Context) {
	make := normalizeCatalogArg(c.Query("make"))
	model := normalizeCatalogArg(c.Query("model"))
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

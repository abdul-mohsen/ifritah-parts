package handler

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/model"
	"parts-engine/internal/service"
)

// OEMHandler handles OEM number lookup requests.
type OEMHandler struct {
	oem   *service.OEMLookup
	cross *service.CrossRef
	parts *service.PartsLookup
}

func NewOEMHandler(oem *service.OEMLookup) *OEMHandler {
	return &OEMHandler{oem: oem}
}

// SetCrossRef attaches the cross-reference service for vehicle resolution.
func (h *OEMHandler) SetCrossRef(cr *service.CrossRef) {
	h.cross = cr
}

// SetPartsLookup attaches the parts lookup for reverse vehicle resolution.
func (h *OEMHandler) SetPartsLookup(pl *service.PartsLookup) {
	h.parts = pl
}

// Lookup handles GET /api/oem/:number.
// Returns the OEM part, all aftermarket alternatives, and vehicles that use it.
func (h *OEMHandler) Lookup(c *gin.Context) {
	start := time.Now()
	number := c.Param("number")
	if number == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "OEM number required"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	includeVehicles := c.DefaultQuery("vehicles", "true") == "true"

	log.Printf("[OEMHandler] >>> GET /api/oem/%s limit=%d vehicles=%v", number, limit, includeVehicles)

	result, err := h.oem.Search(number, limit)
	if err != nil {
		log.Printf("[OEMHandler] <<< ERROR after %v: %v", time.Since(start), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[OEMHandler] oem.Search returned %d results in %v", result.Total, time.Since(start))
	if h.parts != nil {
		directParts, directErr := h.parts.FindByArticleNumber(number, 0, limit)
		if directErr != nil {
			log.Printf("[OEMHandler] exact catalog part lookup failed: %v", directErr)
		} else {
			seen := make(map[int]bool, len(result.Results))
			for _, ref := range result.Results {
				seen[ref.LegacyArticleId] = true
			}
			directRefs := make([]model.OEMReference, 0, len(directParts))
			for _, part := range directParts {
				if seen[part.LegacyArticleId] {
					continue
				}
				seen[part.LegacyArticleId] = true
				directRefs = append(directRefs, model.OEMReference{
					RawNumber:       number,
					Normalized:      service.NormalizeOEM(number),
					LegacyArticleId: part.LegacyArticleId,
					Manufacturer:    "HYUNDAI/KIA",
					BrandName:       part.BrandName,
					ArticleNumber:   part.ArticleNumber,
					Description:     part.Description,
				})
			}
			if len(directRefs) > 0 {
				result.Results = append(directRefs, result.Results...)
				result.Total = len(result.Results)
			}
		}
	}

	resp := gin.H{
		"query":      number,
		"normalized": result.Normalized,
		"results":    result.Results,
		"total":      result.Total,
	}

	// Enrich: find vehicles that use these parts
	if includeVehicles && h.cross != nil && len(result.Results) > 0 {
		articleId := result.Results[0].LegacyArticleId
		log.Printf("[OEMHandler] enriching vehicles for articleId=%d", articleId)
		if articleId > 0 {
			vehicles, verr := h.cross.FindVehiclesForArticle(articleId, 0, "", 20)
			if verr == nil && len(vehicles) > 0 {
				resp["fitsVehicles"] = vehicles
			}
		}
	}

	// Decode OEM prefix category
	prefix := service.DecodeOEMPrefix(number)
	if prefix != nil {
		resp["oemCategory"] = prefix
	}

	log.Printf("[OEMHandler] <<< OK results=%d elapsed=%v", result.Total, time.Since(start))
	c.JSON(http.StatusOK, resp)
}

package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	mdl "parts-engine/internal/model"
	"parts-engine/internal/service"
)

// VINHandler handles VIN decode requests.
type VINHandler struct {
	decoder  *service.VINDecoder
	parts    *service.PartsLookup
	platform *service.Platform
	recalls  *service.RecallsClient
	cache    *service.VINCache
}

func NewVINHandler(decoder *service.VINDecoder, parts *service.PartsLookup, platform *service.Platform, recalls *service.RecallsClient, cache *service.VINCache) *VINHandler {
	return &VINHandler{decoder: decoder, parts: parts, platform: platform, recalls: recalls, cache: cache}
}

type vinRequest struct {
	VIN string `json:"vin" binding:"required"`
}

// Decode handles POST /api/vin/decode.
func (h *VINHandler) Decode(c *gin.Context) {
	var req vinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "VIN is required"})
		return
	}

	vin := strings.ToUpper(strings.TrimSpace(req.VIN))

	// Check cache
	if cached, ok := h.cache.Get(vin); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	// Step 1: Validate
	if err := h.decoder.ValidateVIN(vin); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Step 2: Decode via NHTSA
	nhtsa, err := h.decoder.DecodeVIN(vin)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to decode VIN: " + err.Error()})
		return
	}

	make := strings.ToUpper(nhtsa.Make)
	model := strings.ToUpper(nhtsa.Model)
	year := service.ParseModelYear(nhtsa.ModelYear)

	// Parse engine CC from NHTSA for smarter variant matching
	var engineCC int
	if nhtsa.EngineCC != "" {
		if v, err := strconv.ParseFloat(nhtsa.EngineCC, 64); err == nil {
			engineCC = int(v)
		}
	}

	result := gin.H{
		"vin":      vin,
		"nhtsaRaw": nhtsa,
		"vehicle":  nil,
	}

	// Step 3: Resolve ALL matching TecDoc variants for this make/model/year
	allVariants, varErr := h.parts.ResolveLinkageTargets(make, model, year)
	if varErr != nil {
		result["dbWarning"] = varErr.Error()
	}
	if len(allVariants) > 0 {
		result["allVariants"] = allVariants
	}

	// Step 4: Pick best variant using engine hints (auto-select)
	vehicle, err := h.parts.BestLinkageTargetWithHints(make, model, year, engineCC, nhtsa.FuelType)
	if err != nil {
		// DB error is non-fatal: we still have NHTSA data
		result["dbWarning"] = err.Error()
	}
	if vehicle != nil {
		result["vehicle"] = vehicle
		// Flag if there are multiple variants (frontend should show configurator)
		if len(allVariants) > 1 {
			result["needsConfirmation"] = true
		}

		// Step 5: Get parts if vehicle found.
		parts, total, perr := h.parts.FindByLinkageTarget(vehicle.LinkageTargetId, "", 1, 20)
		if perr == nil {
			result["parts"] = parts
			result["totalParts"] = total
		}

		// Step 6: Cross-brand suggestions
		siblings, _ := h.platform.FindSiblings(make, model)
		if len(siblings) > 0 {
			result["crossBrand"] = siblings
		}
	}

	// Step 6: Recalls
	recalls, _ := h.recalls.GetRecalls(make, model, year)
	if len(recalls) > 0 {
		result["recalls"] = recalls
	}

	// Build cacheable result
	cacheResult := &mdl.VINDecodeResult{
		VIN:      vin,
		NHTSARaw: nhtsa,
		Vehicle:  vehicle,
	}
	if p, ok := result["parts"]; ok {
		cacheResult.Parts = p.([]mdl.Part)
	}
	if t, ok := result["totalParts"]; ok {
		cacheResult.TotalParts = t.(int)
	}
	if recalls, ok := result["recalls"]; ok {
		cacheResult.Recalls = recalls.([]mdl.Recall)
	}
	if len(allVariants) > 0 {
		cacheResult.AllVariants = allVariants
	}
	if needsConfirmation, ok := result["needsConfirmation"].(bool); ok {
		cacheResult.NeedsConfirmation = needsConfirmation
	}
	h.cache.Set(vin, cacheResult)

	c.JSON(http.StatusOK, result)
}

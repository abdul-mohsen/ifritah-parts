package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// VINPartsHandler serves GET /api/vin/:vin/parts.
//
// Composed endpoint:
//  1. Decode VIN via VINDecoder -> NHTSAVehicle (make, model, year)
//  2. Resolve to TecDoc linkageTargetIds via TecDoc.LinkageTargetsForNHTSA
//  3. Fetch parts for each linkage via TecDoc.PartsForVehicle (bounded to
//     the first 3 linkage targets so a fuzzy match doesn't fan out).
//  4. Merge + dedupe + group by category (optionally filtered).
//
// M5.S2.T2. Fills the "vin_assembly" strategy's gap: previously the
// user had to know a linkageTargetId to see parts for their car;
// now they just paste the VIN.
//
// GET /api/vin/:vin/parts?category=filters&limit=50
type VINPartsHandler struct {
	vin    *service.VINDecoder
	tecdoc *service.TecDoc
}

func NewVINPartsHandler(vin *service.VINDecoder, tecdoc *service.TecDoc) *VINPartsHandler {
	return &VINPartsHandler{vin: vin, tecdoc: tecdoc}
}

// Get is the gin handler.
func (h *VINPartsHandler) Get(c *gin.Context) {
	vin := c.Param("vin")
	if h.vin == nil || h.tecdoc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "VIN parts service not configured (needs VIN decoder + TecDoc MySQL)",
		})
		return
	}
	if err := h.vin.ValidateVIN(vin); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Step 1: decode
	vehicle, err := h.vin.DecodeVIN(vin)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"vin":   vin,
			"error": "VIN decode failed: " + err.Error(),
			"parts": []any{},
			"total": 0,
		})
		return
	}
	year, _ := strconv.Atoi(vehicle.ModelYear)

	// Step 2: NHTSA → linkageTargetId(s)
	linkageIds, err := h.tecdoc.LinkageTargetsForNHTSA(vehicle.Make, vehicle.Model, year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "linkage resolution failed: " + err.Error(),
			"vin":     vin,
			"vehicle": vehicle,
		})
		return
	}
	if len(linkageIds) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"vin":              vin,
			"vehicle":          vehicle,
			"linkageTargetIds": []int{},
			"parts":            []any{},
			"total":            0,
			"warning":          "VIN decoded but no matching TecDoc vehicle found — try /api/catalog/vehicles to search manually",
		})
		return
	}

	// Step 3: fetch parts for top-3 linkage targets, merge
	category := c.Query("category")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	maxLinkages := 3
	if len(linkageIds) < maxLinkages {
		maxLinkages = len(linkageIds)
	}

	type resultKey struct {
		articleNumber string
		brand         string
	}
	seen := make(map[resultKey]bool)
	byCategory := make(map[string][]service.SmartResult)
	total := 0

	for i := 0; i < maxLinkages; i++ {
		lid := linkageIds[i]
		parts, _, perr := h.tecdoc.PartsForVehicle(lid, category, 1, limit)
		if perr != nil {
			continue
		}
		for _, p := range parts {
			key := resultKey{articleNumber: p.ArticleNumber, brand: p.BrandName}
			if seen[key] {
				continue
			}
			seen[key] = true
			cat := p.Category
			if cat == "" {
				cat = "Uncategorised"
			}
			byCategory[cat] = append(byCategory[cat], p)
			total++
			if total >= limit {
				break
			}
		}
		if total >= limit {
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"vin":              vin,
		"vehicle":          vehicle,
		"linkageTargetIds": linkageIds,
		"category":         category,
		"parts":            byCategory,
		"total":            total,
	})
}

package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// PlatformHandler exposes platform compatibility endpoints.
type PlatformHandler struct {
	platform *service.Platform
	parts    *service.PartsLookup
}

func NewPlatformHandler(platform *service.Platform, parts *service.PartsLookup) *PlatformHandler {
	return &PlatformHandler{platform: platform, parts: parts}
}

// Siblings handles GET /api/vehicle/:id/platform.
// Returns cross-brand siblings with shared part counts.
func (h *PlatformHandler) Siblings(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vehicle id"})
		return
	}

	// Look up the vehicle to get make/model
	vehicleMake := c.Query("make")
	vehicleModel := c.Query("model")
	if vehicleMake == "" || vehicleModel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "make and model query params required"})
		return
	}

	siblings, err := h.platform.FindSiblings(vehicleMake, vehicleModel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Enrich with shared part counts if parts service is available
	if h.parts != nil {
		for i, sib := range siblings {
			count, cerr := h.parts.CountSharedParts(id, sib.SiblingMake, sib.SiblingModel)
			if cerr == nil {
				siblings[i].SharedParts = count
			}
		}
	}

	type siblingResp struct {
		SiblingMake  string `json:"siblingMake"`
		SiblingModel string `json:"siblingModel"`
		Platform     string `json:"platform"`
		SharedParts  int    `json:"sharedParts"`
	}

	resp := make([]siblingResp, len(siblings))
	for i, s := range siblings {
		resp[i] = siblingResp{
			SiblingMake:  s.SiblingMake,
			SiblingModel: s.SiblingModel,
			Platform:     s.Platform,
			SharedParts:  s.SharedParts,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"linkageTargetId": id,
		"make":            vehicleMake,
		"model":           vehicleModel,
		"siblings":        resp,
	})
}

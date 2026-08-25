package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// RelatedPartsHandler serves /api/parts/related.
//
// GET /api/parts/related?oem={OEM}&limit={N}
//   - Decodes OEM via prefixMap to a category.
//   - Returns top-N related categories from db.related_parts sorted
//     by priority desc, correlation desc.
//   - Empty when the OEM doesn't decode (partial stem, non-HK, dealer
//     accessory prefix). Empty is NOT an error — the frontend can still
//     render the primary result without suggested-related parts.
//
// GET /api/parts/related?category={Category}&limit={N}
//   - Direct-category variant when the caller already knows the category.
//
// One of `oem` or `category` is required.
type RelatedPartsHandler struct {
	related *service.RelatedParts
}

func NewRelatedPartsHandler(rp *service.RelatedParts) *RelatedPartsHandler {
	return &RelatedPartsHandler{related: rp}
}

// Get is the gin handler.
func (h *RelatedPartsHandler) Get(c *gin.Context) {
	if h.related == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "related-parts service not configured",
		})
		return
	}

	oem := c.Query("oem")
	category := c.Query("category")
	if oem == "" && category == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "one of 'oem' or 'category' is required",
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))

	var (
		related []service.RelatedPart
		err     error
	)
	if oem != "" {
		related, err = h.related.FindRelatedByOEM(c.Request.Context(), oem, limit)
	} else {
		related, err = h.related.FindRelatedByCategory(c.Request.Context(), category, limit)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"oem":      oem,
		"category": category,
		"related":  related,
		"total":    len(related),
	})
}

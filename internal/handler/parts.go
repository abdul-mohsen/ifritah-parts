package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/model"
	"parts-engine/internal/service"
)

// PartsHandler handles parts lookup requests.
type PartsHandler struct {
	parts        *service.PartsLookup
	oem          *service.OEMLookup
	cross        *service.CrossRef
	alternatives *service.Alternatives
	categoryTree *service.CategoryTree
	placement    *service.PlacementAdvisor
	replacements *service.ReplacementAdvisor
	tecdoc       *service.TecDoc
}

func NewPartsHandler(parts *service.PartsLookup, oem *service.OEMLookup) *PartsHandler {
	return &PartsHandler{parts: parts, oem: oem}
}

// SetCrossRef attaches the cross-reference service.
func (h *PartsHandler) SetCrossRef(cr *service.CrossRef) {
	h.cross = cr
}

// SetAlternatives attaches the alternatives service.
func (h *PartsHandler) SetAlternatives(a *service.Alternatives) {
	h.alternatives = a
}

// SetCategoryTree attaches the category tree service.
func (h *PartsHandler) SetCategoryTree(ct *service.CategoryTree) {
	h.categoryTree = ct
}

// SetPlacementAdvisor attaches placement and diagram guidance service.
func (h *PartsHandler) SetPlacementAdvisor(pa *service.PlacementAdvisor) {
	h.placement = pa
}

// SetReplacementAdvisor attaches conservative replacement suggestion service.
func (h *PartsHandler) SetReplacementAdvisor(ra *service.ReplacementAdvisor) {
	h.replacements = ra
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

	// Step 0: MySQL/TecDoc is the source of truth for vehicle fitment.
	// When connected, its articlesvehicletrees join is authoritative — the
	// local Postgres cache is a snapshot of it. Prefer TecDoc when available.
	var rawParts []model.Part
	var total int
	var ferr error
	if h.tecdoc != nil {
		tdResults, tdTotal, tdErr := h.tecdoc.PartsForVehicle(id, category, page, limit)
		if tdErr == nil && len(tdResults) > 0 {
			total = tdTotal
			rawParts = make([]model.Part, 0, len(tdResults))
			for _, r := range tdResults {
				rawParts = append(rawParts, r.Part)
			}
		}
	}
	if rawParts == nil {
		rawParts, total, ferr = h.parts.FindByLinkageTarget(id, category, page, limit)
		if ferr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": ferr.Error()})
			return
		}
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

// Detail handles GET /api/part/:id/detail.
func (h *PartsHandler) Detail(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid part id"})
		return
	}

	vehicleID, _ := strconv.Atoi(c.Query("vehicleId"))

	part, err := h.parts.FindByArticle(id, vehicleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if part == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "part not found"})
		return
	}

	var (
		oemNumbers     []string
		criteria       map[string]string
		fitVehicles    []any
		alternatives   []any
		replacements   []any
		warnings       []string
		sourceKind     = "owned_catalog"
		sourceLabel    = "Owned catalog detail"
		sourceDetail   = "Core part identity is from the owned PostgreSQL catalog. Each enriched section is marked unavailable when its evidence is absent."
		confidence     = 0.9
		confidenceNote = "This detail comes from the owned catalog path for the selected part."
	)

	if h.oem != nil {
		oemNumbers, _ = h.oem.OEMNumbersForArticle(id)
	}
	if len(oemNumbers) == 0 && h.cross != nil {
		refs, _ := h.cross.FindOEMNumbers(id)
		for _, ref := range refs {
			if ref.RawNumber != "" {
				oemNumbers = append(oemNumbers, ref.RawNumber)
			}
		}
	}
	if len(oemNumbers) == 0 {
		warnings = append(warnings, "No OEM references are available for this part in the current runtime mode.")
		confidence = 0.84
	}

	if h.tecdoc != nil {
		criteria, _ = h.tecdoc.ArticleSpecs(id)
	}
	if len(criteria) == 0 {
		warnings = append(warnings, "Technical specifications are unavailable in the current runtime mode.")
		confidence = minFloat(confidence, 0.86)
	}

	if h.alternatives != nil {
		alts, _ := h.alternatives.FindForArticle(id, vehicleID, 6)
		for _, alt := range alts {
			alternatives = append(alternatives, gin.H{
				"legacyArticleId": alt.LegacyArticleId,
				"articleNumber":   alt.ArticleNumber,
				"description":     alt.Description,
				"brandName":       alt.BrandName,
				"category":        alt.Category,
				"assemblyGroupId": alt.AssemblyGroupId,
				"sharedVehicles":  alt.SharedVehicles,
			})
		}
	}

	if h.replacements != nil {
		candidates, candidateWarnings, err := h.replacements.Build(part, vehicleID, uniqueStrings(oemNumbers), 6)
		if err != nil {
			warnings = append(warnings, "Replacement suggestions could not be fully expanded from the current evidence set.")
		} else {
			warnings = append(warnings, candidateWarnings...)
			for _, candidate := range candidates {
				replacements = append(replacements, gin.H{
					"legacyArticleId": candidate.LegacyArticleId,
					"articleNumber":   candidate.ArticleNumber,
					"description":     candidate.Description,
					"brandName":       candidate.BrandName,
					"category":        candidate.Category,
					"assemblyGroupId": candidate.AssemblyGroupId,
					"candidateType":   candidate.CandidateType,
					"explanation":     candidate.Explanation,
					"oemReference":    candidate.OEMReference,
					"confidence":      candidate.Confidence,
					"source":          candidate.Source,
					"warnings":        candidate.Warnings,
				})
			}
		}
	}

	vehicles, err := h.parts.ReverseByArticle(id, 8)
	if err == nil {
		for _, vehicle := range vehicles {
			fitVehicles = append(fitVehicles, vehicle)
		}
	} else {
		warnings = append(warnings, "Vehicle fitment context could not be expanded for this part.")
	}

	if vehicleID == 0 {
		confidenceNote = "This detail comes from the owned catalog path, but without a selected vehicle context."
		confidence = minFloat(confidence, 0.82)
	}

	var selectedVehicle *model.Vehicle
	for _, item := range fitVehicles {
		if vehicle, ok := item.(model.Vehicle); ok {
			if vehicleID > 0 && vehicle.LinkageTargetId == vehicleID {
				selectedVehicle = &vehicle
				break
			}
			if selectedVehicle == nil {
				copyVehicle := vehicle
				selectedVehicle = &copyVehicle
			}
		}
	}

	placement := model.PartPlacement{}
	if h.placement != nil {
		placement = h.placement.Build(part, selectedVehicle, uniqueStrings(oemNumbers))
	}
	if placement.Kind == "unavailable" {
		confidence = minFloat(confidence, 0.8)
		warnings = append(warnings, "No exact diagram is loaded for this part yet; placement remains unavailable rather than over-claimed.")
	}

	provenanceGaps := make([]string, 0, 4)
	if len(oemNumbers) == 0 {
		provenanceGaps = append(provenanceGaps, "OEM reference evidence")
	}
	if len(criteria) == 0 {
		provenanceGaps = append(provenanceGaps, "technical specification evidence")
	}
	if len(fitVehicles) == 0 {
		provenanceGaps = append(provenanceGaps, "expanded vehicle fitment evidence")
	}
	if placement.Kind == "" || placement.Kind == "unavailable" {
		provenanceGaps = append(provenanceGaps, "placement evidence")
	}
	provenanceComplete := len(provenanceGaps) == 0
	if !provenanceComplete {
		warnings = append(warnings, "Evidence is incomplete for: "+strings.Join(provenanceGaps, ", ")+".")
	}

	c.JSON(http.StatusOK, gin.H{
		"legacyArticleId": part.LegacyArticleId,
		"vehicleId":       vehicleID,
		"articleNumber":   part.ArticleNumber,
		"description":     part.Description,
		"brandName":       part.BrandName,
		"category":        part.Category,
		"assemblyGroupId": part.AssemblyGroupId,
		"oemNumbers":      uniqueStrings(oemNumbers),
		"criteria":        criteria,
		"fitVehicles":     fitVehicles,
		"replacements":    replacements,
		"alternatives":    alternatives,
		"placement":       placement,
		"source": gin.H{
			"kind":   sourceKind,
			"label":  sourceLabel,
			"detail": sourceDetail,
		},
		"confidence": gin.H{
			"score":  confidence,
			"reason": confidenceNote,
		},
		"quality": gin.H{
			"provenanceComplete":       provenanceComplete,
			"provenanceGaps":           provenanceGaps,
			"hasOEMNumbers":            len(oemNumbers) > 0,
			"hasCriteria":              len(criteria) > 0,
			"hasVehicleContext":        vehicleID > 0,
			"hasFitmentEvidence":       len(fitVehicles) > 0,
			"hasPlacement":             placement.Kind != "" && placement.Kind != "unavailable",
			"placementExact":           placement.Kind == "exact",
			"hasReplacementCandidates": len(replacements) > 0,
		},
		"warnings": warnings,
	})
}

// Engine reports that engine-code filtering is unavailable on the PostgreSQL runtime.
func (h *PartsHandler) Engine(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid vehicle id"})
		return
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":                    "engine-code filtering is not available on the PostgreSQL catalog runtime",
		"linkageTargetId":          id,
		"engineFilteringAvailable": false,
		"fitmentMethod":            "Use the confirmed vehicle variant linkage target; it is the supported fitment constraint.",
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

func uniqueStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	var out []string
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func minFloat(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

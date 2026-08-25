package handler

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// CommunityContribHandler serves the /api/aftermarket/contribute route
// (submit) and the /api/admin/moderation routes (list + review).
//
// Rate limit: 10 contributions per IP per day, enforced here (not in
// gin's rate-limit middleware because the semantics are business-logic
// not global).
type CommunityContribHandler struct {
	svc            *service.CommunityContribService
	dailyPerIP     int
	adminAuthToken string // simple bearer for admin routes; empty disables admin auth (dev mode)
}

func NewCommunityContribHandler(svc *service.CommunityContribService) *CommunityContribHandler {
	return &CommunityContribHandler{svc: svc, dailyPerIP: 10}
}

// SetAdminAuthToken configures the bearer token admin endpoints check
// against. When empty, admin routes accept any request (dev mode only).
func (h *CommunityContribHandler) SetAdminAuthToken(tok string) {
	h.adminAuthToken = tok
}

// ContributeRequest is the JSON body for POST /api/aftermarket/contribute.
type ContributeRequest struct {
	OEM         string `json:"oem"         binding:"required"`
	Brand       string `json:"brand"       binding:"required"`
	PartNumber  string `json:"partNumber"  binding:"required"`
	Description string `json:"description,omitempty"`
	SourceURL   string `json:"sourceUrl,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Contributor string `json:"contributor,omitempty"`
}

// Submit is POST /api/aftermarket/contribute.
func (h *CommunityContribHandler) Submit(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "community-contrib service not configured",
		})
		return
	}
	var req ContributeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ip := c.ClientIP()

	// Rate limit — count recent submissions from the same IP.
	if h.dailyPerIP > 0 && ip != "" {
		count, err := h.svc.CountRecentByIP(c.Request.Context(), ip)
		if err != nil {
			log.Printf("[contribute] rate-limit check err: %v", err)
			// Fail open on rate-limit lookup errors — don't block submissions.
		} else if count >= h.dailyPerIP {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "daily submission limit reached; try again tomorrow",
				"limit": h.dailyPerIP,
				"used":  count,
			})
			return
		}
	}

	id, err := h.svc.Submit(c.Request.Context(), service.CommunityContribution{
		OEMNormalized: req.OEM,
		Brand:         req.Brand,
		PartNumber:    req.PartNumber,
		Description:   req.Description,
		SourceURL:     req.SourceURL,
		Notes:         req.Notes,
		Contributor:   req.Contributor,
		ContributorIP: ip,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":      id,
		"status":  "pending",
		"message": "thanks — your contribution is queued for review",
	})
}

// requireAdmin gates the admin routes on a bearer token when configured.
// Returns true when the request should proceed.
func (h *CommunityContribHandler) requireAdmin(c *gin.Context) bool {
	if h.adminAuthToken == "" {
		return true // dev mode
	}
	auth := c.GetHeader("Authorization")
	expected := "Bearer " + h.adminAuthToken
	if auth != expected {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "admin token required"})
		return false
	}
	return true
}

// ListPending is GET /api/admin/moderation/pending.
func (h *CommunityContribHandler) ListPending(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service not configured"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	items, err := h.svc.ListPending(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"pending": items,
		"total":   len(items),
	})
}

// ReviewRequest is the JSON body for POST /api/admin/moderation/:id/review.
type ReviewRequest struct {
	Decision string `json:"decision" binding:"required,oneof=approved rejected"`
	Reviewer string `json:"reviewer,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Review is POST /api/admin/moderation/:id/review.
func (h *CommunityContribHandler) Review(c *gin.Context) {
	if !h.requireAdmin(c) {
		return
	}
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service not configured"})
		return
	}
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req ReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err = h.svc.Review(c.Request.Context(), id,
		service.ContribStatus(req.Decision), req.Reviewer, req.Note)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":       id,
		"decision": req.Decision,
	})
}

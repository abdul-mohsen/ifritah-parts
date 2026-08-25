package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// FeedbackHandler serves POST /api/search/feedback.
//
// Records a single thumbs-up / thumbs-down from a user against a search
// result. Rate-limited to prevent spam (see cmd/server/main.go). No
// authentication — feedback is anonymous by design; sessionId is optional
// and helps de-dupe on the aggregation side.
type FeedbackHandler struct {
	fb *service.FeedbackService
}

func NewFeedbackHandler(fb *service.FeedbackService) *FeedbackHandler {
	return &FeedbackHandler{fb: fb}
}

// FeedbackRequest is the JSON body shape.
type FeedbackRequest struct {
	QueryOEM    string `json:"queryOem"    binding:"required"`
	ResultOEM   string `json:"resultOem"   binding:"required"`
	ResultDesc  string `json:"resultDesc,omitempty"`
	ResultBrand string `json:"resultBrand,omitempty"`
	Verdict     string `json:"verdict"     binding:"required,oneof=up down"`
	Reason      string `json:"reason,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
}

func (h *FeedbackHandler) Submit(c *gin.Context) {
	if h.fb == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "feedback service not configured",
		})
		return
	}
	var req FeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := h.fb.Submit(c.Request.Context(), service.SearchFeedback{
		QueryOEM:    req.QueryOEM,
		ResultOEM:   req.ResultOEM,
		ResultDesc:  req.ResultDesc,
		ResultBrand: req.ResultBrand,
		Verdict:     service.FeedbackVerdict(req.Verdict),
		Reason:      req.Reason,
		SessionID:   req.SessionID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":     id,
		"status": "recorded",
	})
}

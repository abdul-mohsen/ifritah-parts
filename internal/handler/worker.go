package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/model"
	"parts-engine/internal/service"
)

type WorkerHandler struct {
	store *service.WorkerStore
}

func NewWorkerHandler(store *service.WorkerStore) *WorkerHandler {
	return &WorkerHandler{store: store}
}

func (h *WorkerHandler) SubmitReplacement(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker store not available"})
		return
	}
	var input model.WorkerReplacementSubmissionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission payload"})
		return
	}
	submission, err := h.store.SubmitReplacement(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, submission)
}

func (h *WorkerHandler) ListReplacements(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker store not available"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	status := c.Query("status")
	submissions, err := h.store.ListReplacementSubmissions(status, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"submissions": submissions,
		"total":       len(submissions),
		"status":      status,
	})
}

func (h *WorkerHandler) ReviewReplacement(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker store not available"})
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid submission id"})
		return
	}
	var input model.WorkerReplacementReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review payload"})
		return
	}
	submission, err := h.store.ReviewReplacement(id, input.Action, input.Reviewer, input.ReviewNotes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, submission)
}

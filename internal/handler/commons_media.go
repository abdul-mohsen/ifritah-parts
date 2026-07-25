package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/model"
	"parts-engine/internal/service"
)

type CommonsMediaHandler struct{ store *service.CommonsMediaStore }

func NewCommonsMediaHandler(store *service.CommonsMediaStore) *CommonsMediaHandler {
	return &CommonsMediaHandler{store: store}
}

func (h *CommonsMediaHandler) Submit(c *gin.Context) {
	var input model.CommonsMediaSubmission
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Commons media review store is not available"})
		return
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Commons media submission"})
		return
	}
	item, err := h.store.Submit(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *CommonsMediaHandler) List(c *gin.Context) {
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Commons media review store is not available"})
		return
	}
	items, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

func (h *CommonsMediaHandler) Review(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if h.store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Commons media review store is not available"})
		return
	}
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid media review id"})
		return
	}
	var input model.CommonsMediaReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Commons media review"})
		return
	}
	item, err := h.store.Review(id, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

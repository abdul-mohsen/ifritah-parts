package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// SemanticSearchHandler serves GET /api/search/semantic?q=X&topK=N.
//
// Natural-language search over the article_embeddings vector index.
// Returns [{legacyArticleId, description, score}, ...] sorted by
// cosine similarity, filtered by the minScore floor configured on
// the service.
//
// M5.S1.T2.
type SemanticSearchHandler struct {
	svc *service.SemanticSearch
}

func NewSemanticSearchHandler(svc *service.SemanticSearch) *SemanticSearchHandler {
	return &SemanticSearchHandler{svc: svc}
}

func (h *SemanticSearchHandler) Get(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "semantic search not configured (needs pgvector + embedder sidecar)",
		})
		return
	}
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q is required"})
		return
	}
	topK, _ := strconv.Atoi(c.DefaultQuery("topK", "20"))

	results, err := h.svc.Search(c.Request.Context(), q, topK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"query":   q,
		"topK":    topK,
		"results": results,
		"total":   len(results),
	})
}

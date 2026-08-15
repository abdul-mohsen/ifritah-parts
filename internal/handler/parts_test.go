package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEngineReportsUnsupportedFiltering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	parts := NewPartsHandler(nil, nil)
	router.GET("/api/vehicle/:id/engine", parts.Engine)

	request := httptest.NewRequest(http.MethodGet, "/api/vehicle/10001/engine", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotImplemented)
	}
	if got := response.Body.String(); !strings.Contains(got, "engineFilteringAvailable") || !strings.Contains(got, "confirmed vehicle variant linkage target") {
		t.Fatalf("response does not explain supported fitment behavior: %s", got)
	}
}

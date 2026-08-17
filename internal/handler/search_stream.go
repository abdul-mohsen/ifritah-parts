package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/service"
)

// SearchStream handles GET /api/search/stream — same params as /api/search but
// responds with Server-Sent Events so the browser can show live progress.
//
// Event format (each line is terminated by \n\n):
//
//	data: {"type":"progress","step":"oem_index","label":"Checking OEM index…","elapsed_ms":12}
//	data: {"type":"progress","step":"oem_index","label":"Checking OEM index…","elapsed_ms":340,"done":true,"count":8}
//	data: {"type":"result","results":[…],"total":8,"searchStrategy":"tecdoc_crossref","elapsed_ms":890}
//	data: {"type":"done","elapsed_ms":1200}
//	data: [DONE]
func (h *SearchHandler) SearchStream(c *gin.Context) {
	start := time.Now()
	q := c.Query("q")
	category := c.Query("category")
	fuelType := c.Query("fuelType")
	mode := c.Query("mode")
	enrichmentLevel := c.DefaultQuery("enrichmentLevel", "basic")

	linkageTargetId, _ := strconv.Atoi(c.Query("linkageTargetId"))
	vehicleCC, _ := strconv.Atoi(c.Query("vehicleCC"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if q == "" && linkageTargetId == 0 && category == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provide 'q', 'linkageTargetId', or 'category'"})
		return
	}
	if mode != "" && !h.isValidMode(mode) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown mode", "mode": mode, "validModes": h.validModeKeys()})
		return
	}
	if !validEnrichmentLevels[enrichmentLevel] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown enrichmentLevel", "validValues": []string{"none", "basic", "full"}})
		return
	}

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // disable Nginx buffering in dev proxy

	w := c.Writer
	flusher, canFlush := w.(http.Flusher)

	sendEvent := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", b)
		if canFlush {
			flusher.Flush()
		}
	}

	// Wire up progress channel — buffered so strategy goroutines never block.
	progressCh := make(chan service.ProgressEvent, 32)

	// Run the search in a goroutine; close the channel when done.
	type searchResult struct {
		resp *service.SmartSearchResponse
		err  error
	}
	done := make(chan searchResult, 1)
	go func() {
		resp, err := h.search.SearchWithProgress(
			q, linkageTargetId, vehicleCC, fuelType, category,
			page, limit, mode, enrichmentLevel,
			progressCh,
		)
		close(progressCh)
		done <- searchResult{resp, err}
	}()

	// Stream progress events until the channel closes.
	for ev := range progressCh {
		if ev.Type == "done" {
			// Don't forward the internal done sentinel — we'll emit the final
			// result event instead.
			continue
		}
		sendEvent(ev)
	}

	// Retrieve final result.
	sr := <-done
	if sr.err != nil {
		sendEvent(map[string]any{
			"type":       "error",
			"error":      sr.err.Error(),
			"elapsed_ms": time.Since(start).Milliseconds(),
		})
		fmt.Fprint(w, "data: [DONE]\n\n")
		if canFlush {
			flusher.Flush()
		}
		return
	}

	// Emit the result event — same JSON as /api/search but wrapped in type=result.
	if sr.resp != nil {
		resultEnvelope := map[string]any{
			"type":           "result",
			"query":          sr.resp.Query,
			"results":        sr.resp.Results,
			"total":          sr.resp.Total,
			"searchStrategy": sr.resp.SearchStrategy,
			"mode":           sr.resp.Mode,
			"warnings":       sr.resp.Warnings,
			"categories":     sr.resp.Categories,
			"elapsed_ms":     time.Since(start).Milliseconds(),
		}
		sendEvent(resultEnvelope)
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	if canFlush {
		flusher.Flush()
	}
}

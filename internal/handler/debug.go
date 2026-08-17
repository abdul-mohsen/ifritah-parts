package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"parts-engine/internal/debug"
)

// DebugHandler streams server log lines via SSE.
// Only registered when DEBUG_LOGS=1 is set in the environment.
type DebugHandler struct {
	logger *debug.Logger
}

func NewDebugHandler(logger *debug.Logger) *DebugHandler {
	return &DebugHandler{logger: logger}
}

// LogStream handles GET /api/debug/logs
// Streams all buffered log lines first (so the browser sees recent history),
// then delivers new lines as they arrive.
//
// Event format: data: {"ts":"...","level":"INFO","msg":"[SearchHandler] >>>..."}
func (h *DebugHandler) LogStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	w := c.Writer
	flusher, canFlush := w.(http.Flusher)

	// First flush: send buffered history.
	for _, line := range h.logger.Snapshot() {
		fmt.Fprint(w, debug.SSELine(line))
	}
	if canFlush {
		flusher.Flush()
	}

	// Subscribe to live lines.
	ch := h.logger.Subscribe()
	defer h.logger.Unsubscribe(ch)

	// Stream until client disconnects.
	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, debug.SSELine(line))
			if canFlush {
				flusher.Flush()
			}
		}
	}
}

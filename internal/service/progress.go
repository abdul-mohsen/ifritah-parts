package service

import "time"

// ProgressEvent is a single step-update emitted while a search runs.
// It is sent over the ProgressCh channel and forwarded to the SSE stream
// by the search handler so the browser can show a live step list.
type ProgressEvent struct {
	Type      string `json:"type"`                // "progress" | "done" | "error"
	Step      string `json:"step"`                // machine key, e.g. "oem_index"
	Label     string `json:"label"`               // human-readable, e.g. "Checking OEM index…"
	ElapsedMs int64  `json:"elapsed_ms"`          // wall-clock ms from search start
	Done      bool   `json:"done,omitempty"`      // true once the step finished
	Count     int    `json:"count,omitempty"`     // result count for the step (optional)
}

// progressStep is a lightweight helper that emits start + done events.
// Returns a function that must be called when the step completes.
//
//	defer step(ch, start, "oem_index", "Checking OEM index…")()
func progressStep(ch chan<- ProgressEvent, start time.Time, stepKey, label string) func(count int) {
	if ch == nil {
		return func(int) {}
	}
	send(ch, ProgressEvent{
		Type:      "progress",
		Step:      stepKey,
		Label:     label,
		ElapsedMs: time.Since(start).Milliseconds(),
	})
	return func(count int) {
		send(ch, ProgressEvent{
			Type:      "progress",
			Step:      stepKey,
			Label:     label,
			ElapsedMs: time.Since(start).Milliseconds(),
			Done:      true,
			Count:     count,
		})
	}
}

func send(ch chan<- ProgressEvent, e ProgressEvent) {
	if ch == nil {
		return
	}
	select {
	case ch <- e:
	default: // never block the search path
	}
}

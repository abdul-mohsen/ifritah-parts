// Package debug provides dev-mode request tracing and a ring-buffer log streamer.
// Nothing in this package runs in production — every exported symbol is guarded
// by the caller checking Config.DebugLogs before calling into it.
package debug

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

// LogLine is one structured log entry emitted by the debug logger.
type LogLine struct {
	TS    string `json:"ts"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// ringBuf is a fixed-capacity circular buffer of LogLines.
type ringBuf struct {
	mu   sync.RWMutex
	buf  []LogLine
	head int
	size int
	cap  int
}

func newRingBuf(capacity int) *ringBuf {
	return &ringBuf{buf: make([]LogLine, capacity), cap: capacity}
}

func (r *ringBuf) push(l LogLine) {
	r.mu.Lock()
	r.buf[r.head%r.cap] = l
	r.head++
	if r.size < r.cap {
		r.size++
	}
	r.mu.Unlock()
}

// snapshot returns a copy of all lines in chronological order.
func (r *ringBuf) snapshot() []LogLine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.size == 0 {
		return nil
	}
	out := make([]LogLine, r.size)
	start := 0
	if r.size == r.cap {
		start = r.head % r.cap
	}
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(start+i)%r.cap]
	}
	return out
}

// Logger wraps the standard library logger and fans log lines out to a
// ring buffer + a set of live subscribers.
type Logger struct {
	inner *log.Logger
	ring  *ringBuf
	mu    sync.RWMutex
	subs  map[chan LogLine]struct{}
}

// New creates a Logger that writes to w (e.g. os.Stderr) while also keeping
// the last `ringSize` lines in memory for late subscribers.
func New(w io.Writer, ringSize int) *Logger {
	l := &Logger{
		ring: newRingBuf(ringSize),
		subs: make(map[chan LogLine]struct{}),
	}
	// Tee writer: go to `w` AND into our ring/broadcast.
	l.inner = log.New(&teeWriter{w: w, logger: l}, "", log.LstdFlags)
	return l
}

// Write implements io.Writer so Logger can be used with log.SetOutput.
// Every write is parsed as a log line and broadcast to subscribers.
func (l *Logger) Write(p []byte) (int, error) {
	return l.inner.Writer().Write(p)
}

// Printf writes a formatted log line.
func (l *Logger) Printf(format string, args ...any) {
	l.inner.Printf(format, args...)
}

// Subscribe returns a channel that receives new log lines.
// The caller must call Unsubscribe when done to avoid a leak.
func (l *Logger) Subscribe() chan LogLine {
	ch := make(chan LogLine, 64)
	l.mu.Lock()
	l.subs[ch] = struct{}{}
	l.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (l *Logger) Unsubscribe(ch chan LogLine) {
	l.mu.Lock()
	delete(l.subs, ch)
	l.mu.Unlock()
	// Drain so sender never blocks.
	for len(ch) > 0 {
		<-ch
	}
}

// Snapshot returns all buffered log lines (up to ringSize).
func (l *Logger) Snapshot() []LogLine {
	return l.ring.snapshot()
}

// SSELine serialises a LogLine as an SSE data frame.
func SSELine(line LogLine) string {
	b, _ := json.Marshal(line)
	return fmt.Sprintf("data: %s\n\n", b)
}

// broadcast is called by teeWriter for every line.
func (l *Logger) broadcast(line LogLine) {
	l.ring.push(line)
	l.mu.RLock()
	for ch := range l.subs {
		select {
		case ch <- line:
		default: // subscriber too slow — drop rather than block
		}
	}
	l.mu.RUnlock()
}

// teeWriter implements io.Writer so it can be used as a log.Logger output.
type teeWriter struct {
	w      io.Writer
	logger *Logger
}

func (t *teeWriter) Write(p []byte) (int, error) {
	n, err := t.w.Write(p)
	msg := strings.TrimRight(string(p), "\n")
	level := "INFO"
	if strings.Contains(msg, "ERROR") || strings.Contains(msg, "error") {
		level = "ERROR"
	} else if strings.Contains(msg, "WARN") || strings.Contains(msg, "⚠") {
		level = "WARN"
	}
	t.logger.broadcast(LogLine{
		TS:    time.Now().Format(time.RFC3339),
		Level: level,
		Msg:   msg,
	})
	return n, err
}

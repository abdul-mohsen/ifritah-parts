package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// UnixSocketEmbedder is the production EmbedderClient. Connects to the
// sentence-transformer sidecar running as a separate Python process on
// a unix domain socket. Same wire protocol as cmd/embed_articles:
//
//	REQ  {"texts": ["oil filter", "brake pad"]}
//	RESP {"embeddings": [[0.1, -0.2, ...], [0.3, ...]]}
//
// Falls back to a noop when the socket doesn't exist so the server
// boots cleanly on machines without the sidecar. Semantic-search then
// returns a "service not configured" error instead of crashing.
type UnixSocketEmbedder struct {
	SocketPath string
	Timeout    time.Duration
}

func NewUnixSocketEmbedder(socketPath string) *UnixSocketEmbedder {
	return &UnixSocketEmbedder{
		SocketPath: socketPath,
		Timeout:    5 * time.Second,
	}
}

// Embed implements EmbedderClient.
func (e *UnixSocketEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.SocketPath == "" {
		return nil, fmt.Errorf("no embedder socket configured")
	}

	deadline := e.Timeout
	if d, ok := ctx.Deadline(); ok {
		if remaining := time.Until(d); remaining < deadline {
			deadline = remaining
		}
	}

	conn, err := net.DialTimeout("unix", e.SocketPath, deadline)
	if err != nil {
		return nil, fmt.Errorf("dial embedder socket %s: %w", e.SocketPath, err)
	}
	defer conn.Close()

	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	} else {
		_ = conn.SetDeadline(time.Now().Add(deadline))
	}

	req, err := json.Marshal(struct {
		Texts []string `json:"texts"`
	}{Texts: texts})
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte{'\n'}); err != nil {
		return nil, err
	}

	var resp struct {
		Embeddings [][]float32 `json:"embeddings"`
		Error      string      `json:"error,omitempty"`
	}
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("embedder: %s", resp.Error)
	}
	return resp.Embeddings, nil
}

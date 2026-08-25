package service

import (
	"context"
	"database/sql"
	"fmt"
)

// SemanticSearch queries article_embeddings by cosine similarity so
// natural-language input like "oil filter for Sonata 2020" returns the
// most relevant HK articles even when the query doesn't contain a
// specific OEM. Powers M5.S1.T2 and the /api/search/semantic endpoint.
type SemanticSearch struct {
	pg           *sql.DB
	embedder     EmbedderClient
	minScore     float64
	defaultLimit int
}

// EmbedderClient abstracts the sentence-transformer sidecar so tests can
// inject a stub. The production impl connects to a unix socket the
// Python embedder listens on (see cmd/embed_articles/main.go).
type EmbedderClient interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// SemanticResult is one row from the cosine-similarity search.
type SemanticResult struct {
	LegacyArticleID int     `json:"legacyArticleId"`
	Description     string  `json:"description"`
	Score           float64 `json:"score"` // 1 - cosine_distance; higher = closer match
}

func NewSemanticSearch(pg *sql.DB, embedder EmbedderClient) *SemanticSearch {
	return &SemanticSearch{
		pg:           pg,
		embedder:     embedder,
		minScore:     0.5,
		defaultLimit: 20,
	}
}

// Search runs a semantic query. Steps:
//  1. Embed the query text via the sidecar.
//  2. Run pgvector `<=>` cosine-distance query with ORDER BY distance ASC.
//  3. Convert distance -> score (1 - distance), filter by minScore.
//
// Returns up to `topK` results (default 20, clamp [1, 100]).
func (s *SemanticSearch) Search(ctx context.Context, query string, topK int) ([]SemanticResult, error) {
	if s.pg == nil {
		return nil, fmt.Errorf("semantic search: postgres not configured")
	}
	if s.embedder == nil {
		return nil, fmt.Errorf("semantic search: embedder not configured")
	}
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if topK <= 0 || topK > 100 {
		topK = s.defaultLimit
	}

	// Embed the query.
	embeds, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(embeds) != 1 || len(embeds[0]) == 0 {
		return nil, fmt.Errorf("embedder returned no vector for query")
	}
	vec := embeds[0]
	vecStr := vectorToPgString(vec)

	// Query — `<=>` is cosine distance in pgvector.
	const q = `
		SELECT legacy_article_id, description, 1 - (embedding <=> $1::vector) AS score
		FROM article_embeddings
		WHERE embedding IS NOT NULL
		ORDER BY embedding <=> $1::vector
		LIMIT $2`

	rows, err := s.pg.QueryContext(ctx, q, vecStr, topK)
	if err != nil {
		return nil, fmt.Errorf("semantic search query: %w", err)
	}
	defer rows.Close()

	var out []SemanticResult
	for rows.Next() {
		var r SemanticResult
		if err := rows.Scan(&r.LegacyArticleID, &r.Description, &r.Score); err != nil {
			continue
		}
		if r.Score < s.minScore {
			// Results come back sorted; once we're below the floor,
			// nothing else will match.
			break
		}
		out = append(out, r)
	}
	return out, nil
}

// vectorToPgString formats a float32 slice as pgvector text literal
// "[1.0,2.0,3.0]". Same helper as the embedder cmd — duplicated to
// avoid a cross-package dep (small function, low churn risk).
func vectorToPgString(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	buf := make([]byte, 0, len(v)*10)
	buf = append(buf, '[')
	for i, f := range v {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = fmt.Appendf(buf, "%g", f)
	}
	buf = append(buf, ']')
	return string(buf)
}

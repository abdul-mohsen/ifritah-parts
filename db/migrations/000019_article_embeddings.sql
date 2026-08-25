-- ============================================================================
-- 000019 - article_embeddings (M5.S1.T1)
-- ============================================================================
-- pgvector-backed semantic search over TecDoc article descriptions. One
-- row per article; embedding is a 384-dim vector produced by the
-- sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2 model.
--
-- Runtime: 27M articles at ~50 embeds/sec = ~150 hours end-to-end.
-- Batch job (cmd/embed_articles) is idempotent — safe to re-run,
-- skips articles that already have embeddings.
--
-- Index: IVFFlat with 1000 lists. Tuned for HK-scale corpus; adjust
-- when article count doubles.
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS article_embeddings (
	legacy_article_id  INTEGER PRIMARY KEY,
	description        TEXT NOT NULL,
	embedding          vector(384),
	model              TEXT NOT NULL,
	created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- IVFFlat index for cosine-similarity search. Lists=1000 tuned for
-- ~27M rows; sqrt(N) heuristic says 5000 but IVFFlat quality/speed
-- tradeoff prefers fewer larger lists at this scale.
--
-- pgvector's `<->` operator is L2 distance; `<=>` is cosine distance.
-- We index for cosine because semantic similarity queries are
-- direction-based, not magnitude-based.
CREATE INDEX IF NOT EXISTS idx_article_embeddings_cosine
	ON article_embeddings USING ivfflat (embedding vector_cosine_ops)
	WITH (lists = 1000);

CREATE INDEX IF NOT EXISTS idx_article_embeddings_model
	ON article_embeddings (model);

-- ============================================================================
-- 000019 - article_embeddings (M5.S1.T1)
-- ============================================================================
-- pgvector-backed semantic search over TecDoc article descriptions. One
-- row per article; embedding is a 384-dim vector produced by the
-- sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2 model.
--
-- Runtime: 27M articles at ~50 embeds/sec = ~150 hours end-to-end.
-- Batch job (cmd/embed_articles) is idempotent - safe to re-run,
-- skips articles that already have embeddings.
--
-- Soft-dependency on pgvector: when the extension isn't available on
-- the target Postgres (vanilla postgres:17 in CI, some managed hosts),
-- the whole migration NO-OPS with a NOTICE rather than failing. The
-- runtime's SemanticSearch service returns 503 when article_embeddings
-- doesn't exist, so semantic-search degrades gracefully to disabled
-- instead of blocking server boot.
--
-- Production deployments should install pgvector separately (e.g. via
-- the pgvector/pgvector:pg17 image or `apt install postgresql-17-pgvector`)
-- to enable the feature; the app doesn't require it.
-- ============================================================================

DO $$
BEGIN
	-- Attempt to enable pgvector. Every downstream DDL is guarded by the
	-- outer EXCEPTION block, so a missing extension turns this whole
	-- migration into a no-op with a NOTICE.
	CREATE EXTENSION IF NOT EXISTS vector;

	-- Table
	EXECUTE $ddl$
		CREATE TABLE IF NOT EXISTS article_embeddings (
			legacy_article_id  INTEGER PRIMARY KEY,
			description        TEXT NOT NULL,
			embedding          vector(384),
			model              TEXT NOT NULL,
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	$ddl$;

	-- IVFFlat cosine-similarity index; lists=1000 tuned for ~27M rows.
	EXECUTE $ddl$
		CREATE INDEX IF NOT EXISTS idx_article_embeddings_cosine
			ON article_embeddings USING ivfflat (embedding vector_cosine_ops)
			WITH (lists = 1000)
	$ddl$;

	EXECUTE $ddl$
		CREATE INDEX IF NOT EXISTS idx_article_embeddings_model
			ON article_embeddings (model)
	$ddl$;

	RAISE NOTICE 'pgvector: article_embeddings ready';
EXCEPTION
	WHEN feature_not_supported OR undefined_file THEN
		-- SQLSTATE 0A000: pgvector .so not installed on the Postgres host.
		-- The most common case for CI + fresh docker environments.
		RAISE NOTICE 'pgvector not installed on this Postgres — skipping article_embeddings. Semantic search will be disabled at runtime (SemanticSearch returns 503).';
	WHEN undefined_object THEN
		-- vector type unavailable for some other reason (e.g. superuser
		-- rejected CREATE EXTENSION). Same graceful skip.
		RAISE NOTICE 'pgvector types not found — skipping article_embeddings';
END
$$;

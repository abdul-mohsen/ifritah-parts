-- ============================================================================
-- 000016 - search_feedback (M6.S2.T1)
-- ============================================================================
-- Captures thumbs-up / thumbs-down on individual search results so we
-- can measure UX quality alongside algorithmic F1. Feeds weekly reports
-- via /docs/reports/feedback-{week}.md.
--
-- One row per (query_oem, result_oem, session_id) tuple - so a user can
-- change their mind by re-submitting on the same session.
-- ============================================================================

CREATE TABLE IF NOT EXISTS search_feedback (
	id            BIGSERIAL PRIMARY KEY,
	query_oem     TEXT NOT NULL,
	result_oem    TEXT NOT NULL,
	result_desc   TEXT,
	result_brand  TEXT,
	verdict       TEXT NOT NULL CHECK (verdict IN ('up', 'down')),
	reason        TEXT,
	session_id    TEXT,
	submitted_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_search_feedback_query_oem
	ON search_feedback (query_oem);
CREATE INDEX IF NOT EXISTS idx_search_feedback_verdict
	ON search_feedback (verdict);
CREATE INDEX IF NOT EXISTS idx_search_feedback_submitted_at
	ON search_feedback (submitted_at DESC);

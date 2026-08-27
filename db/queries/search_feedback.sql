-- Search-feedback SQL queries (M6.S2.T1).
--
-- These queries are the source-of-truth for the SQL used by
-- internal/service/feedback.go. They are documented here in the
-- sqlc-style so that a future refactor can plug them into sqlc.yaml
-- and generate typed Go — but the current runtime uses raw
-- db.QueryContext / QueryRowContext so that:
--
--   1. New columns can be added without a code-generation step.
--   2. The generated store package doesn't grow a Postgres-only
--      shape (date_trunc, COUNT(*) FILTER, …) that would fight the
--      existing sqlite-backed unit tests in cmd/*.
--
-- To move this to sqlc later:
--   1. Add "db/queries/search_feedback.sql" under `sql.queries` in
--      sqlc.yaml.
--   2. Run `sqlc generate`.
--   3. Swap the raw SQL in feedback.go for the generated store methods.

-- name: InsertSearchFeedback :one
--
-- Records one thumbs-up / thumbs-down / skip event. All PII fields
-- (user_hash, client_ip_hash) are ALREADY SHA256-hashed by the caller
-- — this table never sees a raw session cookie or IP address.
INSERT INTO search_feedback (
    search_id,
    query_oem,
    result_article_id,
    result_brand,
    result_part_num,
    verdict,
    reason,
    user_hash,
    client_ip_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, created_at;

-- name: AggregateFeedbackWeekly :many
--
-- Per-week vote counts for the last 90 days, one row per (week, verdict).
-- Powers docs/reports/user-feedback-{week}.md and the /api/feedback/weekly
-- endpoint. Empty result set when the table is empty — the handler must
-- return `[]` (not null) in that case for a stable API shape.
SELECT
    date_trunc('week', created_at)::date AS week_start,
    verdict,
    COUNT(*)                             AS votes
FROM search_feedback
WHERE created_at >= NOW() - INTERVAL '90 days'
GROUP BY 1, 2
ORDER BY 1 DESC, 2;

-- name: TopDisputedOEMs :many
--
-- OEMs that have accumulated ≥ 3 thumbs-down votes in the last 30 days.
-- Feeds the M7 ranker's negative-signal training set and the weekly
-- report's "what to fix next" table. Limit 50 keeps the payload small
-- when there is a spike.
SELECT
    query_oem,
    COUNT(*) FILTER (WHERE verdict = 'thumbs_down') AS down_votes,
    COUNT(*) FILTER (WHERE verdict = 'thumbs_up')   AS up_votes,
    COUNT(*)                                         AS total_votes
FROM search_feedback
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY query_oem
HAVING COUNT(*) FILTER (WHERE verdict = 'thumbs_down') >= 3
ORDER BY down_votes DESC
LIMIT 50;

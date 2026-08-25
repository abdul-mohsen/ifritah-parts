-- ============================================================================
-- 000018 - aftermarket_community (M4.S4.T1)
-- ============================================================================
-- User-submitted aftermarket cross-references. Anyone can POST an OEM +
-- aftermarket brand/part pair via /api/aftermarket/contribute. Admins
-- review + approve/reject via /api/admin/moderation. Approved entries
-- flow into FindAftermarketForOEM as a lowest-priority path (below
-- articlecrosses + oem_number + oem_search_index + aftermarket_rockauto
-- + aftermarket_regional).
--
-- Rate limit: 10 contributions per IP per day (enforced at the handler
-- level; DB just stores).
-- ============================================================================

CREATE TABLE IF NOT EXISTS aftermarket_community (
	id              BIGSERIAL PRIMARY KEY,
	oem_normalized  TEXT NOT NULL,
	brand           TEXT NOT NULL,
	part_number     TEXT NOT NULL,
	description     TEXT,
	source_url      TEXT,
	notes           TEXT,
	contributor     TEXT,                                 -- opaque contributor id (session or email)
	contributor_ip  INET,                                  -- captured for rate limiting + spam review
	status          TEXT NOT NULL DEFAULT 'pending'
		CHECK (status IN ('pending', 'approved', 'rejected')),
	submitted_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	reviewed_at     TIMESTAMPTZ,
	reviewed_by     TEXT,
	review_note     TEXT
);

CREATE INDEX IF NOT EXISTS idx_aftermarket_community_status
	ON aftermarket_community (status, submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_aftermarket_community_oem
	ON aftermarket_community (oem_normalized)
	WHERE status = 'approved';
CREATE INDEX IF NOT EXISTS idx_aftermarket_community_ip_day
	ON aftermarket_community (contributor_ip, submitted_at);

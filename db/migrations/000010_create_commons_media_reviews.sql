CREATE TABLE IF NOT EXISTS commons_media_reviews (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    category_norm TEXT NOT NULL,
    media_url TEXT NOT NULL UNIQUE,
    thumbnail_url TEXT NOT NULL DEFAULT '',
    file_page_url TEXT NOT NULL,
    license_name TEXT NOT NULL,
    license_url TEXT NOT NULL,
    attribution TEXT NOT NULL,
    source_revision TEXT NOT NULL DEFAULT '',
    identity_scope TEXT NOT NULL DEFAULT 'generic_component',
    review_status TEXT NOT NULL DEFAULT 'pending',
    review_notes TEXT NOT NULL DEFAULT '',
    reviewed_by TEXT NOT NULL DEFAULT '',
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT commons_media_identity_scope_check CHECK (identity_scope = 'generic_component'),
    CONSTRAINT commons_media_review_status_check CHECK (review_status IN ('pending', 'approved', 'rejected')),
    CONSTRAINT commons_media_license_check CHECK (license_name IN ('CC0', 'CC BY 4.0'))
);

CREATE INDEX IF NOT EXISTS idx_commons_media_approved_category
    ON commons_media_reviews (category_norm)
    WHERE review_status = 'approved';

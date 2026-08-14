CREATE TABLE IF NOT EXISTS external_sources (
    source_key             TEXT PRIMARY KEY,
    display_name           TEXT NOT NULL,
    website_url            TEXT NOT NULL,
    data_type              TEXT NOT NULL,
    access_method          TEXT NOT NULL,
    license_risk           TEXT NOT NULL,
    hyundai_kia_usefulness TEXT NOT NULL,
    multi_make_usefulness  TEXT NOT NULL,
    false_positive_risk    TEXT NOT NULL,
    recommendation         TEXT NOT NULL,
    user_facing_eligible   BOOLEAN NOT NULL DEFAULT FALSE,
    freshness_notes        TEXT NOT NULL DEFAULT '',
    rate_limit_notes       TEXT NOT NULL DEFAULT '',
    notes                  TEXT NOT NULL DEFAULT '',
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS external_source_assessments (
    source_key            TEXT PRIMARY KEY REFERENCES external_sources(source_key) ON DELETE CASCADE,
    sample_scope          TEXT NOT NULL,
    evidence_score        INTEGER NOT NULL,
    precision_score       INTEGER NOT NULL,
    duplicate_noise_score INTEGER NOT NULL,
    false_positive_score  INTEGER NOT NULL,
    qa_decision           TEXT NOT NULL,
    rationale             TEXT NOT NULL,
    assessed_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS external_part_refs (
    id               BIGSERIAL PRIMARY KEY,
    source_key       TEXT NOT NULL REFERENCES external_sources(source_key) ON DELETE CASCADE,
    part_number_norm TEXT NOT NULL,
    brand_norm       TEXT NOT NULL DEFAULT '',
    make_norm        TEXT NOT NULL DEFAULT '',
    model_norm       TEXT NOT NULL DEFAULT '',
    vehicle_hint     TEXT NOT NULL DEFAULT '',
    exact_match      BOOLEAN NOT NULL DEFAULT FALSE,
    confidence       DOUBLE PRECISION NOT NULL DEFAULT 0,
    provenance_url   TEXT NOT NULL DEFAULT '',
    discovered_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_external_part_refs_lookup
    ON external_part_refs (part_number_norm, make_norm, model_norm);

CREATE TABLE IF NOT EXISTS external_artifacts (
    id             BIGSERIAL PRIMARY KEY,
    source_key     TEXT NOT NULL REFERENCES external_sources(source_key) ON DELETE CASCADE,
    artifact_type  TEXT NOT NULL,
    title          TEXT NOT NULL,
    media_url      TEXT NOT NULL DEFAULT '',
    thumbnail_url  TEXT NOT NULL DEFAULT '',
    mime_type      TEXT NOT NULL DEFAULT '',
    exactness      TEXT NOT NULL DEFAULT 'unknown',
    confidence     DOUBLE PRECISION NOT NULL DEFAULT 0,
    license_note   TEXT NOT NULL DEFAULT '',
    provenance_url TEXT NOT NULL DEFAULT '',
    fetched_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_external_artifacts_source_type
    ON external_artifacts (source_key, artifact_type);

CREATE TABLE IF NOT EXISTS external_install_hints (
    id               BIGSERIAL PRIMARY KEY,
    source_key       TEXT NOT NULL REFERENCES external_sources(source_key) ON DELETE CASCADE,
    part_number_norm TEXT NOT NULL DEFAULT '',
    make_norm        TEXT NOT NULL DEFAULT '',
    model_norm       TEXT NOT NULL DEFAULT '',
    hint_type        TEXT NOT NULL,
    hint_text        TEXT NOT NULL,
    exactness        TEXT NOT NULL DEFAULT 'unknown',
    confidence       DOUBLE PRECISION NOT NULL DEFAULT 0,
    provenance_url   TEXT NOT NULL DEFAULT '',
    fetched_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_external_install_hints_lookup
    ON external_install_hints (part_number_norm, make_norm, model_norm);

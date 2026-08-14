-- name: UpsertExternalSource :exec
INSERT INTO external_sources (
    source_key,
    display_name,
    website_url,
    data_type,
    access_method,
    license_risk,
    hyundai_kia_usefulness,
    multi_make_usefulness,
    false_positive_risk,
    recommendation,
    user_facing_eligible,
    freshness_notes,
    rate_limit_notes,
    notes,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW()
)
ON CONFLICT (source_key) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    website_url = EXCLUDED.website_url,
    data_type = EXCLUDED.data_type,
    access_method = EXCLUDED.access_method,
    license_risk = EXCLUDED.license_risk,
    hyundai_kia_usefulness = EXCLUDED.hyundai_kia_usefulness,
    multi_make_usefulness = EXCLUDED.multi_make_usefulness,
    false_positive_risk = EXCLUDED.false_positive_risk,
    recommendation = EXCLUDED.recommendation,
    user_facing_eligible = EXCLUDED.user_facing_eligible,
    freshness_notes = EXCLUDED.freshness_notes,
    rate_limit_notes = EXCLUDED.rate_limit_notes,
    notes = EXCLUDED.notes,
    updated_at = NOW();

-- name: UpsertExternalSourceAssessment :exec
INSERT INTO external_source_assessments (
    source_key,
    sample_scope,
    evidence_score,
    precision_score,
    duplicate_noise_score,
    false_positive_score,
    qa_decision,
    rationale,
    assessed_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, NOW()
)
ON CONFLICT (source_key) DO UPDATE SET
    sample_scope = EXCLUDED.sample_scope,
    evidence_score = EXCLUDED.evidence_score,
    precision_score = EXCLUDED.precision_score,
    duplicate_noise_score = EXCLUDED.duplicate_noise_score,
    false_positive_score = EXCLUDED.false_positive_score,
    qa_decision = EXCLUDED.qa_decision,
    rationale = EXCLUDED.rationale,
    assessed_at = NOW();

-- name: ListUserFacingEligibleSources :many
SELECT source_key
FROM external_sources
WHERE user_facing_eligible = TRUE
  AND recommendation = $1
  AND false_positive_risk IN ('very_low', 'low')
  AND license_risk IN ('very_low', 'low')
ORDER BY source_key;

-- name: FindExternalInstallHintsByPart :many
SELECT
    source_key,
    part_number_norm,
    make_norm,
    model_norm,
    hint_type,
    hint_text,
    exactness,
    confidence,
    provenance_url
FROM external_install_hints
WHERE part_number_norm = $1
  AND (make_norm = '' OR make_norm = $2)
  AND (model_norm = '' OR model_norm = $3)
ORDER BY
    CASE exactness WHEN 'exact' THEN 0 WHEN 'catalog_group' THEN 1 WHEN 'inferred' THEN 2 ELSE 3 END,
    confidence DESC,
    source_key;

-- name: FindExternalArtifactsBySource :many
SELECT
    source_key,
    artifact_type,
    title,
    media_url,
    thumbnail_url,
    mime_type,
    exactness,
    confidence,
    license_note,
    provenance_url
FROM external_artifacts
WHERE source_key = $1
ORDER BY
    CASE exactness WHEN 'exact' THEN 0 WHEN 'catalog_group' THEN 1 WHEN 'inferred' THEN 2 ELSE 3 END,
    confidence DESC,
    title;

-- name: CreateCommonsMediaReview :one
INSERT INTO commons_media_reviews (
    title,
    category_norm,
    media_url,
    thumbnail_url,
    file_page_url,
    license_name,
    license_url,
    attribution,
    source_revision
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, title, category_norm, media_url, thumbnail_url, file_page_url,
    license_name, license_url, attribution, source_revision, identity_scope,
    review_status, review_notes, reviewed_by, reviewed_at, created_at;

-- name: ReviewCommonsMedia :one
UPDATE commons_media_reviews
SET review_status = $2,
    review_notes = $3,
    reviewed_by = $4,
    reviewed_at = NOW()
WHERE id = $1
RETURNING id, title, category_norm, media_url, thumbnail_url, file_page_url,
    license_name, license_url, attribution, source_revision, identity_scope,
    review_status, review_notes, reviewed_by, reviewed_at, created_at;

-- name: ListCommonsMediaReviews :many
SELECT id, title, category_norm, media_url, thumbnail_url, file_page_url,
    license_name, license_url, attribution, source_revision, identity_scope,
    review_status, review_notes, reviewed_by, reviewed_at, created_at
FROM commons_media_reviews
ORDER BY created_at DESC, id DESC;

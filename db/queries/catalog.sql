-- name: FindPartByArticleForVehicle :one
SELECT legacy_article_id, article_number, generic_article_desc, brand_name, category_name, assembly_group_node_id
FROM hk_parts_cache
WHERE legacy_article_id = $1
  AND linking_target_id = $2
LIMIT 1;

-- name: FindPartByArticleAnyVehicle :one
SELECT legacy_article_id, article_number, generic_article_desc, brand_name, category_name, assembly_group_node_id
FROM hk_parts_cache
WHERE legacy_article_id = $1
LIMIT 1;

-- name: CountPartsByVehicleCategory :one
SELECT COUNT(DISTINCT legacy_article_id)::int
FROM hk_parts_cache
WHERE linking_target_id = $1
  AND ($2::text = '' OR COALESCE(category_name, '') ILIKE '%' || $2 || '%');

-- name: ListPartsByVehicleCategory :many
SELECT DISTINCT ON (legacy_article_id)
    legacy_article_id,
    article_number,
    generic_article_desc,
    brand_name,
    category_name,
    assembly_group_node_id
FROM hk_parts_cache
WHERE linking_target_id = $1
  AND ($2::text = '' OR COALESCE(category_name, '') ILIKE '%' || $2 || '%')
ORDER BY legacy_article_id, category_name, brand_name
LIMIT $3 OFFSET $4;

-- name: ResolveLinkageTargets :many
SELECT DISTINCT
    linkage_target_id,
    description,
    begin_year_month,
    end_year_month,
    fuel_type,
    capacity_cc,
    horse_power_from
FROM vehicle_lookup
WHERE nhtsa_make = $1
  AND nhtsa_model = $2
  AND $3 BETWEEN year_from AND year_to
ORDER BY begin_year_month DESC, linkage_target_id;

-- name: BestLinkageTargetCandidates :many
SELECT
    v.linkage_target_id,
    v.description,
    v.begin_year_month,
    v.end_year_month,
    v.fuel_type,
    v.capacity_cc,
    v.horse_power_from,
    COUNT(*)::int AS part_count
FROM vehicle_lookup v
JOIN hk_parts_cache hk ON hk.linking_target_id = v.linkage_target_id
WHERE v.nhtsa_make = $1
  AND v.nhtsa_model = $2
  AND $3 BETWEEN v.year_from AND v.year_to
GROUP BY
    v.linkage_target_id,
    v.description,
    v.begin_year_month,
    v.end_year_month,
    v.fuel_type,
    v.capacity_cc,
    v.horse_power_from
ORDER BY part_count DESC, v.begin_year_month DESC
LIMIT 20;

-- name: ReverseVehiclesByArticle :many
SELECT DISTINCT
    linking_target_id,
    vehicle_desc,
    begin_year_month,
    end_year_month,
    fuel_type,
    capacity_cc,
    horse_power_from,
    model_name,
    CASE manu_id WHEN 183 THEN 'HYUNDAI' WHEN 184 THEN 'KIA' ELSE '' END AS make_name
FROM hk_parts_cache
WHERE legacy_article_id = $1
ORDER BY make_name, model_name
LIMIT $2;

-- name: ListModelsByMake :many
SELECT
    nhtsa_model,
    MIN(year_from)::int AS year_from,
    MAX(year_to)::int AS year_to,
    COUNT(DISTINCT linkage_target_id)::int AS variants
FROM vehicle_lookup
WHERE nhtsa_make = $1
GROUP BY nhtsa_model
ORDER BY nhtsa_model;

-- name: ListVehicleVariantsByMakeModel :many
SELECT
    linkage_target_id,
    description,
    COALESCE(fuel_type, '') AS fuel_type,
    COALESCE(capacity_cc, 0)::int AS capacity_cc,
    COALESCE(horse_power_from, 0)::int AS horse_power_from,
    year_from,
    year_to
FROM vehicle_lookup
WHERE nhtsa_make = $1
  AND nhtsa_model = $2
ORDER BY year_from DESC, capacity_cc, linkage_target_id;

-- name: ListAssemblyGroupsByVehicle :many
SELECT
    assembly_group_node_id,
    COALESCE(category_name, '') AS category_name,
    COUNT(DISTINCT legacy_article_id)::int AS part_count
FROM hk_parts_cache
WHERE linking_target_id = $1
  AND assembly_group_node_id > 0
GROUP BY assembly_group_node_id, category_name
ORDER BY category_name;

-- name: ListPartsByGroup :many
SELECT DISTINCT ON (legacy_article_id)
    legacy_article_id,
    article_number,
    generic_article_desc,
    brand_name,
    category_name,
    assembly_group_node_id
FROM hk_parts_cache
WHERE linking_target_id = $1
  AND ($2::int = 0 OR assembly_group_node_id = $2)
ORDER BY legacy_article_id, category_name, generic_article_desc, brand_name;

-- name: SearchOEMByNormalized :many
SELECT
    raw_number,
    normalized,
    legacy_article_id,
    source_table,
    mfr_name,
    brand_name,
    article_number,
    description
FROM oem_search_index
WHERE normalized = $1
LIMIT $2;

-- name: SearchOEMPrefix :many
SELECT
    raw_number,
    normalized,
    legacy_article_id,
    source_table,
    mfr_name,
    brand_name,
    article_number,
    description
FROM oem_search_index
WHERE normalized LIKE $1
LIMIT $2;

-- name: ListOEMRawNumbersForArticle :many
SELECT DISTINCT raw_number
FROM oem_search_index
WHERE legacy_article_id = $1
LIMIT 20;

-- name: ListSubstitutionLinksForArticle :many
WITH article_numbers AS (
    SELECT DISTINCT UPPER(article_number) AS article_number
    FROM hk_parts_cache
    WHERE legacy_article_id = $1
      AND article_number IS NOT NULL
      AND article_number != ''
),
raw_links AS (
    SELECT
        links.to_part_number AS article_number,
        links.description,
        'reported_replacement' AS direction,
        links.source_key,
        links.source_label,
        links.source_detail,
        links.source_warning,
        links.confidence
    FROM substitution_links links
    JOIN article_numbers current_part
      ON UPPER(links.from_part_number) = current_part.article_number
    UNION ALL
    SELECT
        links.from_part_number AS article_number,
        links.description,
        'reported_predecessor' AS direction,
        links.source_key,
        links.source_label,
        links.source_detail,
        links.source_warning,
        links.confidence
    FROM substitution_links links
    JOIN article_numbers current_part
      ON UPPER(links.to_part_number) = current_part.article_number
)
SELECT
    article_number,
    COALESCE(MIN(description), '')::text AS description,
    (
      CASE
        WHEN COUNT(DISTINCT direction) > 1 THEN 'reported_related'
        ELSE MIN(direction)
      END
    )::text AS direction,
    source_key,
    COALESCE(MIN(source_label), '')::text AS source_label,
    COALESCE(MIN(source_detail), '')::text AS source_detail,
    COALESCE(MIN(source_warning), '')::text AS source_warning,
    MIN(confidence)::double precision AS confidence
FROM raw_links
GROUP BY article_number, source_key
ORDER BY article_number
LIMIT 20;

-- name: FindOEMReferencesForArticle :many
SELECT
    raw_number,
    mfr_name,
    article_number,
    description
FROM oem_search_index
WHERE legacy_article_id = $1
LIMIT 50;

-- name: FindVehiclesForArticle :many
SELECT DISTINCT
    linking_target_id,
    vehicle_desc,
    begin_year_month,
    end_year_month,
    fuel_type,
    capacity_cc,
    horse_power_from,
    CASE manu_id WHEN 183 THEN 'HYUNDAI' WHEN 184 THEN 'KIA' ELSE '' END AS make_name
FROM hk_parts_cache
WHERE legacy_article_id = $1
ORDER BY make_name, vehicle_desc
LIMIT $2;

-- name: LookupAlternativeDescription :one
SELECT generic_article_desc
FROM hk_parts_cache
WHERE legacy_article_id = $1
LIMIT 1;

-- name: FindAlternativesForArticleVehicle :many
SELECT DISTINCT ON (legacy_article_id)
    legacy_article_id,
    article_number,
    generic_article_desc,
    brand_name,
    category_name,
    assembly_group_node_id
FROM hk_parts_cache
WHERE generic_article_desc = $1
  AND linking_target_id = $2
  AND legacy_article_id != $3
ORDER BY legacy_article_id, brand_name
LIMIT $4;

-- name: FindAlternativesForArticleAnyVehicle :many
SELECT DISTINCT ON (legacy_article_id)
    legacy_article_id,
    article_number,
    generic_article_desc,
    brand_name,
    category_name,
    assembly_group_node_id
FROM hk_parts_cache
WHERE generic_article_desc = $1
  AND legacy_article_id != $2
ORDER BY legacy_article_id, brand_name
LIMIT $3;

-- name: CategoryTreeLeavesByVehicle :many
SELECT
    COALESCE(category_name, '') AS category_name,
    assembly_group_node_id,
    COUNT(DISTINCT legacy_article_id)::int AS part_count
FROM hk_parts_cache
WHERE linking_target_id = $1
  AND category_name IS NOT NULL
  AND category_name != ''
GROUP BY category_name, assembly_group_node_id
ORDER BY category_name;

-- name: SearchByArticleNumber :many
SELECT DISTINCT ON (legacy_article_id)
    legacy_article_id,
    article_number,
    generic_article_desc,
    brand_name,
    category_name,
    assembly_group_node_id,
    capacity_cc
FROM hk_parts_cache
WHERE UPPER(article_number) = UPPER($1)
ORDER BY legacy_article_id
LIMIT $2;

-- name: CountVehicleSearchParts :one
SELECT COUNT(DISTINCT legacy_article_id)::int
FROM hk_parts_cache
WHERE linking_target_id = $1
  AND ($2::text = '' OR (COALESCE(generic_article_desc, '') ILIKE '%' || $2 || '%' OR COALESCE(category_name, '') ILIKE '%' || $2 || '%'))
  AND (
    $3::text = ''
    OR (
      NOT EXISTS (
        SELECT 1
        FROM unnest(regexp_split_to_array($3, '\s+')) AS query_token(token)
        WHERE concat_ws(' ', generic_article_desc, article_number, category_name) NOT ILIKE '%' || query_token.token || '%'
      )
      AND EXISTS (
        SELECT 1
        FROM unnest(regexp_split_to_array($3, '\s+')) AS query_token(token)
        WHERE concat_ws(' ', generic_article_desc, article_number) ILIKE '%' || query_token.token || '%'
      )
    )
  );

-- name: ListVehicleSearchCategories :many
SELECT DISTINCT generic_article_desc
FROM hk_parts_cache
WHERE linking_target_id = $1
  AND generic_article_desc IS NOT NULL
  AND generic_article_desc != ''
ORDER BY generic_article_desc;

-- name: ListVehicleSearchParts :many
SELECT DISTINCT ON (legacy_article_id)
    legacy_article_id,
    article_number,
    generic_article_desc,
    brand_name,
    category_name,
    assembly_group_node_id,
    capacity_cc,
    fuel_type
FROM hk_parts_cache
WHERE linking_target_id = $1
  AND ($2::text = '' OR (COALESCE(generic_article_desc, '') ILIKE '%' || $2 || '%' OR COALESCE(category_name, '') ILIKE '%' || $2 || '%'))
  AND (
    $3::text = ''
    OR (
      NOT EXISTS (
        SELECT 1
        FROM unnest(regexp_split_to_array($3, '\s+')) AS query_token(token)
        WHERE concat_ws(' ', generic_article_desc, article_number, category_name) NOT ILIKE '%' || query_token.token || '%'
      )
      AND EXISTS (
        SELECT 1
        FROM unnest(regexp_split_to_array($3, '\s+')) AS query_token(token)
        WHERE concat_ws(' ', generic_article_desc, article_number) ILIKE '%' || query_token.token || '%'
      )
    )
  )
ORDER BY legacy_article_id, generic_article_desc, brand_name
LIMIT $4 OFFSET $5;

-- name: SearchByText :many
SELECT DISTINCT ON (legacy_article_id)
    legacy_article_id,
    article_number,
    generic_article_desc,
    brand_name,
    category_name,
    assembly_group_node_id,
    capacity_cc
FROM hk_parts_cache
WHERE
  NOT EXISTS (
    SELECT 1
    FROM unnest(regexp_split_to_array($1, '\s+')) AS query_token(token)
    WHERE concat_ws(' ', generic_article_desc, article_number, category_name) NOT ILIKE '%' || query_token.token || '%'
  )
  AND EXISTS (
    SELECT 1
    FROM unnest(regexp_split_to_array($1, '\s+')) AS query_token(token)
    WHERE concat_ws(' ', generic_article_desc, article_number) ILIKE '%' || query_token.token || '%'
  )
ORDER BY legacy_article_id, generic_article_desc, brand_name
LIMIT $2 OFFSET $3;

-- name: CheckPartFitsVehicle :one
SELECT EXISTS (
    SELECT 1
    FROM hk_parts_cache
    WHERE linking_target_id = $1
      AND legacy_article_id = $2
);

-- name: ListCategoryInfos :many
SELECT
    generic_article_desc,
    COUNT(DISTINCT legacy_article_id)::int AS part_count
FROM hk_parts_cache
WHERE linking_target_id = $1
  AND generic_article_desc IS NOT NULL
  AND generic_article_desc != ''
GROUP BY generic_article_desc
ORDER BY part_count DESC;

-- name: FindPlatformSiblingsForHyundai :many
SELECT 'KIA' AS sibling_make, kia_model AS sibling_model, platform_code
FROM hk_platform_map
WHERE UPPER(hyundai_model) = UPPER($1);

-- name: FindPlatformSiblingsForKia :many
SELECT 'HYUNDAI' AS sibling_make, hyundai_model AS sibling_model, platform_code
FROM hk_platform_map
WHERE UPPER(kia_model) = UPPER($1);

-- name: CountSourcesByRecommendation :many
SELECT recommendation, COUNT(*)::int AS source_count
FROM external_sources
GROUP BY recommendation;

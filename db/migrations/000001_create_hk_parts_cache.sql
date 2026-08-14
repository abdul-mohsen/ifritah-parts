CREATE TABLE IF NOT EXISTS hk_parts_cache (
    linking_target_id      INTEGER NOT NULL,
    legacy_article_id      INTEGER NOT NULL,
    assembly_group_node_id INTEGER NOT NULL DEFAULT 0,
    article_number         TEXT,
    generic_article_desc   TEXT,
    data_supplier_id       INTEGER,
    brand_name             TEXT,
    category_name          TEXT,
    vehicle_desc           TEXT,
    manu_id                INTEGER,
    model_id               INTEGER,
    model_name             TEXT,
    begin_year_month       TEXT,
    end_year_month         TEXT,
    fuel_type              TEXT,
    capacity_cc            INTEGER,
    horse_power_from       INTEGER,
    PRIMARY KEY (linking_target_id, legacy_article_id, assembly_group_node_id)
);

CREATE INDEX IF NOT EXISTS idx_hk_parts_cache_article
    ON hk_parts_cache (legacy_article_id);

CREATE INDEX IF NOT EXISTS idx_hk_parts_cache_model
    ON hk_parts_cache (manu_id, model_id);

CREATE INDEX IF NOT EXISTS idx_hk_parts_cache_brand
    ON hk_parts_cache (data_supplier_id);

CREATE INDEX IF NOT EXISTS idx_hk_parts_cache_category
    ON hk_parts_cache (assembly_group_node_id);

CREATE INDEX IF NOT EXISTS idx_hk_parts_cache_article_number
    ON hk_parts_cache (article_number);

CREATE INDEX IF NOT EXISTS idx_hk_parts_cache_vehicle
    ON hk_parts_cache (linking_target_id);

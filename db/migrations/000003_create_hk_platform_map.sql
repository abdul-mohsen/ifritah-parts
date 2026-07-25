CREATE TABLE IF NOT EXISTS hk_platform_map (
    id             BIGSERIAL PRIMARY KEY,
    platform_code  TEXT NOT NULL,
    hyundai_model  TEXT NOT NULL,
    kia_model      TEXT NOT NULL,
    gen_start_year INTEGER,
    gen_end_year   INTEGER,
    notes          TEXT
);

CREATE INDEX IF NOT EXISTS idx_hk_platform_map_hyundai
    ON hk_platform_map (hyundai_model);

CREATE INDEX IF NOT EXISTS idx_hk_platform_map_kia
    ON hk_platform_map (kia_model);

CREATE INDEX IF NOT EXISTS idx_hk_platform_map_platform
    ON hk_platform_map (platform_code);

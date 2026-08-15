CREATE TABLE IF NOT EXISTS nhtsa_tecdoc_bridge (
    id                BIGSERIAL PRIMARY KEY,
    nhtsa_make        TEXT NOT NULL,
    nhtsa_model       TEXT NOT NULL,
    year_from         INTEGER NOT NULL,
    year_to           INTEGER NOT NULL,
    tecdoc_manu_id    INTEGER NOT NULL,
    tecdoc_model_id   INTEGER NOT NULL,
    tecdoc_model_name TEXT
);

CREATE INDEX IF NOT EXISTS idx_nhtsa_tecdoc_bridge_lookup
    ON nhtsa_tecdoc_bridge (nhtsa_make, nhtsa_model, year_from, year_to);

CREATE INDEX IF NOT EXISTS idx_nhtsa_tecdoc_bridge_tecdoc
    ON nhtsa_tecdoc_bridge (tecdoc_manu_id, tecdoc_model_id);

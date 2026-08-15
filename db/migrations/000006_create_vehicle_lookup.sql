CREATE TABLE IF NOT EXISTS vehicle_lookup (
    nhtsa_make        TEXT NOT NULL,
    nhtsa_model       TEXT NOT NULL,
    year_from         INTEGER NOT NULL,
    year_to           INTEGER NOT NULL,
    linkage_target_id INTEGER NOT NULL PRIMARY KEY,
    description       TEXT NOT NULL,
    begin_year_month  TEXT,
    end_year_month    TEXT,
    fuel_type         TEXT,
    capacity_cc       INTEGER,
    horse_power_from  INTEGER
);

CREATE INDEX IF NOT EXISTS idx_vehicle_lookup_lookup
    ON vehicle_lookup (nhtsa_make, nhtsa_model, year_from, year_to);

CREATE INDEX IF NOT EXISTS idx_vehicle_lookup_model
    ON vehicle_lookup (nhtsa_make, nhtsa_model);

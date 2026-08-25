-- ============================================================================
-- 000015 - related_parts co-occurrence table
-- ============================================================================
-- M5.S3.T1 - "if you're buying an oil filter, also change air filter + cabin
-- filter + spark plugs" mapping. Powers /api/parts/related.
--
-- Source of truth: hard-coded Hyundai/Kia service-interval schedules
-- (60,000 km / 90,000 km / 120,000 km bundles). Categories, not specific
-- OEMs — so a query for ANY oil filter OEM decodes via the prefixMap and
-- gets the "also change" siblings.
--
-- User-cart co-occurrence is a future evidence source once M6.S2 lands
-- the feedback loop; for now the schedule seed is enough to cover the
-- top 20 service categories.
-- ============================================================================

CREATE TABLE IF NOT EXISTS related_parts (
	source_category  TEXT NOT NULL,
	related_category TEXT NOT NULL,
	correlation      REAL NOT NULL,
	evidence_source  TEXT NOT NULL,      -- 'service_60k' / 'service_90k' / 'service_120k' / 'cart' / 'seasonal'
	priority         INTEGER NOT NULL DEFAULT 50,  -- 0-100; higher = surface first
	created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (source_category, related_category, evidence_source)
);

CREATE INDEX IF NOT EXISTS idx_related_parts_source
	ON related_parts (source_category, priority DESC);

-- ── Seed: 60,000 km service bundle ────────────────────────────────────────
-- Every filter cross-references the other two; plus spark plugs + coolant.
INSERT INTO related_parts (source_category, related_category, correlation, evidence_source, priority) VALUES
	('Oil Filter',       'Air Filter',                 0.92, 'service_60k', 90),
	('Oil Filter',       'Cabin Air Filter',           0.85, 'service_60k', 80),
	('Oil Filter',       'Spark Plug & Ignition Coil', 0.60, 'service_60k', 70),
	('Air Filter',       'Oil Filter',                 0.92, 'service_60k', 90),
	('Air Filter',       'Cabin Air Filter',           0.85, 'service_60k', 80),
	('Cabin Air Filter', 'Oil Filter',                 0.85, 'service_60k', 85),
	('Cabin Air Filter', 'Air Filter',                 0.85, 'service_60k', 85),
	('Spark Plug & Ignition Coil', 'Oil Filter',       0.60, 'service_60k', 60)
ON CONFLICT DO NOTHING;

-- ── Seed: 90,000 km service bundle ────────────────────────────────────────
-- Adds fuel filter + brake fluid + coolant.
INSERT INTO related_parts (source_category, related_category, correlation, evidence_source, priority) VALUES
	('Fuel Pump',                  'Oil Filter',                    0.55, 'service_90k', 55),
	('Fuel Pump',                  'Air Filter',                    0.55, 'service_90k', 55),
	('Fuel Pump',                  'Spark Plug & Ignition Coil',    0.70, 'service_90k', 65),
	('Spark Plug & Ignition Coil', 'Oxygen Sensor',                 0.55, 'service_90k', 55),
	('Radiator',                   'Water Pump',                    0.65, 'service_90k', 65),
	('Water Pump',                 'Radiator',                      0.65, 'service_90k', 65),
	('Water Pump',                 'Thermostat & Housing',          0.75, 'service_90k', 75),
	('Thermostat & Housing',       'Water Pump',                    0.75, 'service_90k', 75)
ON CONFLICT DO NOTHING;

-- ── Seed: brake service (any-mileage triggered) ───────────────────────────
INSERT INTO related_parts (source_category, related_category, correlation, evidence_source, priority) VALUES
	('Front Brake Pad / Disc',  'Front Brake Caliper',      0.60, 'service_brake', 70),
	('Front Brake Pad / Disc',  'Rear Brake / Drum',        0.55, 'service_brake', 65),
	('Front Brake Pad / Disc',  'Brake Master Cylinder',    0.35, 'service_brake', 40),
	('Rear Brake / Drum',       'Front Brake Pad / Disc',   0.55, 'service_brake', 65),
	('Rear Brake / Drum',       'Rear Brake Caliper',       0.60, 'service_brake', 70)
ON CONFLICT DO NOTHING;

-- ── Seed: suspension service ─────────────────────────────────────────────
INSERT INTO related_parts (source_category, related_category, correlation, evidence_source, priority) VALUES
	('Shock Absorber (Front)',  'Shock Absorber (Rear)',    0.75, 'service_susp', 80),
	('Shock Absorber (Front)',  'Front Suspension',         0.65, 'service_susp', 70),
	('Shock Absorber (Rear)',   'Shock Absorber (Front)',   0.75, 'service_susp', 80),
	('Shock Absorber (Rear)',   'Rear Suspension',          0.65, 'service_susp', 70),
	('Front Suspension',        'Tie Rod',                  0.55, 'service_susp', 55),
	('Front Suspension',        'Steering Column & Gear',   0.35, 'service_susp', 35),
	('Tie Rod',                 'Front Suspension',         0.55, 'service_susp', 55)
ON CONFLICT DO NOTHING;

-- ── Seed: timing service (every 90k km on Hyundai/Kia CVVT + GDI) ────────
INSERT INTO related_parts (source_category, related_category, correlation, evidence_source, priority) VALUES
	('Camshaft & Timing',   'Cylinder Head & Valvetrain',  0.55, 'service_timing', 55),
	('Camshaft & Timing',   'Water Pump',                  0.75, 'service_timing', 75),
	('Cylinder Head',       'Cylinder Head & Valvetrain',  0.80, 'service_timing', 80)
ON CONFLICT DO NOTHING;

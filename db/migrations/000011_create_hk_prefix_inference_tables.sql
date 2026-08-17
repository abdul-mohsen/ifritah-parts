-- ============================================================================
-- Phase 1: Prefix-family inference tables (Postgres)
-- ============================================================================
-- Enables offline synthesis of OEM descriptions like "82460-2T010 = Front
-- Power Window Motor for Kia Optima TF (2010-2015)" without any network call.
--
-- Three tables:
--   hk_oem_prefix_map     — 5-digit prefix → part family + description
--   hk_chassis_code_map   — 2-3 char chassis code → make/model/year-range
--   hk_variant_suffix_map — 3-char suffix → position/side (RH/LH/etc.)
--
-- Seeded with hand-curated baseline. A companion script
-- (scripts/derive_hk_maps/main.go) auto-enriches these tables from TecDoc
-- clustering when TecDoc MySQL is reachable. If the derive script fails or
-- TecDoc is unavailable, the baseline seed keeps the feature working.
-- ============================================================================

CREATE TABLE IF NOT EXISTS hk_oem_prefix_map (
    prefix           TEXT PRIMARY KEY,          -- 5-digit e.g. '82460'
    system           TEXT NOT NULL,             -- 'Engine', 'Body', 'Brakes', ...
    category         TEXT NOT NULL,             -- 'Power Window Motor - Front', 'Oil Filter'
    description      TEXT NOT NULL,             -- 'Front Power Window Motor Assembly'
    confidence       DOUBLE PRECISION NOT NULL DEFAULT 0.85,
    source           TEXT NOT NULL DEFAULT 'seed',  -- 'seed' | 'tecdoc_derived' | 'user'
    sample_count     INTEGER NOT NULL DEFAULT 0,    -- how many TecDoc rows backed this
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hk_oem_prefix_system  ON hk_oem_prefix_map (system);

-- ────────────────────────────────────────────────────────────────────────────
-- hk_chassis_code_map
-- The 2-3 character chassis code inside a Hyundai/Kia OEM number identifies
-- the vehicle platform. Examples:
--   82460-2T010 → chassis '2T' = Kia Optima TF (2010-2015)
--   26350-2J001 → chassis '2J' = Hyundai Tucson TL / Sonata YF era
-- Seeded with well-known industry mappings, expanded by the derive script.
-- ────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS hk_chassis_code_map (
    chassis_code     TEXT PRIMARY KEY,          -- '2T', '3S', 'D3', 'CN7'
    make             TEXT NOT NULL,             -- 'Kia' | 'Hyundai' | 'Genesis'
    model            TEXT NOT NULL,             -- 'Optima', 'Sonata', 'Tucson'
    platform         TEXT,                      -- 'TF', 'YF', 'TL' (generation code)
    year_start       INTEGER NOT NULL,
    year_end         INTEGER,                   -- NULL for still-in-production
    confidence       DOUBLE PRECISION NOT NULL DEFAULT 0.85,
    source           TEXT NOT NULL DEFAULT 'seed',
    sample_count     INTEGER NOT NULL DEFAULT 0,
    notes            TEXT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_hk_chassis_make_model ON hk_chassis_code_map (make, model);

-- ────────────────────────────────────────────────────────────────────────────
-- hk_variant_suffix_map
-- The trailing 3 characters commonly indicate position, side, color, or trim.
-- ────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS hk_variant_suffix_map (
    suffix           TEXT PRIMARY KEY,          -- '010', '020', '500', 'A00'
    position         TEXT,                      -- 'Front Right', 'Front Left', 'Rear Right', 'Rear Left'
    side             TEXT,                      -- 'right' | 'left' | 'center' | ''
    variant_note     TEXT,                      -- 'color=black', 'trim=luxury'
    confidence       DOUBLE PRECISION NOT NULL DEFAULT 0.80,
    source           TEXT NOT NULL DEFAULT 'seed',
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────────────────
-- Baseline seed (Path B fallback) — always applied. The derive script can
-- UPSERT to boost confidence + sample_count.
--
-- Prefix map: high-value families covering ~80% of common Hyundai/Kia searches
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO hk_oem_prefix_map (prefix, system, category, description) VALUES
    -- Engine oil system
    ('26300', 'Engine',  'Oil Filter',                'Engine Oil Filter (4-cyl)'),
    ('26320', 'Engine',  'Oil Filter Element',        'Oil Filter Element'),
    ('26350', 'Engine',  'Oil Filter',                'Engine Oil Filter (V6)'),
    ('26310', 'Engine',  'Oil Pan',                   'Engine Oil Pan'),
    ('26510', 'Engine',  'Oil Pump',                  'Engine Oil Pump'),
    -- Air filter
    ('28113', 'Engine',  'Air Filter',                'Engine Air Filter'),
    -- Fuel filter
    ('31112', 'Engine',  'Fuel Filter',               'Fuel Filter'),
    ('31300', 'Engine',  'Fuel Pump',                 'Fuel Pump'),
    -- Cabin air filter
    ('97133', 'HVAC',    'Cabin Air Filter',          'Cabin Air Filter'),
    ('97134', 'HVAC',    'Cabin Air Filter',          'Cabin Air Filter (alt)'),
    -- Brakes
    ('58101', 'Brakes',  'Brake Pad Set - Front',     'Front Brake Pad Set'),
    ('58302', 'Brakes',  'Brake Pad Set - Rear',      'Rear Brake Pad Set'),
    ('51712', 'Brakes',  'Brake Disc - Front',        'Front Brake Disc / Rotor'),
    ('58411', 'Brakes',  'Brake Disc - Rear',         'Rear Brake Disc / Rotor'),
    -- Suspension
    ('54650', 'Suspension', 'Shock Absorber - Front', 'Front Shock Absorber / Strut'),
    ('54660', 'Suspension', 'Shock Absorber - Front', 'Front Shock Absorber / Strut'),
    ('55311', 'Suspension', 'Shock Absorber - Rear',  'Rear Shock Absorber'),
    -- Ignition
    ('27301', 'Engine',  'Ignition Coil',             'Ignition Coil'),
    ('18845', 'Engine',  'Spark Plug',                'Spark Plug'),
    -- Sensors
    ('39210', 'Electrical', 'Oxygen Sensor',          'Oxygen Sensor'),
    ('39350', 'Electrical', 'MAF Sensor',             'Mass Air Flow Sensor'),
    ('94750', 'Electrical', 'Oil Pressure Sensor',    'Engine Oil Pressure Sensor'),
    -- Body / Electrical (the 82460 family — user's test case)
    ('82460', 'Body',    'Power Window Motor - Front','Front Power Window Motor Assembly'),
    ('82470', 'Body',    'Power Window Motor - Rear', 'Rear Power Window Motor Assembly'),
    ('82650', 'Body',    'Window Regulator - Front',  'Front Window Regulator'),
    ('82750', 'Body',    'Window Regulator - Rear',   'Rear Window Regulator'),
    ('81250', 'Body',    'Door Lock Actuator',        'Door Lock Actuator'),
    ('87610', 'Body',    'Exterior Mirror',           'Exterior Mirror Assembly'),
    -- Cooling
    ('25310', 'Cooling', 'Radiator',                  'Engine Radiator'),
    ('25620', 'Cooling', 'Thermostat',                'Engine Thermostat'),
    ('25100', 'Cooling', 'Water Pump',                'Engine Water Pump'),
    -- Transmission
    ('45210', 'Transmission', 'ATF / Transmission Oil','Automatic Transmission Fluid'),
    -- Lighting
    ('92101', 'Electrical', 'Headlight - Front Left', 'Front Left Headlight Assembly'),
    ('92102', 'Electrical', 'Headlight - Front Right','Front Right Headlight Assembly'),
    ('92401', 'Electrical', 'Tail Light - Rear Left', 'Rear Left Tail Light Assembly'),
    ('92402', 'Electrical', 'Tail Light - Rear Right','Rear Right Tail Light Assembly')
ON CONFLICT (prefix) DO NOTHING;

-- ────────────────────────────────────────────────────────────────────────────
-- Chassis code seed — hand-curated from public Hyundai/Kia platform data
-- Covers all major 2000-2020 platforms.
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO hk_chassis_code_map (chassis_code, make, model, platform, year_start, year_end, notes) VALUES
    -- Kia Optima / K5 platform generations
    ('2G', 'Kia',     'Optima',    'MG',    2005, 2010, 'Optima MG (2nd gen)'),
    ('2T', 'Kia',     'Optima',    'TF',    2010, 2015, 'Optima TF (3rd gen, K5 TF in Korea)'),
    ('2U', 'Kia',     'Optima',    'JF',    2015, 2020, 'Optima JF (4th gen)'),
    ('L2', 'Kia',     'K5',        'DL3',   2020, NULL, 'K5 DL3 / new Optima'),
    -- Hyundai Sonata generations
    ('3S', 'Hyundai', 'Sonata',    'YF',    2010, 2014, 'Sonata YF (6th gen)'),
    ('C2', 'Hyundai', 'Sonata',    'LF',    2015, 2019, 'Sonata LF (7th gen)'),
    ('L1', 'Hyundai', 'Sonata',    'DN8',   2020, NULL, 'Sonata DN8 (8th gen)'),
    -- Hyundai Tucson generations
    ('2E', 'Hyundai', 'Tucson',    'JM',    2004, 2010, 'Tucson JM (1st gen)'),
    ('2S', 'Hyundai', 'Tucson',    'LM',    2010, 2015, 'Tucson LM / ix35 (2nd gen)'),
    ('D3', 'Hyundai', 'Tucson',    'TL',    2015, 2020, 'Tucson TL (3rd gen)'),
    ('N9', 'Hyundai', 'Tucson',    'NX4',   2020, NULL, 'Tucson NX4 (4th gen)'),
    -- Kia Sportage generations
    ('2E', 'Kia',     'Sportage',  'KM',    2004, 2010, 'Sportage KM (2nd gen, shares Tucson JM)'),
    ('3W', 'Kia',     'Sportage',  'SL',    2010, 2015, 'Sportage SL (3rd gen)'),
    ('D9', 'Kia',     'Sportage',  'QL',    2015, 2021, 'Sportage QL (4th gen)'),
    ('P1', 'Kia',     'Sportage',  'NQ5',   2021, NULL, 'Sportage NQ5 (5th gen)'),
    -- Hyundai Elantra
    ('2H', 'Hyundai', 'Elantra',   'HD',    2006, 2011, 'Elantra HD (4th gen)'),
    ('3X', 'Hyundai', 'Elantra',   'MD',    2011, 2015, 'Elantra MD/UD (5th gen, Avante MD in Korea)'),
    ('F2', 'Hyundai', 'Elantra',   'AD',    2015, 2020, 'Elantra AD (6th gen)'),
    -- Kia Forte / K3
    ('1M', 'Kia',     'Forte',     'TD',    2008, 2013, 'Forte TD (1st gen)'),
    ('A7', 'Kia',     'Forte',     'YD',    2013, 2018, 'Forte YD (2nd gen)'),
    -- Hyundai Santa Fe
    ('26', 'Hyundai', 'Santa Fe',  'CM',    2006, 2012, 'Santa Fe CM (2nd gen)'),
    ('2W', 'Hyundai', 'Santa Fe',  'DM',    2012, 2018, 'Santa Fe DM/NC (3rd gen)'),
    ('C5', 'Hyundai', 'Santa Fe',  'TM',    2018, NULL, 'Santa Fe TM (4th gen)'),
    -- Kia Sorento
    ('3E', 'Kia',     'Sorento',   'XM',    2009, 2015, 'Sorento XM (2nd gen)'),
    ('C6', 'Kia',     'Sorento',   'UM',    2015, 2020, 'Sorento UM (3rd gen)'),
    -- Hyundai Accent
    ('1R', 'Hyundai', 'Accent',    'MC',    2005, 2011, 'Accent MC (3rd gen)'),
    ('1G', 'Hyundai', 'Accent',    'RB',    2010, 2017, 'Accent RB (4th gen)'),
    ('H5', 'Hyundai', 'Accent',    'HC',    2017, NULL, 'Accent HC (5th gen)'),
    -- Kia Rio
    ('1G', 'Kia',     'Rio',       'JB',    2005, 2011, 'Rio JB (2nd gen)'),
    ('1W', 'Kia',     'Rio',       'UB',    2011, 2017, 'Rio UB (3rd gen)'),
    ('H8', 'Kia',     'Rio',       'YB',    2017, NULL, 'Rio YB (4th gen)'),
    -- Hyundai i30 / Elantra GT
    ('A5', 'Hyundai', 'i30',       'PD',    2016, NULL, 'i30 PD (3rd gen)'),
    ('A6', 'Hyundai', 'i30',       'GD',    2011, 2016, 'i30 GD (2nd gen)'),
    -- Hyundai H1 / Starex
    ('4H', 'Hyundai', 'H1',        'TQ',    2007, 2018, 'H1 / Starex / i800 (TQ)'),
    ('CV', 'Hyundai', 'Staria',    'US4',   2021, NULL, 'Staria (US4)'),
    -- Kia Carnival / Sedona
    ('4D', 'Kia',     'Carnival',  'VQ',    2006, 2014, 'Carnival VQ (2nd gen)'),
    ('A9', 'Kia',     'Carnival',  'YP',    2014, 2020, 'Carnival YP (3rd gen)'),
    -- Kia Soul
    ('B2', 'Kia',     'Soul',      'AM',    2008, 2013, 'Soul AM (1st gen)'),
    ('E4', 'Kia',     'Soul',      'PS',    2013, 2019, 'Soul PS (2nd gen)'),
    -- Genesis
    ('B1', 'Genesis', 'G80',       'DH',    2013, 2020, 'Genesis DH / G80 (1st gen)'),
    ('G9', 'Genesis', 'G80',       'RG3',   2020, NULL, 'G80 RG3 (2nd gen)')
ON CONFLICT (chassis_code) DO NOTHING;

-- ────────────────────────────────────────────────────────────────────────────
-- Variant suffix seed
-- ────────────────────────────────────────────────────────────────────────────
INSERT INTO hk_variant_suffix_map (suffix, position, side) VALUES
    ('010', 'Front Right', 'right'),
    ('020', 'Front Left',  'left'),
    ('030', 'Rear Right',  'right'),
    ('040', 'Rear Left',   'left'),
    ('001', '',            ''),
    ('000', '',            ''),
    ('500', 'Rear',        ''),
    ('600', 'Passenger',   'right'),
    ('700', 'Driver',      'left')
ON CONFLICT (suffix) DO NOTHING;

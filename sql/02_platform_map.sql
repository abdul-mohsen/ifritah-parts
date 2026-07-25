-- ============================================================================
-- Phase 1, Step 1.2: Hyundai ↔ Kia platform mapping
-- Maps sibling vehicles that share the same platform/parts
-- Run against dev_ifritah
-- ============================================================================

DROP TABLE IF EXISTS hk_platform_map;

CREATE TABLE hk_platform_map (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    platform_code   VARCHAR(20) NOT NULL,
    hyundai_model   VARCHAR(100) NOT NULL,
    kia_model       VARCHAR(100) NOT NULL,
    gen_start_year  INT DEFAULT NULL,
    gen_end_year    INT DEFAULT NULL,
    notes           VARCHAR(300) DEFAULT NULL,

    INDEX idx_hyundai (hyundai_model),
    INDEX idx_kia (kia_model),
    INDEX idx_platform (platform_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO hk_platform_map (platform_code, hyundai_model, kia_model, gen_start_year, gen_end_year, notes) VALUES
-- SUVs / Crossovers
('NX4/NQ5',   'TUCSON',     'SPORTAGE',    2021, NULL, '4th gen Tucson / 5th gen Sportage, shared N platform'),
('TL/SL',     'TUCSON',     'SPORTAGE',    2015, 2021, '3rd gen Tucson / 4th gen Sportage'),
('JM/KM',     'TUCSON',     'SPORTAGE',    2004, 2010, '1st gen Tucson / 2nd gen Sportage'),
('SX2/MQ4a',  'VENUE',      'SELTOS',      2019, NULL, 'Subcompact crossover pair'),
('SU2/MQ4',   'CRETA',      'SELTOS',      2019, NULL, 'India/global subcompact'),
('MU/YN',     'SANTA FE',   'SORENTO',     2020, NULL, '5th gen Santa Fe / 4th gen Sorento'),
('TM/UM',     'SANTA FE',   'SORENTO',     2018, 2020, '4th gen Santa Fe / 3rd gen Sorento FL'),
('LX2/MQ4L',  'KONA',       'STONIC',      2017, NULL, 'Subcompact crossover'),
('GS/JA',     'PALISADE',   'TELLURIDE',   2019, NULL, 'Full-size 3-row SUV pair'),

-- Sedans / Compact
('CN7/DL3',   'ELANTRA',    'FORTE',       2020, NULL, '7th gen Elantra / 4th gen Forte (K3)'),
('AD/BD',     'ELANTRA',    'FORTE',       2015, 2020, '6th gen Elantra / 3rd gen Forte'),
('DN8/DL3S',  'SONATA',     'K5',          2019, NULL, '8th gen Sonata / 1st gen K5 (Optima)'),
('LF/JF',     'SONATA',     'OPTIMA',      2014, 2019, '7th gen Sonata / 4th gen Optima'),
('RG3/GL3',   'GRANDEUR',   'K8',          2022, NULL, 'Full-size sedan pair'),
('IG/YD',     'GRANDEUR',   'CADENZA',     2016, 2022, 'Full-size sedan pair'),
('IK/CK',     'ACCENT',     'RIO',         2017, NULL, '5th gen Accent / 4th gen Rio'),
('HCR/YB',    'ACCENT',     'RIO',         2011, 2017, '4th gen Accent / 3rd gen Rio'),

-- MPVs / Vans
('KM/VQ',     'STARIA',     'CARNIVAL',    2021, NULL, 'MPV pair, shared platform'),
('US4/KA4',   'STAREX',     'CARNIVAL',    2014, 2021, 'Previous gen MPV'),

-- Electric
('NE/CV',     'IONIQ 5',    'EV6',         2021, NULL, 'E-GMP platform, shared EV'),
('CE/MV',     'IONIQ 6',    'EV6',         2022, NULL, 'E-GMP sedan vs crossover');

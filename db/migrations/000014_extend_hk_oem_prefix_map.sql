-- ============================================================================
-- Postgres migration 000014 — extend hk_oem_prefix_map seed
-- ============================================================================
-- Adds ~60 more Hyundai/Kia 5-digit OEM prefixes to hk_oem_prefix_map.
--
-- The audit in docs/reports/2026-08-19-post-pr14-data-quality.md §3-4
-- measured this: on 1,190 real Hyundai/Kia OEMs from the kiapartsnow
-- sitemap, only 60 (5%) had a prefix seeded in the original migration
-- 000011. The other 95% fell through to the coarse 2/3-digit map in
-- oem_prefix.go, which gave poor F1 (0.11 on the coarse slice).
--
-- Extending the seed to cover the top prefixes in the real HK catalog
-- lifts combined-mode F1 from 0.54 (measured 2026-08-19) toward the
-- 0.97 ceiling we see on already-seeded families.
--
-- Prefixes below were chosen from the top of the sitemap-derived HK OEM
-- frequency distribution captured during the 2026-08-19 audit. See:
--   C:\Users\ALMAAB~1\AppData\Local\Temp\opencode\raw-hk-oems.txt (15,108
--   unique 5+5 OEMs across 3 sitemap files).
--
-- All rows use ON CONFLICT (prefix) DO NOTHING so re-runs are idempotent
-- and existing tecdoc_derived rows (higher confidence) are not overwritten.
-- ============================================================================

INSERT INTO hk_oem_prefix_map (prefix, system, category, description) VALUES
    -- ── Wiring & Electrical Harnesses ─────────────────────────────────────
    ('91950', 'Electrical', 'Wiring Harness',                 'Wire Harness (main body)'),
    ('91600', 'Electrical', 'Wiring Harness',                 'Wire Harness (front body)'),
    ('91610', 'Electrical', 'Wiring Harness',                 'Wire Harness (engine bay)'),
    ('91880', 'Electrical', 'Wiring Harness',                 'Wire Harness (roof)'),
    ('91890', 'Electrical', 'Wiring Harness',                 'Wire Harness (rear)'),
    ('91931', 'Electrical', 'Junction Box',                   'Junction Box'),
    ('91951', 'Electrical', 'Junction Box',                   'Junction Box (multi-use)'),
    ('91952', 'Electrical', 'Junction Box',                   'Junction Box (variant)'),
    ('91961', 'Electrical', 'Fuse Box',                       'Fuse Box'),

    -- ── Sensors ───────────────────────────────────────────────────────────
    ('95400', 'Electrical', 'Body Control Module',            'BCM (Body Control Module)'),
    ('95440', 'Electrical', 'Body Control Module',            'BCM Junction Assembly'),
    ('39220', 'Electrical', 'Coolant Temperature Sensor',     'ECT Sensor'),
    ('39250', 'Electrical', 'MAP Sensor',                     'Manifold Absolute Pressure Sensor'),
    ('39280', 'Electrical', 'TPS Sensor',                     'Throttle Position Sensor'),
    ('96230', 'Electrical', 'Display / Audio Head Unit',      'AVN / Head Unit'),

    -- ── Ignition ──────────────────────────────────────────────────────────
    ('32450', 'Engine', 'Ignition Switch',                    'Ignition Switch Key'),

    -- ── Body / Mirrors / Trim ─────────────────────────────────────────────
    ('87620', 'Body', 'Exterior Mirror - Right',              'Right Exterior Mirror Assembly'),
    ('87630', 'Body', 'Exterior Mirror',                      'Exterior Mirror (variant)'),
    ('86110', 'Body', 'Emblem / Badge',                       'Emblem / Badge Front'),
    ('86350', 'Body', 'Bumper Cover Trim',                    'Bumper Cover Trim'),
    ('05203', 'Body', 'Emblem / Ornament',                    'Ornament Emblem'),
    ('81905', 'Body', 'Weatherstrip',                         'Door Weatherstrip'),
    ('82397', 'Body', 'Door Belt Weatherstrip',               'Front Door Belt Weatherstrip'),
    ('82650', 'Body', 'Front Door Handle',                    'Front Outside Door Handle'),
    ('82750', 'Body', 'Rear Door Handle',                     'Rear Outside Door Handle'),

    -- ── Interior ──────────────────────────────────────────────────────────
    ('88500', 'Interior', 'Seat Belt - Front',                'Front Seat Belt Assembly'),
    ('88600', 'Interior', 'Seat Belt - Rear',                 'Rear Seat Belt Assembly'),
    ('84540', 'Interior', 'Console Trim',                     'Console Trim'),

    -- ── Steering ──────────────────────────────────────────────────────────
    ('56310', 'Steering', 'Power Steering Pump',              'Power Steering Pump'),
    ('56390', 'Steering', 'Steering Column',                  'Steering Column Assembly'),
    ('56512', 'Steering', 'Steering Wheel',                   'Steering Wheel'),
    ('57700', 'Steering', 'Steering Gear',                    'Steering Gear Assembly'),

    -- ── Suspension ────────────────────────────────────────────────────────
    ('52910', 'Suspension', 'Wheel & Tire',                   'Wheel Rim / Alloy'),
    ('54630', 'Suspension', 'Front Coil Spring',              'Front Coil Spring'),
    ('54620', 'Suspension', 'Upper Control Arm - Front',      'Front Upper Control Arm'),
    ('54610', 'Suspension', 'Lower Control Arm - Front',      'Front Lower Control Arm'),

    -- ── Engine sub-systems ────────────────────────────────────────────────
    ('21020', 'Engine', 'Piston / Piston Ring',               'Piston with Rings'),
    ('21510', 'Engine', 'Oil Pan',                            'Engine Oil Pan'),
    ('23120', 'Engine', 'Timing Belt',                        'Timing Belt'),
    ('24410', 'Engine', 'Camshaft',                           'Camshaft Assembly'),
    ('25100', 'Cooling', 'Water Pump',                        'Engine Water Pump'),
    ('25620', 'Cooling', 'Thermostat',                        'Engine Thermostat'),
    ('25330', 'Cooling', 'Cooling Fan',                       'Cooling Fan Motor'),
    ('27301', 'Engine', 'Ignition Coil',                      'Ignition Coil Assembly'),
    ('27200', 'Engine', 'Distributor',                        'Ignition Distributor'),
    ('28110', 'Engine', 'Air Filter Housing',                 'Air Filter Housing / Airbox'),

    -- ── Exhaust ───────────────────────────────────────────────────────────
    ('28600', 'Exhaust', 'Muffler',                           'Muffler / Silencer'),
    ('28620', 'Exhaust', 'Exhaust Pipe',                      'Exhaust Pipe Center'),

    -- ── Drivetrain / Transmission ─────────────────────────────────────────
    ('43520', 'Transmission', 'CV Axle - Front',              'Front CV Axle Shaft'),
    ('45211', 'Transmission', 'Automatic Transmission Filter','ATF Filter'),
    ('49500', 'Drivetrain', 'Rear Differential',              'Rear Differential Assembly'),

    -- ── Brakes ────────────────────────────────────────────────────────────
    ('58411', 'Brakes', 'Brake Disc - Rear',                  'Rear Brake Disc / Rotor'),
    ('58500', 'Brakes', 'Brake Master Cylinder',              'Brake Master Cylinder'),
    ('59810', 'Brakes', 'ABS Sensor - Front',                 'Front ABS Wheel Speed Sensor'),
    ('59830', 'Brakes', 'ABS Sensor - Rear',                  'Rear ABS Wheel Speed Sensor'),

    -- ── HVAC ──────────────────────────────────────────────────────────────
    ('97205', 'HVAC', 'A/C Compressor',                       'A/C Compressor Assembly'),
    ('97701', 'HVAC', 'A/C Condenser',                        'A/C Condenser'),
    ('97719', 'HVAC', 'A/C Compressor Clutch',                'A/C Compressor Clutch'),

    -- ── Fuel System ───────────────────────────────────────────────────────
    ('31111', 'Engine', 'Fuel Filter (petrol)',               'Petrol Fuel Filter'),
    ('31300', 'Engine', 'Fuel Pump',                          'In-Tank Fuel Pump Assembly'),
    ('31350', 'Engine', 'Fuel Sending Unit',                  'Fuel Level Sending Unit'),
    ('31910', 'Engine', 'Fuel Filter (diesel)',               'Diesel Fuel Filter')
ON CONFLICT (prefix) DO NOTHING;

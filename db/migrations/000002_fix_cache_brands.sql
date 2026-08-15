-- The original MySQL step renamed mfrId -> dataSupplierId and backfilled
-- brand/category metadata from TecDoc source tables.
--
-- The PostgreSQL schema starts directly with the corrected columns:
--   data_supplier_id, brand_name, category_name
--
-- The ETL/import step that populates hk_parts_cache must provide these fields.
SELECT 1;

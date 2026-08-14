ALTER TABLE oem_search_index
DROP CONSTRAINT IF EXISTS oem_search_index_source_table_check;

ALTER TABLE oem_search_index
ADD CONSTRAINT oem_search_index_source_table_check
CHECK (source_table IN ('oemnumbers', 'oem_number', 'articlecrosses', 'discovered', 'substitution'));

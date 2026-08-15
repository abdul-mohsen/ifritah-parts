CREATE TABLE IF NOT EXISTS substitution_links (
    from_part_number TEXT NOT NULL,
    to_part_number   TEXT NOT NULL,
    description      TEXT,
    source_key       TEXT NOT NULL,
    source_label     TEXT NOT NULL,
    source_detail    TEXT NOT NULL,
    source_warning   TEXT,
    confidence       DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    PRIMARY KEY (from_part_number, to_part_number, source_key)
);

CREATE INDEX IF NOT EXISTS idx_substitution_links_from_part
    ON substitution_links (from_part_number);

CREATE INDEX IF NOT EXISTS idx_substitution_links_to_part
    ON substitution_links (to_part_number);

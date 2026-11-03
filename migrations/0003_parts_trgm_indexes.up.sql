CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS parts_partnumber_trgm_idx  ON parts USING gin (partnumber gin_trgm_ops);
CREATE INDEX IF NOT EXISTS parts_pcbref_trgm_idx      ON parts USING gin (pcbref gin_trgm_ops);
CREATE INDEX IF NOT EXISTS parts_value_trgm_idx       ON parts USING gin (value gin_trgm_ops);
CREATE INDEX IF NOT EXISTS parts_description_trgm_idx ON parts USING gin (description gin_trgm_ops);
CREATE INDEX IF NOT EXISTS parts_type_trgm_idx        ON parts USING gin (type gin_trgm_ops);

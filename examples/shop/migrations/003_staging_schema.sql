-- 003_staging_schema.sql
-- A throwaway staging schema (dropped wholesale in 005), exercising a second
-- schema and an array-typed column.

CREATE SCHEMA staging;

CREATE TABLE staging.import_tmp (
    id      serial PRIMARY KEY,
    payload text NOT NULL,
    tags    text[]
);

CREATE INDEX idx_import_payload ON staging.import_tmp (payload);

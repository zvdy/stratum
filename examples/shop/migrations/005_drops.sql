-- 005_drops.sql
-- The DROP surface that stratum's pure-Go parser now models structurally.

-- Remove an index by name.
DROP INDEX idx_orders_status;

-- Drop a previously-added unique constraint (the coupon is no longer unique).
ALTER TABLE orders DROP CONSTRAINT uq_orders_coupon;

-- Drop a deprecated table.
DROP TABLE legacy_carts;

-- Drop the entire staging schema (and everything in it).
DROP SCHEMA staging CASCADE;

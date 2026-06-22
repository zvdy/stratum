-- 005_seed_and_cleanup.sql
-- Seed data (DML) and a procedural block are parsed but ignored by stratum;
-- only the structural DROP affects the rendered schema.

INSERT INTO categories (name) VALUES ('Books'), ('Electronics');
INSERT INTO customers (email, full_name) VALUES ('a@example.com', 'Ada Lovelace');

UPDATE products SET price_cents = 999 WHERE sku = 'DEMO';
DELETE FROM legacy_wishlist WHERE note IS NULL;

-- Procedural block: skipped silently.
DO $$
BEGIN
    RAISE NOTICE 'post-deploy hook';
END
$$;

-- Structural change: remove the deprecated table.
DROP TABLE legacy_wishlist;

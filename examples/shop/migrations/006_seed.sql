-- 006_seed.sql
-- DML and a procedural block: parsed but ignored by stratum (structure only).

INSERT INTO categories (name) VALUES ('Books'), ('Electronics');
INSERT INTO customers (email, full_name) VALUES ('ada@example.com', 'Ada Lovelace');

UPDATE products SET price = 9.99 WHERE sku = 'DEMO';
DELETE FROM addresses WHERE region IS NULL;

DO $$
BEGIN
    -- The ; characters inside this block must not split the statement.
    RAISE NOTICE 'seed complete';
END
$$;

-- 004_refactors.sql
-- Schema evolution: add/rename/retype columns, set/drop nullability, and add
-- constraints/indexes that later migrations partly undo.

ALTER TABLE products ADD COLUMN description text;
ALTER TABLE products ADD COLUMN weight_grams integer;

ALTER TABLE customers ADD COLUMN phone varchar(20);
ALTER TABLE customers RENAME COLUMN phone TO phone_number;
ALTER TABLE customers ALTER COLUMN phone_number TYPE text;
ALTER TABLE customers ALTER COLUMN full_name SET NOT NULL;

ALTER TABLE addresses ADD COLUMN region varchar(120);

ALTER TABLE orders ADD COLUMN coupon varchar(32);
ALTER TABLE orders ADD CONSTRAINT uq_orders_coupon UNIQUE (coupon);

CREATE UNIQUE INDEX uq_products_name ON products (name);

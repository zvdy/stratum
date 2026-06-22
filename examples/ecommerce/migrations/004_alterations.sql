-- 004_alterations.sql
-- Schema evolution: add/drop/rename/retype columns, add constraints, add indexes.

-- Add new columns to products.
ALTER TABLE products ADD COLUMN description text;
ALTER TABLE products ADD COLUMN weight_grams int;

-- Rename and retype an existing column.
ALTER TABLE customers ADD COLUMN phone varchar(20);
ALTER TABLE customers RENAME COLUMN phone TO phone_number;
ALTER TABLE customers ALTER COLUMN phone_number TYPE text;

-- Add a temporary column then drop it.
ALTER TABLE orders ADD COLUMN scratch int;
ALTER TABLE orders DROP COLUMN scratch;

-- Add a foreign key and a unique constraint via ALTER.
ALTER TABLE orders ADD COLUMN coupon_code varchar(32);
ALTER TABLE orders ADD CONSTRAINT uq_orders_coupon UNIQUE (coupon_code);

ALTER TABLE addresses ADD COLUMN region varchar(120);

-- Composite, non-unique index on order_items.
CREATE INDEX idx_order_items_product ON order_items (product_id);

-- Unique index spanning two columns.
CREATE UNIQUE INDEX uq_inventory_product ON inventory (product_id);

-- Cross-schema FK added after the fact.
ALTER TABLE auth.users ADD CONSTRAINT fk_users_customer
    FOREIGN KEY (customer_id) REFERENCES customers (id);

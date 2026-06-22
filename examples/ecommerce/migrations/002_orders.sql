-- 002_orders.sql
-- Orders, order line items (composite PK), and inventory.

CREATE TABLE orders (
    id           bigserial PRIMARY KEY,
    customer_id  int NOT NULL REFERENCES customers (id),
    address_id   int REFERENCES addresses (id),
    status       varchar(20) NOT NULL DEFAULT 'pending',
    total_cents  int NOT NULL DEFAULT 0,
    placed_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (total_cents >= 0)
);

CREATE TABLE order_items (
    order_id    bigint NOT NULL REFERENCES orders (id),
    product_id  int NOT NULL REFERENCES products (id),
    quantity    int NOT NULL DEFAULT 1,
    price_cents int NOT NULL,
    PRIMARY KEY (order_id, product_id),
    CHECK (quantity > 0)
);

CREATE TABLE inventory (
    product_id  int PRIMARY KEY REFERENCES products (id),
    on_hand     int NOT NULL DEFAULT 0,
    reserved    int NOT NULL DEFAULT 0,
    CHECK (on_hand >= 0)
);

-- Index to speed up status filtering.
CREATE INDEX idx_orders_status ON orders (status);

-- 002_orders.sql
-- Orders, composite-key line items, and an index dropped later.

CREATE TABLE orders (
    id          bigserial PRIMARY KEY,
    customer_id int NOT NULL REFERENCES customers (id),
    status      varchar(20) NOT NULL DEFAULT 'pending',
    total       numeric(12,2) NOT NULL DEFAULT 0,
    placed_at   timestamp with time zone NOT NULL DEFAULT now()
);

CREATE TABLE order_items (
    order_id    bigint NOT NULL REFERENCES orders (id),
    product_id  int NOT NULL REFERENCES products (id),
    quantity    int NOT NULL DEFAULT 1,
    unit_price  numeric(10,2) NOT NULL,
    PRIMARY KEY (order_id, product_id),
    CHECK (quantity > 0)
);

CREATE INDEX idx_orders_status ON orders (status);

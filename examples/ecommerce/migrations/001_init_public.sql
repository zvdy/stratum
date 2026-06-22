-- 001_init_public.sql
-- Core e-commerce catalog and customer tables (public schema).

CREATE TABLE customers (
    id          serial PRIMARY KEY,
    email       varchar(255) NOT NULL UNIQUE,
    full_name   varchar(200) NOT NULL,
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (email <> '')
);

CREATE TABLE addresses (
    id          serial PRIMARY KEY,
    customer_id int NOT NULL REFERENCES customers (id),
    line1       varchar(255) NOT NULL,
    line2       varchar(255),
    city        varchar(120) NOT NULL,
    country     char(2) NOT NULL,
    is_default  boolean NOT NULL DEFAULT false
);

CREATE TABLE categories (
    id          serial PRIMARY KEY,
    name        varchar(120) NOT NULL UNIQUE,
    parent_id   int REFERENCES categories (id)
);

CREATE TABLE products (
    id          serial PRIMARY KEY,
    sku         varchar(64) NOT NULL UNIQUE,
    name        varchar(255) NOT NULL,
    category_id int REFERENCES categories (id),
    price_cents int NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (price_cents >= 0)
);

-- A deprecated table dropped in a later migration (see 005).
CREATE TABLE legacy_wishlist (
    id          serial PRIMARY KEY,
    customer_id int NOT NULL,
    note        text
);

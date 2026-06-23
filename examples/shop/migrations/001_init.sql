-- 001_init.sql
-- Core catalog & customer tables in the public schema, plus a table that a later
-- migration will DROP.

CREATE TABLE customers (
    id          serial PRIMARY KEY,
    email       varchar(255) NOT NULL UNIQUE,
    full_name   text,
    created_at  timestamp with time zone NOT NULL DEFAULT now(),
    CHECK (email <> '')
);

CREATE TABLE addresses (
    id          serial PRIMARY KEY,
    customer_id int NOT NULL REFERENCES customers (id),
    line1       varchar(255) NOT NULL,
    city        varchar(120) NOT NULL,
    country     char(2) NOT NULL,
    is_default  boolean NOT NULL DEFAULT false
);

-- Self-referential foreign key.
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
    price       numeric(10,2) NOT NULL DEFAULT 0,
    CHECK (price >= 0)
);

-- Deprecated table, dropped in 005.
CREATE TABLE legacy_carts (
    id          serial PRIMARY KEY,
    customer_id int NOT NULL
);

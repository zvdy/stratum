-- 003_auth_schema.sql
-- Authentication objects live in a dedicated `auth` schema.

CREATE SCHEMA auth;

CREATE TABLE auth.users (
    id            serial PRIMARY KEY,
    customer_id   int,
    username      varchar(64) NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auth.sessions (
    id          uuid PRIMARY KEY,
    user_id     int NOT NULL REFERENCES auth.users (id),
    issued_at   timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    user_agent  text
);

CREATE INDEX idx_sessions_user ON auth.sessions (user_id);

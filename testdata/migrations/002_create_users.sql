CREATE TABLE public.users (
    id serial PRIMARY KEY,
    email varchar(255) NOT NULL UNIQUE,
    org_id int NOT NULL REFERENCES orgs (id),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz DEFAULT now(),
    CHECK (email <> '')
);

CREATE INDEX idx_users_org_id ON users (org_id);
CREATE UNIQUE INDEX uq_users_email ON public.users (email);

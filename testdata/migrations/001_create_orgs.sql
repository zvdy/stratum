-- Create the orgs table.
CREATE TABLE orgs (
    id serial PRIMARY KEY,
    name varchar(255) NOT NULL,
    slug varchar(64) UNIQUE
);

-- DML should be ignored by the parser.
INSERT INTO orgs (name, slug) VALUES ('Acme', 'acme');

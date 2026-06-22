-- A table with a foreign key to a table that gets dropped later (dangling FK).
CREATE TABLE audit_log (
    id bigserial PRIMARY KEY,
    legacy_id int REFERENCES legacy_events (id),
    note text
);

CREATE TABLE legacy_events (
    id serial PRIMARY KEY
);

DROP TABLE legacy_events;

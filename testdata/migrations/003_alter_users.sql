-- Add, rename, retype and drop columns; add a table-level FK and a check.
ALTER TABLE users ADD COLUMN full_name varchar(100);
ALTER TABLE users RENAME COLUMN full_name TO display_name;
ALTER TABLE users ALTER COLUMN display_name TYPE text;
ALTER TABLE users ADD COLUMN temp_col int;
ALTER TABLE users DROP COLUMN temp_col;

ALTER TABLE users
    ADD CONSTRAINT fk_users_manager FOREIGN KEY (manager_id) REFERENCES users (id);
ALTER TABLE users ADD COLUMN manager_id int;

ALTER TABLE orgs ADD CONSTRAINT uq_orgs_name UNIQUE (name);

-- Procedural block should be skipped silently.
DO $$
BEGIN
    RAISE NOTICE 'noop';
END
$$;

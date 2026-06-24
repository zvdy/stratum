package parser

import "testing"

func TestDropIndexRemovesIndex(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE users (id serial PRIMARY KEY, email text);
		CREATE UNIQUE INDEX uq_users_email ON users (email);
		CREATE INDEX idx_users_email ON users (email);
		DROP INDEX uq_users_email;
	`)
	users := mustTable(t, s, "public.users")
	for _, idx := range users.Indexes {
		if idx.Name == "uq_users_email" {
			t.Errorf("uq_users_email should have been dropped: %+v", users.Indexes)
		}
	}
	// The other index survives.
	found := false
	for _, idx := range users.Indexes {
		if idx.Name == "idx_users_email" {
			found = true
		}
	}
	if !found {
		t.Errorf("idx_users_email should remain: %+v", users.Indexes)
	}
}

func TestDropIndexSchemaQualified(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE users (id serial PRIMARY KEY, email text);
		CREATE INDEX idx_users_email ON public.users (email);
		DROP INDEX IF EXISTS public.idx_users_email;
	`)
	users := mustTable(t, s, "public.users")
	if len(users.Indexes) != 0 {
		t.Errorf("index should be dropped via qualified name: %+v", users.Indexes)
	}
}

func TestAlterDropConstraintRemovesNamedCheck(t *testing.T) {
	s, warns := parseInline(t, `
		CREATE TABLE t (
			id serial PRIMARY KEY,
			email text CONSTRAINT chk_email CHECK (email <> ''),
			CONSTRAINT chk_id CHECK (id > 0)
		);
		ALTER TABLE t DROP CONSTRAINT chk_email;
		ALTER TABLE t DROP CONSTRAINT chk_id;
	`)
	tbl := mustTable(t, s, "public.t")
	if len(tbl.Checks) != 0 {
		t.Errorf("both named checks should be dropped, got %+v", tbl.Checks)
	}
	for _, w := range warns {
		if w.Msg != "" && (w.Msg == "ALTER TABLE public.t DROP CONSTRAINT chk_email: not found" ||
			w.Msg == "ALTER TABLE public.t DROP CONSTRAINT chk_id: not found") {
			t.Errorf("unexpected not-found warning: %s", w.Msg)
		}
	}
}

func TestAlterDropConstraintRemovesNamedPrimaryKey(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE t (id int CONSTRAINT pk_t PRIMARY KEY, name text);
		CREATE TABLE u (a int, b int, CONSTRAINT pk_u PRIMARY KEY (a, b));
		ALTER TABLE t DROP CONSTRAINT pk_t;
		ALTER TABLE u DROP CONSTRAINT pk_u;
	`)
	tbl := mustTable(t, s, "public.t")
	if id, _ := tbl.Column("id"); id.PrimaryKey {
		t.Errorf("id should no longer be PK after DROP CONSTRAINT pk_t")
	}
	if id, _ := tbl.Column("id"); !id.NotNull {
		t.Errorf("dropping the PK should leave NOT NULL intact")
	}
	u := mustTable(t, s, "public.u")
	for _, c := range u.Columns {
		if c.PrimaryKey {
			t.Errorf("composite PK column %s should be cleared after DROP CONSTRAINT pk_u", c.Name)
		}
	}
}

func TestAlterDropConstraintRemovesFKAndUnique(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE orgs (id serial PRIMARY KEY);
		CREATE TABLE users (id serial PRIMARY KEY, org_id int, email text);
		ALTER TABLE users ADD CONSTRAINT fk_org FOREIGN KEY (org_id) REFERENCES orgs (id);
		ALTER TABLE users ADD CONSTRAINT uq_email UNIQUE (email);
		ALTER TABLE users DROP CONSTRAINT fk_org;
		ALTER TABLE users DROP CONSTRAINT uq_email;
	`)
	users := mustTable(t, s, "public.users")
	if len(users.FKs) != 0 {
		t.Errorf("fk_org should be dropped: %+v", users.FKs)
	}
	for _, idx := range users.Indexes {
		if idx.Name == "uq_email" {
			t.Errorf("uq_email should be dropped: %+v", users.Indexes)
		}
	}
}

func TestDropSchemaRemovesAllTables(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE SCHEMA auth;
		CREATE TABLE auth.users (id serial PRIMARY KEY);
		CREATE TABLE auth.sessions (id uuid PRIMARY KEY);
		CREATE TABLE public.orders (id serial PRIMARY KEY);
		DROP SCHEMA auth CASCADE;
	`)
	if _, ok := s.Get("auth.users"); ok {
		t.Errorf("auth.users should be gone")
	}
	if _, ok := s.Get("auth.sessions"); ok {
		t.Errorf("auth.sessions should be gone")
	}
	if _, ok := s.Get("public.orders"); !ok {
		t.Errorf("public.orders should remain")
	}
}

func TestMultiWordTypesAndModifiers(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE t (
			a double precision,
			b character varying(120),
			c timestamp with time zone,
			d numeric(10,2),
			e integer,
			f bigint,
			g text[]
		);
	`)
	tbl := mustTable(t, s, "public.t")
	want := map[string]string{
		"a": "double precision",
		"b": "varchar(120)",
		"c": "timestamptz",
		"d": "numeric(10,2)",
		"e": "int",
		"f": "bigint",
		"g": "text[]",
	}
	for name, wantType := range want {
		col, ok := tbl.Column(name)
		if !ok {
			t.Errorf("column %s missing", name)
			continue
		}
		if col.Type != wantType {
			t.Errorf("column %s type = %q, want %q", name, col.Type, wantType)
		}
	}
}

func TestDollarQuotedFunctionBodyDoesNotSplit(t *testing.T) {
	// The ';' inside the function body must not be treated as a statement
	// boundary; the CREATE TABLE after it must still be parsed.
	s, _ := parseInline(t, `
		CREATE FUNCTION f() RETURNS int AS $$
		BEGIN
			-- this ; should not split the statement
			RETURN 1;
		END
		$$ LANGUAGE plpgsql;
		CREATE TABLE after_fn (id int);
	`)
	if _, ok := s.Get("public.after_fn"); !ok {
		t.Errorf("table after function body should be parsed; got %v", s.SortedTableKeys())
	}
}

func TestCompositePrimaryKey(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE order_items (
			order_id bigint NOT NULL,
			product_id int NOT NULL,
			PRIMARY KEY (order_id, product_id)
		);
	`)
	tbl := mustTable(t, s, "public.order_items")
	for _, name := range []string{"order_id", "product_id"} {
		col, _ := tbl.Column(name)
		if !col.PrimaryKey {
			t.Errorf("%s should be part of composite PK: %+v", name, col)
		}
	}
}

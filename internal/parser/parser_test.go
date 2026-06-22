package parser

import (
	"strings"
	"testing"

	"github.com/zvdy/stratum/internal/schema"
)

// parseInline is a helper that parses SQL and fails the test on hard errors.
func parseInline(t *testing.T, sql string) (*schema.Schema, []Warning) {
	t.Helper()
	s, warns, err := ParseSQL(sql, Options{})
	if err != nil {
		t.Fatalf("ParseSQL error: %v", err)
	}
	return s, warns
}

func mustTable(t *testing.T, s *schema.Schema, key string) *schema.Table {
	t.Helper()
	tbl, ok := s.Get(key)
	if !ok {
		t.Fatalf("table %q not found; have %v", key, s.SortedTableKeys())
	}
	return tbl
}

func TestCreateTableColumnsAndConstraints(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE orgs (
			id serial PRIMARY KEY,
			name varchar(255) NOT NULL,
			slug varchar(64) UNIQUE
		);
		CREATE TABLE public.users (
			id serial PRIMARY KEY,
			email varchar(255) NOT NULL UNIQUE,
			org_id int NOT NULL REFERENCES orgs (id),
			is_active boolean NOT NULL DEFAULT true,
			CHECK (email <> '')
		);
	`)

	users := mustTable(t, s, "public.users")

	// Column order and basic attributes.
	wantCols := []struct {
		name    string
		typ     string
		notNull bool
		pk      bool
	}{
		{"id", "serial", true, true},
		{"email", "varchar(255)", true, false},
		{"org_id", "int", true, false},
		{"is_active", "boolean", true, false},
	}
	if len(users.Columns) != len(wantCols) {
		t.Fatalf("got %d columns, want %d (%+v)", len(users.Columns), len(wantCols), users.Columns)
	}
	for i, w := range wantCols {
		c := users.Columns[i]
		if c.Name != w.name || c.Type != w.typ || c.NotNull != w.notNull || c.PrimaryKey != w.pk {
			t.Errorf("column %d = %+v, want name=%s type=%s notnull=%v pk=%v",
				i, c, w.name, w.typ, w.notNull, w.pk)
		}
	}

	// Default expression captured.
	if c, _ := users.Column("is_active"); c.Default != "true" {
		t.Errorf("is_active default = %q, want true", c.Default)
	}

	// Inline REFERENCES became a foreign key.
	if len(users.FKs) != 1 {
		t.Fatalf("got %d FKs, want 1: %+v", len(users.FKs), users.FKs)
	}
	fk := users.FKs[0]
	if fk.RefTable != "public.orgs" || len(fk.Columns) != 1 || fk.Columns[0] != "org_id" {
		t.Errorf("fk = %+v, want org_id -> public.orgs", fk)
	}

	// Inline UNIQUE on email + table CHECK.
	foundUnique := false
	for _, idx := range users.Indexes {
		if idx.Unique && len(idx.Columns) == 1 && idx.Columns[0] == "email" {
			foundUnique = true
		}
	}
	if !foundUnique {
		t.Errorf("expected unique index on email, got %+v", users.Indexes)
	}
	if len(users.Checks) != 1 {
		t.Errorf("got %d checks, want 1: %+v", len(users.Checks), users.Checks)
	}
}

func TestAlterTableOperations(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE users (id serial PRIMARY KEY);
		ALTER TABLE users ADD COLUMN full_name varchar(100);
		ALTER TABLE users RENAME COLUMN full_name TO display_name;
		ALTER TABLE users ALTER COLUMN display_name TYPE text;
		ALTER TABLE users ADD COLUMN temp_col int;
		ALTER TABLE users DROP COLUMN temp_col;
		ALTER TABLE users ADD COLUMN org_id int;
		ALTER TABLE users ADD CONSTRAINT fk_org FOREIGN KEY (org_id) REFERENCES orgs (id);
		ALTER TABLE users ADD CONSTRAINT uq_name UNIQUE (display_name);
	`)

	users := mustTable(t, s, "public.users")

	// Renamed + retyped column present; temp_col gone.
	col, ok := users.Column("display_name")
	if !ok {
		t.Fatalf("display_name not found: %+v", users.Columns)
	}
	if col.Type != "text" {
		t.Errorf("display_name type = %q, want text", col.Type)
	}
	if _, ok := users.Column("full_name"); ok {
		t.Errorf("full_name should have been renamed away")
	}
	if _, ok := users.Column("temp_col"); ok {
		t.Errorf("temp_col should have been dropped")
	}

	// Added FK via ALTER.
	if len(users.FKs) != 1 || users.FKs[0].RefTable != "public.orgs" {
		t.Errorf("FKs = %+v, want one to public.orgs", users.FKs)
	}
	if !users.IsFKColumn("org_id") {
		t.Errorf("org_id should be an FK column")
	}

	// Added unique index via ALTER.
	foundUnique := false
	for _, idx := range users.Indexes {
		if idx.Unique && len(idx.Columns) == 1 && idx.Columns[0] == "display_name" {
			foundUnique = true
		}
	}
	if !foundUnique {
		t.Errorf("expected unique index on display_name, got %+v", users.Indexes)
	}
}

func TestCreateIndexAndDropTable(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE users (id serial PRIMARY KEY, org_id int);
		CREATE INDEX idx_users_org ON users (org_id);
		CREATE UNIQUE INDEX uq_users_org ON public.users (org_id);
		CREATE TABLE scratch (id int);
		DROP TABLE scratch;
	`)

	users := mustTable(t, s, "public.users")
	if len(users.Indexes) != 2 {
		t.Fatalf("got %d indexes, want 2: %+v", len(users.Indexes), users.Indexes)
	}
	var sawUnique, sawPlain bool
	for _, idx := range users.Indexes {
		if idx.Name == "uq_users_org" && idx.Unique {
			sawUnique = true
		}
		if idx.Name == "idx_users_org" && !idx.Unique {
			sawPlain = true
		}
	}
	if !sawUnique || !sawPlain {
		t.Errorf("index flags wrong: %+v", users.Indexes)
	}

	if _, ok := s.Get("public.scratch"); ok {
		t.Errorf("scratch table should have been dropped")
	}
}

func TestRenameTable(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE old_name (id int);
		ALTER TABLE old_name RENAME TO new_name;
	`)
	if _, ok := s.Get("public.old_name"); ok {
		t.Errorf("old_name should be gone")
	}
	tbl, ok := s.Get("public.new_name")
	if !ok {
		t.Fatalf("new_name not found: %v", s.SortedTableKeys())
	}
	if tbl.Name != "public.new_name" {
		t.Errorf("table Name = %q, want public.new_name", tbl.Name)
	}
}

func TestDanglingForeignKeyWarns(t *testing.T) {
	_, warns := parseInline(t, `
		CREATE TABLE a (id int, b_id int REFERENCES missing_table (id));
	`)
	found := false
	for _, w := range warns {
		if strings.Contains(w.Msg, "dangling") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a dangling FK warning, got %v", warns)
	}
}

func TestIgnoredStatementsDoNotPanicOrError(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE t (id int);
		INSERT INTO t VALUES (1);
		UPDATE t SET id = 2;
		DELETE FROM t;
		DO $$ BEGIN PERFORM 1; END $$;
		COMMENT ON TABLE t IS 'hi';
	`)
	if _, ok := s.Get("public.t"); !ok {
		t.Errorf("table t should still exist after DML statements")
	}
}

func TestEmptyInputWarns(t *testing.T) {
	// Whitespace-only via file path goes through parseFile; here we confirm an
	// empty statement set is harmless.
	s, _, err := ParseSQL("-- just a comment\n", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.Tables) != 0 {
		t.Errorf("expected no tables, got %v", s.SortedTableKeys())
	}
}

func TestParseErrorIsFatal(t *testing.T) {
	_, _, err := ParseSQL("CREATE TABLE (", Options{})
	if err == nil {
		t.Errorf("expected a parse error for invalid SQL")
	}
}

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAlterColumnVariants(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE t (id serial PRIMARY KEY, a int, b text, c int);
		ALTER TABLE t ALTER COLUMN a SET DEFAULT 0;
		ALTER TABLE t ALTER COLUMN a DROP DEFAULT;
		ALTER TABLE t ALTER COLUMN b SET NOT NULL;
		ALTER TABLE t ALTER COLUMN b DROP NOT NULL;
		ALTER TABLE t ALTER COLUMN c SET DATA TYPE bigint;
	`)
	tbl := mustTable(t, s, "public.t")

	if a, _ := tbl.Column("a"); a.Default != "" {
		t.Errorf("a default should be cleared by DROP DEFAULT, got %q", a.Default)
	}
	if b, _ := tbl.Column("b"); b.NotNull {
		t.Errorf("b NOT NULL should be cleared by DROP NOT NULL")
	}
	if c, _ := tbl.Column("c"); c.Type != "bigint" {
		t.Errorf("c type = %q, want bigint (SET DATA TYPE)", c.Type)
	}
}

func TestTypeAliasNormalization(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE t (
			a int4, b int8, c int2, d float4, e float8,
			f bool, g bpchar, h timestamp without time zone,
			i time with time zone
		);
	`)
	tbl := mustTable(t, s, "public.t")
	want := map[string]string{
		"a": "int", "b": "bigint", "c": "smallint",
		"d": "real", "e": "double precision",
		"f": "boolean", "g": "char",
		"h": "timestamp", "i": "timetz",
	}
	for name, wantType := range want {
		if col, ok := tbl.Column(name); !ok || col.Type != wantType {
			t.Errorf("column %s type = %q, want %q", name, col.Type, wantType)
		}
	}
}

func TestColumnInlineVariants(t *testing.T) {
	s, warns := parseInline(t, `
		CREATE TABLE t (
			id int CONSTRAINT pk_t PRIMARY KEY,
			a text NULL,
			b int GENERATED ALWAYS AS IDENTITY,
			c timestamptz DEFAULT now()
		);
	`)
	tbl := mustTable(t, s, "public.t")
	if id, _ := tbl.Column("id"); !id.PrimaryKey {
		t.Errorf("id should be PK via named inline constraint")
	}
	if c, _ := tbl.Column("c"); c.Default != "now()" {
		t.Errorf("c default = %q, want now()", c.Default)
	}
	// b uses GENERATED (unknown clause) — must be parsed without error.
	if _, ok := tbl.Column("b"); !ok {
		t.Errorf("column b should still be added despite GENERATED clause")
	}
	_ = warns
}

func TestParseFilesWithTempDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("001.sql", `CREATE TABLE a (id serial PRIMARY KEY);`)
	write("002.sql", `CREATE TABLE b (id serial PRIMARY KEY, a_id int REFERENCES a (id));`)
	write("003_empty.sql", "   \n\t  ")

	files := []string{
		filepath.Join(dir, "001.sql"),
		filepath.Join(dir, "002.sql"),
		filepath.Join(dir, "003_empty.sql"),
	}
	s, warns, err := ParseFiles(files, Options{})
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	if _, ok := s.Get("public.a"); !ok {
		t.Errorf("table a missing")
	}
	if _, ok := s.Get("public.b"); !ok {
		t.Errorf("table b missing")
	}
	emptyWarn := false
	for _, w := range warns {
		if strings.Contains(w.Msg, "empty migration file") {
			emptyWarn = true
		}
	}
	if !emptyWarn {
		t.Errorf("expected empty-file warning, got %v", warns)
	}
}

func TestParseFilesMissingFileErrors(t *testing.T) {
	_, _, err := ParseFiles([]string{"/nonexistent/does-not-exist.sql"}, Options{})
	if err == nil {
		t.Errorf("expected error for missing file")
	}
}

func TestVerboseOnStatement(t *testing.T) {
	var notes []string
	opts := Options{Verbose: true, OnStatement: func(_, desc string) {
		notes = append(notes, desc)
	}}
	_, _, err := ParseSQL(`
		CREATE TABLE t (id int);
		INSERT INTO t VALUES (1);
		DROP TABLE t;
	`, opts)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "CREATE TABLE public.t") || !strings.Contains(joined, "DROP TABLE public.t") {
		t.Errorf("verbose notes missing expected entries: %v", notes)
	}
}

func TestSkipsNonStructuralStatements(t *testing.T) {
	s, _ := parseInline(t, `
		SET search_path TO public;
		CREATE TYPE mood AS ENUM ('happy', 'sad');
		CREATE SEQUENCE s START 1;
		CREATE OR REPLACE VIEW v AS SELECT 1;
		GRANT SELECT ON ALL TABLES IN SCHEMA public TO readonly;
		WITH x AS (SELECT 1) SELECT * FROM x;
		CREATE TABLE survivor (id int);
		DROP TYPE mood;
	`)
	if _, ok := s.Get("public.survivor"); !ok {
		t.Errorf("survivor table should be parsed after skipped statements: %v", s.SortedTableKeys())
	}
	if len(s.Tables) != 1 {
		t.Errorf("only survivor should exist, got %v", s.SortedTableKeys())
	}
}

func TestDropTableMultipleAndIfExists(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE a (id int);
		CREATE TABLE b (id int);
		CREATE TABLE c (id int);
		DROP TABLE IF EXISTS a, b CASCADE;
	`)
	if _, ok := s.Get("public.a"); ok {
		t.Errorf("a should be dropped")
	}
	if _, ok := s.Get("public.b"); ok {
		t.Errorf("b should be dropped")
	}
	if _, ok := s.Get("public.c"); !ok {
		t.Errorf("c should remain")
	}
}

func TestQuotedIdentifiersPreserveCase(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE "MixedCase" ("Id" int PRIMARY KEY, "Name" text);
	`)
	tbl, ok := s.Get("public.MixedCase")
	if !ok {
		t.Fatalf("quoted table name should preserve case: %v", s.SortedTableKeys())
	}
	if _, ok := tbl.Column("Id"); !ok {
		t.Errorf("quoted column 'Id' should preserve case: %+v", tbl.Columns)
	}
}

func TestMoreMultiWordTypes(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE t (
			a character varying(10),
			b character(5),
			c bit varying(8),
			d national character varying(4),
			e timestamp with time zone
		);
	`)
	tbl := mustTable(t, s, "public.t")
	want := map[string]string{
		"a": "varchar(10)",
		"b": "char(5)",
		"c": "bit varying(8)",
		"e": "timestamptz",
	}
	for name, wantType := range want {
		if col, ok := tbl.Column(name); !ok || col.Type != wantType {
			got := ""
			if col != nil {
				got = col.Type
			}
			t.Errorf("column %s type = %q, want %q", name, got, wantType)
		}
	}
	if _, ok := tbl.Column("d"); !ok {
		t.Errorf("column d should exist")
	}
}

func TestInlineReferencesWithActions(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE parent (id int PRIMARY KEY);
		CREATE TABLE child (
			id int PRIMARY KEY,
			pid int NOT NULL REFERENCES parent (id) ON DELETE CASCADE ON UPDATE RESTRICT
		);
	`)
	tbl := mustTable(t, s, "public.child")
	if len(tbl.FKs) != 1 || tbl.FKs[0].RefTable != "public.parent" {
		t.Errorf("FK with referential actions not parsed: %+v", tbl.FKs)
	}
	if p, _ := tbl.Column("pid"); !p.NotNull {
		t.Errorf("pid should be NOT NULL even before REFERENCES")
	}
}

func TestAlterTableIfExistsOnlyAndAddColumnNoKeyword(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE t (id int);
		ALTER TABLE IF EXISTS ONLY t ADD extra text;
		ALTER TABLE t RENAME extra TO renamed;
	`)
	tbl := mustTable(t, s, "public.t")
	if _, ok := tbl.Column("renamed"); !ok {
		t.Errorf("column added without COLUMN keyword then renamed without COLUMN keyword: %+v", tbl.Columns)
	}
}

func TestCreateIndexConcurrentlyUsingAndExpression(t *testing.T) {
	s, _ := parseInline(t, `
		CREATE TABLE t (id int, email text);
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_t ON ONLY t USING btree (email);
		CREATE INDEX idx_expr ON t (lower(email));
	`)
	tbl := mustTable(t, s, "public.t")
	foundCols, foundExpr := false, false
	for _, idx := range tbl.Indexes {
		if idx.Name == "idx_t" && len(idx.Columns) == 1 && idx.Columns[0] == "email" {
			foundCols = true
		}
		if idx.Name == "idx_expr" && len(idx.Columns) == 0 {
			foundExpr = true // expression index contributes no plain column
		}
	}
	if !foundCols {
		t.Errorf("idx_t should index column email: %+v", tbl.Indexes)
	}
	if !foundExpr {
		t.Errorf("idx_expr (expression) should have no plain columns: %+v", tbl.Indexes)
	}
}

func TestDropIndexConcurrentlyAndDropSchemaMissing(t *testing.T) {
	_, warns := parseInline(t, `
		DROP INDEX CONCURRENTLY IF EXISTS nope;
		DROP SCHEMA IF EXISTS ghost;
		DROP VIEW whatever;
	`)
	var missingIdx, missingSchema bool
	for _, w := range warns {
		if strings.Contains(w.Msg, "DROP INDEX nope") {
			missingIdx = true
		}
		if strings.Contains(w.Msg, "DROP SCHEMA ghost") {
			missingSchema = true
		}
	}
	if !missingIdx {
		t.Errorf("expected warning for missing index, got %v", warns)
	}
	if !missingSchema {
		t.Errorf("expected warning for empty schema, got %v", warns)
	}
}

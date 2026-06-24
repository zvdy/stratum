package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunEndToEnd exercises the full CLI wiring (discover -> parse -> render ->
// write) without git or network: --push defaults to false.
func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	if err := os.Mkdir(mig, 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"001_orgs.sql": `CREATE TABLE orgs (
			id serial PRIMARY KEY,
			name varchar(255) NOT NULL
		);`,
		"002_users.sql": `CREATE TABLE users (
			id serial PRIMARY KEY,
			org_id int NOT NULL REFERENCES orgs (id)
		);
		CREATE UNIQUE INDEX uq_users_org ON users (org_id);`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(mig, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out := filepath.Join(dir, "OUT.md")
	opts := &options{
		migrationsPath: mig,
		output:         out,
		schemaName:     "public",
	}
	if err := run(opts); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	doc := string(data)
	// org_id is NOT NULL and carries a UNIQUE index, so the relation is a
	// mandatory one-to-one: |o (zero-or-one child) -- || (exactly-one parent).
	for _, want := range []string{"erDiagram", "users", "orgs", "users |o--|| orgs"} {
		if !strings.Contains(doc, want) {
			t.Errorf("output missing %q\n---\n%s", want, doc)
		}
	}
}

// TestRunMissingPathErrors verifies a non-existent migrations dir is fatal.
func TestRunMissingPathErrors(t *testing.T) {
	opts := &options{
		migrationsPath: filepath.Join(t.TempDir(), "does-not-exist"),
		output:         filepath.Join(t.TempDir(), "OUT.md"),
		schemaName:     "public",
	}
	if err := run(opts); err == nil {
		t.Errorf("expected error for missing migrations path")
	}
}

// TestRunVerboseFiltersSchema confirms --schema filtering drops non-matching
// tables from the output.
func TestRunVerboseFiltersSchema(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "001.sql"),
		[]byte(`CREATE TABLE billing.invoices (id serial PRIMARY KEY);
		        CREATE TABLE public.users (id serial PRIMARY KEY);`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "OUT.md")
	opts := &options{
		migrationsPath: dir,
		output:         out,
		schemaName:     "billing",
		verbose:        true,
	}
	if err := run(opts); err != nil {
		t.Fatalf("run: %v", err)
	}
	doc, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), "invoices") {
		t.Errorf("expected billing.invoices in output")
	}
	if strings.Contains(string(doc), "users") {
		t.Errorf("public.users should have been filtered out by --schema=billing")
	}
}

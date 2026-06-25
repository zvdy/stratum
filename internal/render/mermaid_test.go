package render

import (
	"strings"
	"testing"

	"github.com/zvdy/stratum/internal/schema"
)

// sampleSchema builds a small, fully-populated schema used by render tests.
func sampleSchema() *schema.Schema {
	s := schema.NewSchema()
	s.Tables["public.orgs"] = &schema.Table{
		Name: "public.orgs",
		Columns: []schema.Column{
			{Name: "id", Type: "int", NotNull: true, PrimaryKey: true},
			{Name: "name", Type: "varchar(255)", NotNull: true},
		},
	}
	s.Tables["public.users"] = &schema.Table{
		Name: "public.users",
		Columns: []schema.Column{
			{Name: "id", Type: "int", NotNull: true, PrimaryKey: true},
			{Name: "email", Type: "varchar(255)", NotNull: true},
			{Name: "org_id", Type: "int", NotNull: true},
		},
		Indexes: []schema.Index{
			{Name: "uq_users_email", Columns: []string{"email"}, Unique: true},
		},
		FKs: []schema.ForeignKey{
			{Name: "fk_org", Columns: []string{"org_id"}, RefTable: "public.orgs", RefColumns: []string{"id"}},
		},
	}
	return s
}

func TestMermaidExactOutput(t *testing.T) {
	got := Mermaid(sampleSchema())
	want := "```mermaid\n" +
		"erDiagram\n" +
		"    orgs {\n" +
		"        int id PK\n" +
		"        varchar_255 name\n" +
		"    }\n" +
		"    users {\n" +
		"        int id PK\n" +
		"        varchar_255 email \"unique\"\n" +
		"        int org_id FK\n" +
		"    }\n" +
		"    users }o--|| orgs : \"org_id\"\n" +
		"```"

	if got != want {
		t.Errorf("Mermaid output mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSanitizeType(t *testing.T) {
	tests := map[string]string{
		"varchar(255)":     "varchar_255",
		"numeric(10,2)":    "numeric_10_2",
		"double precision": "double_precision",
		"int":              "int",
		"text[]":           "text",
		"":                 "unknown",
	}
	for in, want := range tests {
		if got := sanitizeType(in); got != want {
			t.Errorf("sanitizeType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMermaidRelationCardinality(t *testing.T) {
	build := func(notNull, unique bool) string {
		s := schema.NewSchema()
		s.Tables["public.parent"] = &schema.Table{
			Name:    "public.parent",
			Columns: []schema.Column{{Name: "id", Type: "int", NotNull: true, PrimaryKey: true}},
		}
		child := &schema.Table{
			Name: "public.child",
			Columns: []schema.Column{
				{Name: "id", Type: "int", NotNull: true, PrimaryKey: true},
				{Name: "parent_id", Type: "int", NotNull: notNull},
			},
			FKs: []schema.ForeignKey{{Columns: []string{"parent_id"}, RefTable: "public.parent"}},
		}
		if unique {
			child.Indexes = append(child.Indexes, schema.Index{Columns: []string{"parent_id"}, Unique: true})
		}
		s.Tables["public.child"] = child
		return Mermaid(s)
	}

	cases := []struct {
		name            string
		notNull, unique bool
		want            string
	}{
		{"mandatory many-to-one", true, false, "child }o--|| parent"},
		{"optional many-to-one", false, false, "child }o--o| parent"},
		{"mandatory one-to-one", true, true, "child |o--|| parent"},
		{"optional one-to-one", false, true, "child |o--o| parent"},
	}
	for _, c := range cases {
		if got := build(c.notNull, c.unique); !strings.Contains(got, c.want) {
			t.Errorf("%s: want relation %q in:\n%s", c.name, c.want, got)
		}
	}
}

func TestMermaidPKFKMarker(t *testing.T) {
	s := schema.NewSchema()
	s.Tables["public.t"] = &schema.Table{
		Name: "public.t",
		Columns: []schema.Column{
			{Name: "id", Type: "int", PrimaryKey: true},
		},
		FKs: []schema.ForeignKey{
			{Columns: []string{"id"}, RefTable: "public.other"},
		},
	}
	got := Mermaid(s)
	if !strings.Contains(got, "int id PK,FK") {
		t.Errorf("expected combined PK,FK marker, got:\n%s", got)
	}
}

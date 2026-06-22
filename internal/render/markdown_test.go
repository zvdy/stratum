package render

import (
	"strings"
	"testing"
	"time"

	"github.com/zvdy/stratum/internal/schema"
)

func emptySchema() *schema.Schema { return schema.NewSchema() }

func TestMarkdownStructure(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	doc := Markdown(sampleSchema(), "db/migrations", now)

	required := []string{
		"# Schema",
		"2026-06-22T12:00:00Z",
		"`db/migrations`",
		"## Tables",
		"- [orgs](#orgs)",
		"- [users](#users)",
		"## ERD",
		"```mermaid",
		"erDiagram",
		"## users",
		"### Columns",
		"| Name | Type | Nullable | Default | Constraints |",
		"### Indexes",
		"| Name | Columns | Unique |",
		"### Foreign Keys",
		"| Name | Columns | References |",
	}
	for _, want := range required {
		if !strings.Contains(doc, want) {
			t.Errorf("markdown missing %q\n---\n%s", want, doc)
		}
	}
}

func TestMarkdownColumnRows(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	doc := Markdown(sampleSchema(), "src", now)

	// email is NOT NULL (nullable=no) and carries a unique constraint tag.
	if !strings.Contains(doc, "| email | `varchar(255)` | no |  | unique |") {
		t.Errorf("email row not rendered as expected:\n%s", doc)
	}
	// org_id is an FK.
	if !strings.Contains(doc, "| org_id | `int` | no |  | FK |") {
		t.Errorf("org_id row not rendered as expected:\n%s", doc)
	}
	// FK section references the orgs table with its column.
	if !strings.Contains(doc, "| fk_org | org_id | orgs (id) |") {
		t.Errorf("FK row not rendered as expected:\n%s", doc)
	}
}

func TestMarkdownEmptySchema(t *testing.T) {
	doc := Markdown(emptySchema(), "x", time.Unix(0, 0).UTC())
	if !strings.Contains(doc, "_No tables found._") {
		t.Errorf("expected empty-schema notice, got:\n%s", doc)
	}
}

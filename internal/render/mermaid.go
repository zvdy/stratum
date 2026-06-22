// Package render produces human-readable artifacts (Mermaid ERD, Markdown docs)
// from a schema.Schema IR. Output is deterministic: tables and relations are
// sorted so identical input always yields byte-identical output.
package render

import (
	"sort"
	"strings"

	"github.com/zvdy/stratum/internal/schema"
)

// Mermaid renders the schema as a fenced Mermaid erDiagram block.
func Mermaid(s *schema.Schema) string {
	var b strings.Builder
	b.WriteString("```mermaid\nerDiagram\n")

	for _, key := range s.SortedTableKeys() {
		t := s.Tables[key]
		writeEntity(&b, t)
	}

	for _, rel := range relations(s) {
		b.WriteString(rel)
		b.WriteByte('\n')
	}

	b.WriteString("```")
	return b.String()
}

// writeEntity emits one entity block: its name and column declarations.
func writeEntity(b *strings.Builder, t *schema.Table) {
	name := entityName(t.Name)
	b.WriteString("    ")
	b.WriteString(name)
	b.WriteString(" {\n")
	for _, c := range t.Columns {
		b.WriteString("        ")
		b.WriteString(sanitizeType(c.Type))
		b.WriteByte(' ')
		b.WriteString(c.Name)
		if marker := columnMarker(t, c); marker != "" {
			b.WriteByte(' ')
			b.WriteString(marker)
		}
		if note := columnNote(t, c); note != "" {
			b.WriteString(" \"")
			b.WriteString(note)
			b.WriteByte('"')
		}
		b.WriteByte('\n')
	}
	b.WriteString("    }\n")
}

// columnMarker returns the Mermaid attribute key (PK, FK, or PK,FK).
func columnMarker(t *schema.Table, c schema.Column) string {
	pk := c.PrimaryKey
	fk := t.IsFKColumn(c.Name)
	switch {
	case pk && fk:
		return "PK,FK"
	case pk:
		return "PK"
	case fk:
		return "FK"
	default:
		return ""
	}
}

// columnNote annotates a column that is covered by a single-column unique index.
func columnNote(t *schema.Table, c schema.Column) string {
	for _, idx := range t.Indexes {
		if idx.Unique && len(idx.Columns) == 1 && idx.Columns[0] == c.Name {
			return "unique"
		}
	}
	return ""
}

// relations builds the sorted list of relation lines, one per foreign key.
// Orientation follows the spec example: child }o--|| parent.
func relations(s *schema.Schema) []string {
	var lines []string
	for _, key := range s.SortedTableKeys() {
		t := s.Tables[key]
		child := entityName(t.Name)
		for _, fk := range t.FKs {
			if fk.RefTable == "" {
				continue
			}
			parent := entityName(fk.RefTable)
			label := strings.Join(fk.Columns, ", ")
			lines = append(lines, "    "+child+" }o--|| "+parent+" : \""+label+"\"")
		}
	}
	sort.Strings(lines)
	return lines
}

// entityName strips the default "public." prefix for readability while keeping
// non-default schema qualifiers (joined with an underscore so they remain a
// single valid Mermaid identifier).
func entityName(qualified string) string {
	sch := schema.SchemaOf(qualified)
	tbl := schema.TableOf(qualified)
	if sch == schema.DefaultSchema {
		return tbl
	}
	return sch + "_" + tbl
}

// sanitizeType makes a SQL type safe to use as a Mermaid attribute type token.
// Mermaid attribute types may not contain spaces, commas or parentheses, so
// those are replaced with underscores (collapsing repeats and trimming).
func sanitizeType(t string) string {
	if t == "" {
		return "unknown"
	}
	repl := strings.NewReplacer(
		" ", "_",
		"(", "_",
		")", "",
		",", "_",
		"[", "_",
		"]", "",
	)
	out := repl.Replace(t)
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	out = strings.Trim(out, "_")
	if out == "" {
		return "unknown"
	}
	return out
}

// Package schema defines the intermediate representation (IR) of a PostgreSQL
// schema reconstructed by replaying migration files. The IR is database- and
// parser-agnostic: it carries only the structural facts needed to render an ERD.
package schema

import (
	"sort"
	"strings"
)

// Schema is the top-level container. Tables are keyed by their fully-qualified
// name ("schema.table", e.g. "public.users").
type Schema struct {
	Tables map[string]*Table
}

// Table is a single relation along with the constraints and indexes attached to
// it. Name is the fully-qualified display name ("public.users").
type Table struct {
	Name    string
	Columns []Column
	Indexes []Index
	FKs     []ForeignKey
	Checks  []Check
	// PrimaryKeyName is the constraint name of the table's primary key, when one
	// was declared with an explicit name. It lets ALTER TABLE ... DROP CONSTRAINT
	// remove the primary key (whose membership is otherwise tracked as per-column
	// flags).
	PrimaryKeyName string
}

// Check is a CHECK constraint. Name is empty for anonymous checks. Expr is the
// whitespace-collapsed source of the check expression, when it could be captured.
type Check struct {
	Name string
	Expr string
}

// Column is a single attribute of a table.
type Column struct {
	Name       string
	Type       string
	NotNull    bool
	Default    string
	PrimaryKey bool
}

// Index describes a (possibly unique) index over one or more columns.
type Index struct {
	Name    string
	Columns []string
	Unique  bool
}

// ForeignKey describes a referential constraint from local Columns to
// RefColumns on RefTable. RefTable is fully qualified ("public.orgs").
type ForeignKey struct {
	Name       string
	Columns    []string
	RefTable   string
	RefColumns []string
}

// DefaultSchema is the schema assigned to unqualified table names.
const DefaultSchema = "public"

// NewSchema returns an empty, initialized Schema.
func NewSchema() *Schema {
	return &Schema{Tables: make(map[string]*Table)}
}

// Qualify normalizes a (schemaName, tableName) pair into a fully-qualified key.
// An empty schemaName defaults to DefaultSchema.
func Qualify(schemaName, tableName string) string {
	if schemaName == "" {
		schemaName = DefaultSchema
	}
	return schemaName + "." + tableName
}

// SchemaOf returns the schema portion of a qualified name.
func SchemaOf(qualified string) string {
	if i := strings.IndexByte(qualified, '.'); i >= 0 {
		return qualified[:i]
	}
	return DefaultSchema
}

// TableOf returns the bare table portion of a qualified name.
func TableOf(qualified string) string {
	if i := strings.IndexByte(qualified, '.'); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// GetOrCreateTable returns the table for key, creating an empty one if needed.
func (s *Schema) GetOrCreateTable(key string) *Table {
	if t, ok := s.Tables[key]; ok {
		return t
	}
	t := &Table{Name: key}
	s.Tables[key] = t
	return t
}

// Get returns the table for key and whether it exists.
func (s *Schema) Get(key string) (*Table, bool) {
	t, ok := s.Tables[key]
	return t, ok
}

// SortedTableKeys returns table keys in deterministic (sorted) order.
func (s *Schema) SortedTableKeys() []string {
	keys := make([]string, 0, len(s.Tables))
	for k := range s.Tables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Filter returns a new Schema containing only tables in the given schema name.
// Foreign keys that reference a table outside the filtered set are retained as
// declared (renderers decide how to draw dangling relations).
func (s *Schema) Filter(schemaName string) *Schema {
	if schemaName == "" {
		return s
	}
	out := NewSchema()
	for key, t := range s.Tables {
		if SchemaOf(key) == schemaName {
			out.Tables[key] = t
		}
	}
	return out
}

// Column returns a pointer to the named column and whether it exists.
func (t *Table) Column(name string) (*Column, bool) {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			return &t.Columns[i], true
		}
	}
	return nil, false
}

// AddColumn appends a column, replacing any existing column of the same name.
func (t *Table) AddColumn(c Column) {
	for i := range t.Columns {
		if t.Columns[i].Name == c.Name {
			t.Columns[i] = c
			return
		}
	}
	t.Columns = append(t.Columns, c)
}

// DropColumn removes the named column if present and reports whether it existed.
func (t *Table) DropColumn(name string) bool {
	for i := range t.Columns {
		if t.Columns[i].Name == name {
			t.Columns = append(t.Columns[:i], t.Columns[i+1:]...)
			return true
		}
	}
	return false
}

// RenameColumn renames a column in place across the column list, indexes and
// foreign keys so the IR stays consistent.
func (t *Table) RenameColumn(from, to string) bool {
	renamed := false
	for i := range t.Columns {
		if t.Columns[i].Name == from {
			t.Columns[i].Name = to
			renamed = true
		}
	}
	for i := range t.Indexes {
		for j := range t.Indexes[i].Columns {
			if t.Indexes[i].Columns[j] == from {
				t.Indexes[i].Columns[j] = to
			}
		}
	}
	for i := range t.FKs {
		for j := range t.FKs[i].Columns {
			if t.FKs[i].Columns[j] == from {
				t.FKs[i].Columns[j] = to
			}
		}
	}
	return renamed
}

// IsFKColumn reports whether the named column participates in any foreign key.
func (t *Table) IsFKColumn(name string) bool {
	for _, fk := range t.FKs {
		for _, c := range fk.Columns {
			if c == name {
				return true
			}
		}
	}
	return false
}

// DropIndexByName removes an index of the given (bare) name from the table and
// reports whether one was found.
func (t *Table) DropIndexByName(name string) bool {
	for i := range t.Indexes {
		if t.Indexes[i].Name == name {
			t.Indexes = append(t.Indexes[:i], t.Indexes[i+1:]...)
			return true
		}
	}
	return false
}

// DropConstraintByName removes a foreign key, (unique) index, check, or the
// primary key matching the constraint name and reports whether anything was
// removed. Constraint and index names share a namespace in Postgres, so all are
// checked.
func (t *Table) DropConstraintByName(name string) bool {
	if name == "" {
		return false
	}
	removed := false
	for i := range t.FKs {
		if t.FKs[i].Name == name {
			t.FKs = append(t.FKs[:i], t.FKs[i+1:]...)
			removed = true
			break
		}
	}
	if t.DropIndexByName(name) {
		removed = true
	}
	for i := range t.Checks {
		if t.Checks[i].Name == name {
			t.Checks = append(t.Checks[:i], t.Checks[i+1:]...)
			removed = true
			break
		}
	}
	if t.PrimaryKeyName == name {
		// Dropping the PK constraint clears the key membership but leaves the
		// columns' NOT NULL in place (Postgres keeps NOT NULL as a separate
		// constraint).
		for i := range t.Columns {
			t.Columns[i].PrimaryKey = false
		}
		t.PrimaryKeyName = ""
		removed = true
	}
	return removed
}

// DropIndexByName removes the first index with the given bare name across all
// tables and reports whether one was found.
func (s *Schema) DropIndexByName(name string) bool {
	for _, key := range s.SortedTableKeys() {
		if s.Tables[key].DropIndexByName(name) {
			return true
		}
	}
	return false
}

// DropSchema removes every table belonging to schemaName and returns how many
// were removed.
func (s *Schema) DropSchema(schemaName string) int {
	n := 0
	for key := range s.Tables {
		if SchemaOf(key) == schemaName {
			delete(s.Tables, key)
			n++
		}
	}
	return n
}

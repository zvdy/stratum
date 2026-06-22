package schema

import (
	"reflect"
	"testing"
)

func TestQualifyAndSplit(t *testing.T) {
	tests := []struct {
		schemaName, table, want string
		wantSchema, wantTable   string
	}{
		{"", "users", "public.users", "public", "users"},
		{"billing", "invoices", "billing.invoices", "billing", "invoices"},
	}
	for _, tt := range tests {
		got := Qualify(tt.schemaName, tt.table)
		if got != tt.want {
			t.Errorf("Qualify(%q,%q) = %q, want %q", tt.schemaName, tt.table, got, tt.want)
		}
		if s := SchemaOf(got); s != tt.wantSchema {
			t.Errorf("SchemaOf(%q) = %q, want %q", got, s, tt.wantSchema)
		}
		if tb := TableOf(got); tb != tt.wantTable {
			t.Errorf("TableOf(%q) = %q, want %q", got, tb, tt.wantTable)
		}
	}

	// Unqualified names default to the public schema.
	if SchemaOf("users") != DefaultSchema {
		t.Errorf("SchemaOf unqualified = %q, want %q", SchemaOf("users"), DefaultSchema)
	}
	if TableOf("users") != "users" {
		t.Errorf("TableOf unqualified = %q, want users", TableOf("users"))
	}
}

func TestGetOrCreateAndGet(t *testing.T) {
	s := NewSchema()
	t1 := s.GetOrCreateTable("public.users")
	t2 := s.GetOrCreateTable("public.users")
	if t1 != t2 {
		t.Errorf("GetOrCreateTable returned different pointers for same key")
	}
	if got, ok := s.Get("public.users"); !ok || got != t1 {
		t.Errorf("Get did not return the created table")
	}
	if _, ok := s.Get("public.missing"); ok {
		t.Errorf("Get returned ok for missing table")
	}
}

func TestAddColumnReplaces(t *testing.T) {
	tbl := &Table{Name: "public.t"}
	tbl.AddColumn(Column{Name: "id", Type: "int"})
	tbl.AddColumn(Column{Name: "name", Type: "text"})
	tbl.AddColumn(Column{Name: "id", Type: "bigint"}) // replace existing

	if len(tbl.Columns) != 2 {
		t.Fatalf("got %d columns, want 2: %+v", len(tbl.Columns), tbl.Columns)
	}
	if c, _ := tbl.Column("id"); c.Type != "bigint" {
		t.Errorf("id type = %q, want bigint (should have replaced)", c.Type)
	}
}

func TestDropColumn(t *testing.T) {
	tbl := &Table{Columns: []Column{{Name: "a"}, {Name: "b"}}}
	if !tbl.DropColumn("a") {
		t.Errorf("DropColumn(a) = false, want true")
	}
	if tbl.DropColumn("missing") {
		t.Errorf("DropColumn(missing) = true, want false")
	}
	if _, ok := tbl.Column("a"); ok {
		t.Errorf("column a should be gone")
	}
}

func TestRenameColumnUpdatesIndexesAndFKs(t *testing.T) {
	tbl := &Table{
		Columns: []Column{{Name: "old"}},
		Indexes: []Index{{Name: "i", Columns: []string{"old"}}},
		FKs:     []ForeignKey{{Columns: []string{"old"}, RefTable: "public.other"}},
	}
	if !tbl.RenameColumn("old", "new") {
		t.Fatalf("RenameColumn returned false")
	}
	if _, ok := tbl.Column("new"); !ok {
		t.Errorf("column not renamed")
	}
	if tbl.Indexes[0].Columns[0] != "new" {
		t.Errorf("index column not renamed: %v", tbl.Indexes[0].Columns)
	}
	if tbl.FKs[0].Columns[0] != "new" {
		t.Errorf("fk column not renamed: %v", tbl.FKs[0].Columns)
	}
	if tbl.RenameColumn("absent", "x") {
		t.Errorf("RenameColumn(absent) = true, want false")
	}
}

func TestFilterAndSortedKeys(t *testing.T) {
	s := NewSchema()
	s.GetOrCreateTable("public.users")
	s.GetOrCreateTable("billing.invoices")
	s.GetOrCreateTable("public.orgs")

	if got := s.SortedTableKeys(); !reflect.DeepEqual(got,
		[]string{"billing.invoices", "public.orgs", "public.users"}) {
		t.Errorf("SortedTableKeys = %v", got)
	}

	pub := s.Filter("public")
	if len(pub.Tables) != 2 {
		t.Errorf("Filter(public) kept %d tables, want 2", len(pub.Tables))
	}
	if _, ok := pub.Get("billing.invoices"); ok {
		t.Errorf("Filter(public) should drop billing.invoices")
	}

	// Empty filter is a no-op passthrough.
	if s.Filter("") != s {
		t.Errorf("Filter(\"\") should return the same schema")
	}
}

func TestIsFKColumn(t *testing.T) {
	tbl := &Table{FKs: []ForeignKey{{Columns: []string{"org_id"}}}}
	if !tbl.IsFKColumn("org_id") {
		t.Errorf("org_id should be an FK column")
	}
	if tbl.IsFKColumn("id") {
		t.Errorf("id should not be an FK column")
	}
}

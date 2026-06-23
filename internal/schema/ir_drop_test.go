package schema

import "testing"

func TestTableDropIndexByName(t *testing.T) {
	tbl := &Table{Indexes: []Index{
		{Name: "a"}, {Name: "b"},
	}}
	if !tbl.DropIndexByName("a") {
		t.Errorf("DropIndexByName(a) = false, want true")
	}
	if tbl.DropIndexByName("missing") {
		t.Errorf("DropIndexByName(missing) = true, want false")
	}
	if len(tbl.Indexes) != 1 || tbl.Indexes[0].Name != "b" {
		t.Errorf("indexes after drop = %+v", tbl.Indexes)
	}
}

func TestTableDropConstraintByName(t *testing.T) {
	tbl := &Table{
		FKs:     []ForeignKey{{Name: "fk1"}},
		Indexes: []Index{{Name: "uq1", Unique: true}},
	}
	if !tbl.DropConstraintByName("fk1") {
		t.Errorf("dropping fk1 should succeed")
	}
	if len(tbl.FKs) != 0 {
		t.Errorf("fk1 not removed: %+v", tbl.FKs)
	}
	if !tbl.DropConstraintByName("uq1") {
		t.Errorf("dropping uq1 should succeed")
	}
	if len(tbl.Indexes) != 0 {
		t.Errorf("uq1 not removed: %+v", tbl.Indexes)
	}
	if tbl.DropConstraintByName("nope") {
		t.Errorf("dropping unknown constraint should return false")
	}
}

func TestSchemaDropIndexByName(t *testing.T) {
	s := NewSchema()
	u := s.GetOrCreateTable("public.users")
	u.Indexes = []Index{{Name: "idx_users"}}
	if !s.DropIndexByName("idx_users") {
		t.Errorf("schema-level DropIndexByName should find the index")
	}
	if len(u.Indexes) != 0 {
		t.Errorf("index not removed: %+v", u.Indexes)
	}
	if s.DropIndexByName("idx_users") {
		t.Errorf("second drop should return false")
	}
}

func TestSchemaDropSchema(t *testing.T) {
	s := NewSchema()
	s.GetOrCreateTable("auth.users")
	s.GetOrCreateTable("auth.sessions")
	s.GetOrCreateTable("public.orders")
	if n := s.DropSchema("auth"); n != 2 {
		t.Errorf("DropSchema(auth) = %d, want 2", n)
	}
	if _, ok := s.Get("public.orders"); !ok {
		t.Errorf("public.orders should survive")
	}
	if n := s.DropSchema("auth"); n != 0 {
		t.Errorf("second DropSchema(auth) = %d, want 0", n)
	}
}

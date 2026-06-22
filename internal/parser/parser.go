// Package parser turns PostgreSQL migration files into a schema.Schema IR by
// parsing each file with pg_query_go (the real Postgres grammar) and replaying
// the resulting statements in order. No database connection is used or required.
//
// The parser is deliberately defensive: statement and AST node types it does not
// model produce a Warning and are skipped — they never panic and never abort the
// run. Only a hard parse failure (invalid SQL) is fatal.
package parser

import (
	"fmt"
	"os"
	"strings"

	pg "github.com/pganalyze/pg_query_go/v5"
	"github.com/zvdy/stratum/internal/schema"
)

// Warning is a non-fatal diagnostic emitted during parsing.
type Warning struct {
	File string // source file, if applicable
	Msg  string
}

func (w Warning) String() string {
	if w.File != "" {
		return fmt.Sprintf("%s: %s", w.File, w.Msg)
	}
	return w.Msg
}

// Options controls parsing behavior.
type Options struct {
	// Verbose causes each parsed statement to be reported as a Warning-level
	// note via the OnStatement callback (used by the CLI for --verbose).
	Verbose bool
	// OnStatement, if set, is invoked with a short description of each
	// successfully handled statement. Used for verbose logging.
	OnStatement func(file, desc string)
}

// parser carries mutable state across a single Parse invocation.
type parser struct {
	schema   *schema.Schema
	warnings []Warning
	opts     Options
	file     string
}

// ParseFiles replays the given files (in order) into a fresh schema.Schema.
// It returns the IR, any non-fatal warnings, and a fatal error only if a file
// cannot be read or contains unparseable SQL.
func ParseFiles(files []string, opts Options) (*schema.Schema, []Warning, error) {
	p := &parser{schema: schema.NewSchema(), opts: opts}

	for _, f := range files {
		if err := p.parseFile(f); err != nil {
			return nil, p.warnings, err
		}
	}

	p.resolveForeignKeys()
	return p.schema, p.warnings, nil
}

// ParseSQL replays a single SQL string (used by tests). Errors on invalid SQL.
func ParseSQL(sql string, opts Options) (*schema.Schema, []Warning, error) {
	p := &parser{schema: schema.NewSchema(), opts: opts, file: "<inline>"}
	if err := p.parseString(sql); err != nil {
		return nil, p.warnings, err
	}
	p.resolveForeignKeys()
	return p.schema, p.warnings, nil
}

func (p *parser) warn(format string, args ...any) {
	p.warnings = append(p.warnings, Warning{File: p.file, Msg: fmt.Sprintf(format, args...)})
}

func (p *parser) note(desc string) {
	if p.opts.Verbose && p.opts.OnStatement != nil {
		p.opts.OnStatement(p.file, desc)
	}
}

func (p *parser) parseFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	prev := p.file
	p.file = path
	defer func() { p.file = prev }()

	if strings.TrimSpace(string(data)) == "" {
		p.warn("empty migration file")
		return nil
	}
	return p.parseString(string(data))
}

func (p *parser) parseString(sql string) error {
	tree, err := pg.Parse(sql)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", p.file, err)
	}
	for _, raw := range tree.Stmts {
		if raw.Stmt == nil {
			continue
		}
		p.dispatch(raw.Stmt)
	}
	return nil
}

// dispatch routes a top-level statement node to its handler. Unknown node types
// warn and continue.
func (p *parser) dispatch(node *pg.Node) {
	switch n := node.Node.(type) {
	case *pg.Node_CreateStmt:
		p.handleCreateTable(n.CreateStmt)
	case *pg.Node_AlterTableStmt:
		p.handleAlterTable(n.AlterTableStmt)
	case *pg.Node_IndexStmt:
		p.handleCreateIndex(n.IndexStmt)
	case *pg.Node_DropStmt:
		p.handleDrop(n.DropStmt)
	case *pg.Node_RenameStmt:
		p.handleRename(n.RenameStmt)
	case *pg.Node_InsertStmt, *pg.Node_UpdateStmt, *pg.Node_DeleteStmt,
		*pg.Node_SelectStmt, *pg.Node_DoStmt, *pg.Node_CommentStmt,
		*pg.Node_VariableSetStmt, *pg.Node_TransactionStmt,
		*pg.Node_CreateFunctionStmt, *pg.Node_CreateTrigStmt,
		*pg.Node_CreateSeqStmt, *pg.Node_CreateEnumStmt,
		*pg.Node_CreateSchemaStmt, *pg.Node_GrantStmt:
		// Intentionally ignored DML / procedural / non-structural statements.
		p.note(fmt.Sprintf("skipped %T", n))
	default:
		p.warn("unhandled statement type %T (skipped)", n)
	}
}

// --- CREATE TABLE ----------------------------------------------------------

func (p *parser) handleCreateTable(stmt *pg.CreateStmt) {
	if stmt.Relation == nil {
		p.warn("CREATE TABLE without relation (skipped)")
		return
	}
	key := rangeVarKey(stmt.Relation)
	t := p.schema.GetOrCreateTable(key)
	p.note("CREATE TABLE " + key)

	for _, elt := range stmt.TableElts {
		switch e := elt.Node.(type) {
		case *pg.Node_ColumnDef:
			p.addColumnDef(t, e.ColumnDef)
		case *pg.Node_Constraint:
			p.applyTableConstraint(t, e.Constraint)
		default:
			p.warn("CREATE TABLE %s: unhandled element %T (skipped)", key, e)
		}
	}
}

// addColumnDef appends a column and processes its inline constraints.
func (p *parser) addColumnDef(t *schema.Table, cd *pg.ColumnDef) {
	col := schema.Column{
		Name:    cd.Colname,
		Type:    typeName(cd.TypeName),
		NotNull: cd.IsNotNull,
	}
	for _, c := range cd.Constraints {
		con := c.GetConstraint()
		if con == nil {
			continue
		}
		switch con.Contype {
		case pg.ConstrType_CONSTR_NOTNULL:
			col.NotNull = true
		case pg.ConstrType_CONSTR_PRIMARY:
			col.PrimaryKey = true
			col.NotNull = true
		case pg.ConstrType_CONSTR_DEFAULT:
			col.Default = exprText(con.RawExpr)
		case pg.ConstrType_CONSTR_CHECK:
			t.Checks = append(t.Checks, exprText(con.RawExpr))
		case pg.ConstrType_CONSTR_FOREIGN:
			// Inline column-level REFERENCES: the local column is this column.
			fk := foreignKeyFrom(con)
			if len(fk.Columns) == 0 {
				fk.Columns = []string{cd.Colname}
			}
			t.FKs = append(t.FKs, fk)
		case pg.ConstrType_CONSTR_UNIQUE:
			t.Indexes = append(t.Indexes, schema.Index{
				Name:    con.Conname,
				Columns: []string{cd.Colname},
				Unique:  true,
			})
		}
	}
	t.AddColumn(col)
}

// applyTableConstraint handles table-level constraints inside CREATE TABLE.
func (p *parser) applyTableConstraint(t *schema.Table, con *pg.Constraint) {
	switch con.Contype {
	case pg.ConstrType_CONSTR_PRIMARY:
		for _, name := range stringList(con.Keys) {
			if col, ok := t.Column(name); ok {
				col.PrimaryKey = true
				col.NotNull = true
			}
		}
	case pg.ConstrType_CONSTR_FOREIGN:
		t.FKs = append(t.FKs, foreignKeyFrom(con))
	case pg.ConstrType_CONSTR_UNIQUE:
		t.Indexes = append(t.Indexes, schema.Index{
			Name:    con.Conname,
			Columns: stringList(con.Keys),
			Unique:  true,
		})
	case pg.ConstrType_CONSTR_CHECK:
		t.Checks = append(t.Checks, exprText(con.RawExpr))
	}
}

// --- ALTER TABLE -----------------------------------------------------------

func (p *parser) handleAlterTable(stmt *pg.AlterTableStmt) {
	if stmt.Relation == nil {
		p.warn("ALTER TABLE without relation (skipped)")
		return
	}
	key := rangeVarKey(stmt.Relation)
	t := p.schema.GetOrCreateTable(key)

	for _, c := range stmt.Cmds {
		cmd := c.GetAlterTableCmd()
		if cmd == nil {
			continue
		}
		switch cmd.Subtype {
		case pg.AlterTableType_AT_AddColumn:
			if cd := cmd.Def.GetColumnDef(); cd != nil {
				p.addColumnDef(t, cd)
				p.note(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", key, cd.Colname))
			}
		case pg.AlterTableType_AT_DropColumn:
			if !t.DropColumn(cmd.Name) {
				p.warn("ALTER TABLE %s DROP COLUMN %s: column not found", key, cmd.Name)
			} else {
				p.note(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", key, cmd.Name))
			}
		case pg.AlterTableType_AT_AlterColumnType:
			if cd := cmd.Def.GetColumnDef(); cd != nil {
				if col, ok := t.Column(cmd.Name); ok {
					col.Type = typeName(cd.TypeName)
					p.note(fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE", key, cmd.Name))
				}
			}
		case pg.AlterTableType_AT_AddConstraint:
			if con := cmd.Def.GetConstraint(); con != nil {
				p.applyTableConstraint(t, con)
				p.note(fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT", key))
			}
		case pg.AlterTableType_AT_ColumnDefault:
			if col, ok := t.Column(cmd.Name); ok {
				col.Default = exprText(cmd.Def)
			}
		case pg.AlterTableType_AT_DropConstraint:
			p.note(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s (noted)", key, cmd.Name))
		default:
			p.warn("ALTER TABLE %s: unhandled subtype %v (skipped)", key, cmd.Subtype)
		}
	}
}

// --- CREATE INDEX ----------------------------------------------------------

func (p *parser) handleCreateIndex(stmt *pg.IndexStmt) {
	if stmt.Relation == nil {
		p.warn("CREATE INDEX without relation (skipped)")
		return
	}
	key := rangeVarKey(stmt.Relation)
	t := p.schema.GetOrCreateTable(key)

	idx := schema.Index{Name: stmt.Idxname, Unique: stmt.Unique}
	for _, param := range stmt.IndexParams {
		if ie := param.GetIndexElem(); ie != nil && ie.Name != "" {
			idx.Columns = append(idx.Columns, ie.Name)
		}
	}
	t.Indexes = append(t.Indexes, idx)
	p.note(fmt.Sprintf("CREATE INDEX %s ON %s", stmt.Idxname, key))
}

// --- DROP ------------------------------------------------------------------

func (p *parser) handleDrop(stmt *pg.DropStmt) {
	if stmt.RemoveType != pg.ObjectType_OBJECT_TABLE {
		p.note(fmt.Sprintf("DROP %v (skipped)", stmt.RemoveType))
		return
	}
	for _, obj := range stmt.Objects {
		key := nameListKey(obj)
		if key == "" {
			continue
		}
		if _, ok := p.schema.Tables[key]; ok {
			delete(p.schema.Tables, key)
			p.note("DROP TABLE " + key)
		} else {
			p.warn("DROP TABLE %s: table not found", key)
		}
	}
}

// --- RENAME ----------------------------------------------------------------

func (p *parser) handleRename(stmt *pg.RenameStmt) {
	switch stmt.RenameType {
	case pg.ObjectType_OBJECT_COLUMN:
		if stmt.Relation == nil {
			return
		}
		key := rangeVarKey(stmt.Relation)
		if t, ok := p.schema.Get(key); ok {
			if t.RenameColumn(stmt.Subname, stmt.Newname) {
				p.note(fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", key, stmt.Subname, stmt.Newname))
			} else {
				p.warn("RENAME COLUMN on %s: column %s not found", key, stmt.Subname)
			}
		}
	case pg.ObjectType_OBJECT_TABLE:
		if stmt.Relation == nil {
			return
		}
		oldKey := rangeVarKey(stmt.Relation)
		newKey := schema.Qualify(stmt.Relation.Schemaname, stmt.Newname)
		t, ok := p.schema.Get(oldKey)
		if !ok {
			p.warn("RENAME TABLE %s: table not found", oldKey)
			return
		}
		delete(p.schema.Tables, oldKey)
		t.Name = newKey
		p.schema.Tables[newKey] = t
		p.note(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", oldKey, newKey))
	default:
		p.note(fmt.Sprintf("RENAME %v (skipped)", stmt.RenameType))
	}
}

// --- FK resolution ---------------------------------------------------------

// resolveForeignKeys does a second pass warning about FKs whose referenced table
// is not present in the schema. It is non-fatal by design.
func (p *parser) resolveForeignKeys() {
	for _, key := range p.schema.SortedTableKeys() {
		t := p.schema.Tables[key]
		for _, fk := range t.FKs {
			if fk.RefTable == "" {
				continue
			}
			if _, ok := p.schema.Tables[fk.RefTable]; !ok {
				p.warn("table %s: foreign key references unknown table %s (dangling)", key, fk.RefTable)
			}
		}
	}
}

// --- helpers ---------------------------------------------------------------

// rangeVarKey builds a qualified key from a RangeVar.
func rangeVarKey(rv *pg.RangeVar) string {
	return schema.Qualify(rv.Schemaname, rv.Relname)
}

// foreignKeyFrom extracts a ForeignKey from a CONSTR_FOREIGN constraint.
func foreignKeyFrom(con *pg.Constraint) schema.ForeignKey {
	fk := schema.ForeignKey{
		Name:       con.Conname,
		Columns:    stringList(con.FkAttrs),
		RefColumns: stringList(con.PkAttrs),
	}
	if con.Pktable != nil {
		fk.RefTable = rangeVarKey(con.Pktable)
	}
	return fk
}

// typeName renders a TypeName node into a readable type string, appending the
// length/precision modifier when present (e.g. "varchar(255)", "numeric(10,2)").
func typeName(tn *pg.TypeName) string {
	if tn == nil {
		return ""
	}
	parts := stringList(tn.Names)
	// Postgres prefixes built-in types with the "pg_catalog" schema; drop it.
	if len(parts) > 1 && parts[0] == "pg_catalog" {
		parts = parts[1:]
	}
	base := normalizeType(strings.Join(parts, "."))

	var mods []string
	for _, m := range tn.Typmods {
		if ac := m.GetAConst(); ac != nil {
			if iv := ac.GetIval(); iv != nil {
				mods = append(mods, fmt.Sprintf("%d", iv.Ival))
			}
		}
	}
	out := base
	if len(mods) > 0 {
		out += "(" + strings.Join(mods, ",") + ")"
	}
	if len(tn.ArrayBounds) > 0 {
		out += "[]"
	}
	return out
}

// normalizeType maps Postgres internal type names to their conventional SQL
// spellings for friendlier output.
func normalizeType(t string) string {
	switch t {
	case "int4":
		return "int"
	case "int8":
		return "bigint"
	case "int2":
		return "smallint"
	case "float4":
		return "real"
	case "float8":
		return "double precision"
	case "bool":
		return "boolean"
	case "bpchar":
		return "char"
	case "varchar":
		return "varchar"
	case "timestamptz":
		return "timestamptz"
	case "timestamp":
		return "timestamp"
	default:
		return t
	}
}

// stringList extracts string values from a list of String nodes.
func stringList(nodes []*pg.Node) []string {
	var out []string
	for _, n := range nodes {
		if s := n.GetString_(); s != nil {
			out = append(out, s.Sval)
		}
	}
	return out
}

// nameListKey turns a DROP object (a List of String nodes like {schema, table})
// into a qualified key.
func nameListKey(obj *pg.Node) string {
	list := obj.GetList()
	if list == nil {
		return ""
	}
	parts := stringList(list.Items)
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return schema.Qualify("", parts[0])
	default:
		return schema.Qualify(parts[len(parts)-2], parts[len(parts)-1])
	}
}

// exprText renders a constraint/default expression to an approximate textual
// form. It handles the common constant and function-call cases; anything else
// degrades to a placeholder rather than failing.
func exprText(node *pg.Node) string {
	if node == nil {
		return ""
	}
	switch n := node.Node.(type) {
	case *pg.Node_AConst:
		ac := n.AConst
		switch v := ac.Val.(type) {
		case *pg.A_Const_Sval:
			return "'" + v.Sval.Sval + "'"
		case *pg.A_Const_Ival:
			return fmt.Sprintf("%d", v.Ival.Ival)
		case *pg.A_Const_Fval:
			return v.Fval.Fval
		case *pg.A_Const_Boolval:
			if v.Boolval.Boolval {
				return "true"
			}
			return "false"
		}
		if ac.Isnull {
			return "null"
		}
		return ""
	case *pg.Node_AExpr:
		op := strings.Join(stringList(n.AExpr.Name), "")
		return strings.TrimSpace(exprText(n.AExpr.Lexpr) + " " + op + " " + exprText(n.AExpr.Rexpr))
	case *pg.Node_BoolExpr:
		var sep string
		switch n.BoolExpr.Boolop {
		case pg.BoolExprType_OR_EXPR:
			sep = " OR "
		case pg.BoolExprType_NOT_EXPR:
			sep = "NOT "
		default:
			sep = " AND "
		}
		parts := make([]string, 0, len(n.BoolExpr.Args))
		for _, a := range n.BoolExpr.Args {
			parts = append(parts, exprText(a))
		}
		return strings.Join(parts, sep)
	case *pg.Node_FuncCall:
		return strings.Join(stringList(n.FuncCall.Funcname), ".") + "()"
	case *pg.Node_TypeCast:
		return exprText(n.TypeCast.Arg)
	case *pg.Node_ColumnRef:
		return strings.Join(stringList(n.ColumnRef.Fields), ".")
	case *pg.Node_SqlvalueFunction:
		return "CURRENT"
	default:
		return ""
	}
}

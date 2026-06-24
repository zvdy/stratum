// Package parser turns PostgreSQL migration files into a schema.Schema IR by
// tokenizing each file (see lexer.go) and replaying the DDL statements in order.
// It uses no database connection and no third-party parser — just a small,
// hand-written recursive-descent parser over the subset of DDL an ERD needs:
// CREATE/ALTER/DROP TABLE, CREATE [UNIQUE] INDEX, DROP INDEX, DROP SCHEMA, and
// the constraints/columns they carry.
//
// It is deliberately defensive: statements and clauses it does not model are
// skipped (to the next top-level ';') rather than failing, so real-world
// migrations containing DML, DO blocks, functions, etc. parse cleanly. Only a
// malformed *recognized* statement (e.g. "CREATE TABLE (") is fatal.
package parser

import (
	"fmt"
	"os"
	"strings"

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
	// Verbose causes each parsed statement to be reported via OnStatement.
	Verbose bool
	// OnStatement, if set, is invoked with a short description of each handled
	// statement. Used by the CLI for --verbose logging.
	OnStatement func(file, desc string)
}

// parseFatal is panicked on a fatal parse error and recovered at the file
// boundary, turning it into a returned error.
type parseFatal struct{ msg string }

// parser carries mutable state across a single Parse invocation.
type parser struct {
	schema   *schema.Schema
	warnings []Warning
	opts     Options
	file     string

	src  string
	toks []token
	i    int
}

// ParseFiles replays the given files (in order) into a fresh schema.Schema.
// It returns the IR, any non-fatal warnings, and a fatal error only if a file
// cannot be read or contains a malformed recognized statement.
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

// ParseSQL replays a single SQL string (used by tests).
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

func (p *parser) fatalf(format string, args ...any) {
	panic(parseFatal{msg: fmt.Sprintf(format, args...)})
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

func (p *parser) parseString(sql string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if pf, ok := r.(parseFatal); ok {
				err = fmt.Errorf("parsing %s: %s", p.file, pf.msg)
				return
			}
			panic(r)
		}
	}()

	p.src = sql
	p.toks = lex(sql)
	p.i = 0

	for !p.atEnd() {
		if p.isPunct(";") {
			p.advance()
			continue
		}
		p.parseStatement()
	}
	return nil
}

// --- token cursor helpers --------------------------------------------------

func (p *parser) cur() token { return p.toks[p.i] }

func (p *parser) peek(n int) token {
	j := p.i + n
	if j >= len(p.toks) {
		return p.toks[len(p.toks)-1] // tEOF
	}
	return p.toks[j]
}

func (p *parser) advance() {
	if p.i < len(p.toks)-1 {
		p.i++
	}
}

func (p *parser) atEnd() bool { return p.cur().kind == tEOF }

// isWord reports whether the current token is the (unquoted) keyword s.
func (p *parser) isWord(s string) bool {
	t := p.cur()
	return t.kind == tWord && !t.quoted && t.val == s
}

func (p *parser) acceptWord(s string) bool {
	if p.isWord(s) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) isPunct(s string) bool {
	t := p.cur()
	return t.kind == tPunct && t.val == s
}

func (p *parser) acceptPunct(s string) bool {
	if p.isPunct(s) {
		p.advance()
		return true
	}
	return false
}

// isTerminator reports whether the current token ends a list element at the
// given relative paren depth (top-level ',', ')' or ';').
func (p *parser) isTerminator(depth int) bool {
	if depth != 0 {
		return false
	}
	return p.isPunct(",") || p.isPunct(")") || p.isPunct(";") || p.atEnd()
}

// skipStatement consumes tokens up to and including the next top-level ';'.
func (p *parser) skipStatement() {
	depth := 0
	for !p.atEnd() {
		t := p.cur()
		if t.kind == tPunct {
			switch t.val {
			case "(", "[":
				depth++
			case ")", "]":
				if depth > 0 {
					depth--
				}
			case ";":
				if depth == 0 {
					p.advance()
					return
				}
			}
		}
		p.advance()
	}
}

// skipElement consumes tokens until a top-level element terminator (',', ')'
// or ';'), leaving that terminator unconsumed.
func (p *parser) skipElement() {
	depth := 0
	for !p.atEnd() {
		if p.isTerminator(depth) {
			return
		}
		t := p.cur()
		if t.kind == tPunct {
			switch t.val {
			case "(", "[":
				depth++
			case ")", "]":
				depth--
			}
		}
		p.advance()
	}
}

// --- name parsing ----------------------------------------------------------

// nameParts reads a dotted identifier sequence (a, a.b, a.b.c). Returns nil if
// the current token is not an identifier.
func (p *parser) nameParts() []string {
	if p.cur().kind != tWord {
		return nil
	}
	parts := []string{p.cur().val}
	p.advance()
	for p.isPunct(".") {
		p.advance()
		if p.cur().kind != tWord {
			break
		}
		parts = append(parts, p.cur().val)
		p.advance()
	}
	return parts
}

// qualifiedKey turns dotted name parts into a "schema.table" key, defaulting the
// schema to public.
func qualifiedKey(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return schema.Qualify("", parts[0])
	default:
		return schema.Qualify(parts[len(parts)-2], parts[len(parts)-1])
	}
}

// requireName reads a (possibly qualified) relation name or fails fatally.
func (p *parser) requireName(context string) string {
	parts := p.nameParts()
	if len(parts) == 0 {
		p.fatalf("%s: expected a name, got %q", context, p.cur().val)
	}
	return qualifiedKey(parts)
}

// ident reads a single bare identifier, or "" if the current token isn't one.
func (p *parser) ident() string {
	if p.cur().kind != tWord {
		return ""
	}
	v := p.cur().val
	p.advance()
	return v
}

// --- statement dispatch ----------------------------------------------------

func (p *parser) parseStatement() {
	switch {
	case p.isWord("create"):
		p.parseCreate()
	case p.isWord("alter"):
		p.advance()
		if p.acceptWord("table") {
			p.parseAlterTable()
		} else {
			p.skipStatement()
		}
	case p.isWord("drop"):
		p.advance()
		p.parseDrop()
	default:
		p.note("skipped " + p.cur().val)
		p.skipStatement()
	}
}

func (p *parser) parseCreate() {
	p.advance() // CREATE
	// Skip table-storage modifiers.
	for p.isWord("temp") || p.isWord("temporary") || p.isWord("unlogged") ||
		p.isWord("global") || p.isWord("local") {
		p.advance()
	}
	switch {
	case p.acceptWord("table"):
		p.parseCreateTable()
	case p.isWord("unique"):
		p.advance()
		if p.acceptWord("index") {
			p.parseCreateIndex(true)
		} else {
			p.skipStatement()
		}
	case p.acceptWord("index"):
		p.parseCreateIndex(false)
	default:
		p.note("skipped CREATE " + p.cur().val)
		p.skipStatement()
	}
}

// --- CREATE TABLE ----------------------------------------------------------

func (p *parser) parseCreateTable() {
	p.acceptIfNotExists()
	key := p.requireName("CREATE TABLE")
	t := p.schema.GetOrCreateTable(key)
	p.note("CREATE TABLE " + key)

	if !p.isPunct("(") {
		// CREATE TABLE ... AS / PARTITION OF / OF type — nothing to model.
		p.skipStatement()
		return
	}
	p.advance() // '('

	for !p.atEnd() {
		if p.acceptPunct(")") {
			break
		}
		p.parseTableElement(t, key)
		if p.acceptPunct(",") {
			continue
		}
		if p.acceptPunct(")") {
			break
		}
		// Unexpected; bail out of the element list defensively.
		break
	}
	p.skipStatement()
}

// parseTableElement parses one entry inside CREATE TABLE (...): either a column
// definition or a table constraint.
func (p *parser) parseTableElement(t *schema.Table, key string) {
	if p.startsTableConstraint() {
		p.parseTableConstraint(t)
		return
	}
	p.parseColumnDef(t, key)
}

// startsTableConstraint reports whether the current element is a table-level
// constraint rather than a column definition.
func (p *parser) startsTableConstraint() bool {
	return p.isWord("constraint") || p.isWord("primary") || p.isWord("unique") ||
		p.isWord("foreign") || p.isWord("check") || p.isWord("exclude")
}

// inlineConstraintKeyword reports whether s introduces a column-level constraint
// (used to bound DEFAULT expression capture).
func inlineConstraintKeyword(s string) bool {
	switch s {
	case "not", "null", "primary", "unique", "default", "check", "references",
		"constraint", "generated", "collate", "deferrable", "initially":
		return true
	}
	return false
}

func (p *parser) parseColumnDef(t *schema.Table, key string) {
	name := p.ident()
	if name == "" {
		p.warn("CREATE TABLE %s: expected column name, got %q (skipped)", key, p.cur().val)
		p.skipElement()
		return
	}
	col := schema.Column{Name: name, Type: p.parseType()}
	// serial pseudo-types are implicitly NOT NULL (they carry a nextval default).
	switch col.Type {
	case "serial", "bigserial", "smallserial":
		col.NotNull = true
	}

	for !p.isTerminator(0) {
		switch {
		case p.acceptWord("constraint"):
			p.ident() // named constraint; keep parsing the actual constraint
		case p.acceptWord("not"):
			p.acceptWord("null")
			col.NotNull = true
		case p.acceptWord("null"):
			// nullable; no-op
		case p.isWord("primary"):
			p.advance()
			p.acceptWord("key")
			col.PrimaryKey = true
			col.NotNull = true
		case p.acceptWord("unique"):
			t.Indexes = append(t.Indexes, schema.Index{Columns: []string{name}, Unique: true})
		case p.acceptWord("default"):
			col.Default = p.captureExpr()
		case p.acceptWord("check"):
			if expr, ok := p.captureParenGroup(); ok {
				t.Checks = append(t.Checks, expr)
			}
		case p.acceptWord("references"):
			t.FKs = append(t.FKs, p.parseInlineReferences(name))
		case p.acceptWord("generated"):
			// GENERATED ... AS IDENTITY columns are implicitly NOT NULL.
			if p.parseGenerated() {
				col.NotNull = true
			}
		case p.acceptWord("collate"):
			p.nameParts() // collation name (may be schema-qualified or quoted)
		default:
			// Unknown column clause (e.g. an unsupported storage option): stop and
			// skip the remainder of this element.
			p.skipElement()
			t.AddColumn(col)
			return
		}
	}
	t.AddColumn(col)
}

// parseInlineReferences parses "REFERENCES tbl [(col)]" after the REFERENCES
// keyword for a single-column inline foreign key.
func (p *parser) parseInlineReferences(localCol string) schema.ForeignKey {
	fk := schema.ForeignKey{Columns: []string{localCol}}
	fk.RefTable = qualifiedKey(p.nameParts())
	if cols := p.optColumnList(); cols != nil {
		fk.RefColumns = cols
	}
	return fk
}

// parseGenerated consumes the remainder of a column's GENERATED clause (the
// GENERATED keyword has already been accepted) and reports whether it is an
// IDENTITY column. It handles both forms:
//
//	GENERATED { ALWAYS | BY DEFAULT } AS IDENTITY [ ( sequence_options ) ]
//	GENERATED ALWAYS AS ( expr ) STORED
func (p *parser) parseGenerated() (identity bool) {
	p.acceptWord("always")
	if p.acceptWord("by") {
		p.acceptWord("default")
	}
	p.acceptWord("as")
	if p.acceptWord("identity") {
		if p.isPunct("(") {
			p.captureParenGroup() // sequence options
		}
		return true
	}
	if p.isPunct("(") {
		p.captureParenGroup() // generation expression
	}
	p.acceptWord("stored")
	return false
}

func (p *parser) parseTableConstraint(t *schema.Table) {
	name := ""
	if p.acceptWord("constraint") {
		name = p.ident()
	}

	switch {
	case p.isWord("primary"):
		p.advance()
		p.acceptWord("key")
		for _, c := range p.optColumnList() {
			if col, ok := t.Column(c); ok {
				col.PrimaryKey = true
				col.NotNull = true
			}
		}
	case p.acceptWord("unique"):
		t.Indexes = append(t.Indexes, schema.Index{
			Name: name, Columns: p.optColumnList(), Unique: true,
		})
	case p.isWord("foreign"):
		p.advance()
		p.acceptWord("key")
		fk := schema.ForeignKey{Name: name, Columns: p.optColumnList()}
		if p.acceptWord("references") {
			fk.RefTable = qualifiedKey(p.nameParts())
			if cols := p.optColumnList(); cols != nil {
				fk.RefColumns = cols
			}
		}
		t.FKs = append(t.FKs, fk)
	case p.acceptWord("check"):
		if expr, ok := p.captureParenGroup(); ok {
			t.Checks = append(t.Checks, expr)
		}
	}
	p.skipElement()
}

// --- ALTER TABLE -----------------------------------------------------------

func (p *parser) parseAlterTable() {
	p.acceptIfExists()
	p.acceptWord("only")
	key := p.requireName("ALTER TABLE")
	t := p.schema.GetOrCreateTable(key)

	if p.isWord("rename") {
		p.parseAlterRename(t, key)
		p.skipStatement()
		return
	}

	for !p.atEnd() && !p.isPunct(";") {
		p.parseAlterAction(t, key)
		if p.acceptPunct(",") {
			continue
		}
		break
	}
	p.skipStatement()
}

func (p *parser) parseAlterRename(t *schema.Table, key string) {
	p.advance() // RENAME
	switch {
	case p.acceptWord("to"):
		newName := p.ident()
		if newName == "" {
			return
		}
		p.renameTable(t, key, newName)
	case p.acceptWord("constraint"):
		// constraint rename doesn't affect the ERD shape; ignore.
	default:
		p.acceptWord("column")
		from := p.ident()
		p.acceptWord("to")
		to := p.ident()
		if from != "" && to != "" {
			if t.RenameColumn(from, to) {
				p.note(fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", key, from, to))
			} else {
				p.warn("ALTER TABLE %s RENAME COLUMN %s: not found", key, from)
			}
		}
	}
}

func (p *parser) renameTable(t *schema.Table, oldKey, newName string) {
	newKey := schema.Qualify(schema.SchemaOf(oldKey), newName)
	delete(p.schema.Tables, oldKey)
	t.Name = newKey
	p.schema.Tables[newKey] = t
	p.note(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", oldKey, newKey))
}

func (p *parser) parseAlterAction(t *schema.Table, key string) {
	switch {
	case p.acceptWord("add"):
		p.acceptIfNotExists()
		if p.startsTableConstraint() {
			p.parseTableConstraint(t)
			p.note("ALTER TABLE " + key + " ADD CONSTRAINT")
			return
		}
		p.acceptWord("column")
		p.acceptIfNotExists()
		p.parseColumnDef(t, key)
		p.note("ALTER TABLE " + key + " ADD COLUMN")

	case p.acceptWord("drop"):
		p.acceptWord("column")
		if p.acceptWord("constraint") {
			p.acceptIfExists()
			name := p.ident()
			if t.DropConstraintByName(name) {
				p.note(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s", key, name))
			} else {
				p.warn("ALTER TABLE %s DROP CONSTRAINT %s: not found", key, name)
			}
			p.skipElement()
			return
		}
		p.acceptIfExists()
		name := p.ident()
		if t.DropColumn(name) {
			p.note(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", key, name))
		} else {
			p.warn("ALTER TABLE %s DROP COLUMN %s: not found", key, name)
		}
		p.skipElement()

	case p.acceptWord("alter"):
		p.acceptWord("column")
		name := p.ident()
		p.parseAlterColumn(t, key, name)

	default:
		p.skipElement()
	}
}

func (p *parser) parseAlterColumn(t *schema.Table, key, name string) {
	switch {
	case p.acceptWord("type"):
		p.setColumnType(t, name)
	case p.isWord("set"):
		p.advance()
		switch {
		case p.acceptWord("data"): // SET DATA TYPE <type>
			p.acceptWord("type")
			p.setColumnType(t, name)
		case p.acceptWord("default"):
			if col, ok := t.Column(name); ok {
				col.Default = p.captureExpr()
			}
		case p.acceptWord("not"):
			p.acceptWord("null")
			if col, ok := t.Column(name); ok {
				col.NotNull = true
			}
		default:
			p.skipElement()
		}
		_ = key
	case p.acceptWord("drop"):
		switch {
		case p.acceptWord("default"):
			if col, ok := t.Column(name); ok {
				col.Default = ""
			}
		case p.acceptWord("not"):
			p.acceptWord("null")
			if col, ok := t.Column(name); ok {
				col.NotNull = false
			}
		default:
			p.skipElement()
		}
	default:
		p.skipElement()
	}
}

func (p *parser) setColumnType(t *schema.Table, name string) {
	typ := p.parseType()
	if col, ok := t.Column(name); ok {
		col.Type = typ
	}
	p.skipElement() // consume USING/COLLATE remainder, if any
}

// --- CREATE INDEX ----------------------------------------------------------

func (p *parser) parseCreateIndex(unique bool) {
	p.acceptWord("concurrently")
	p.acceptIfNotExists()

	name := ""
	if !p.isWord("on") {
		name = p.ident() // index name (unqualified in CREATE INDEX)
	}
	if !p.acceptWord("on") {
		p.skipStatement()
		return
	}
	p.acceptWord("only")
	key := p.requireName("CREATE INDEX")
	t := p.schema.GetOrCreateTable(key)

	if p.acceptWord("using") {
		p.ident() // access method
	}

	idx := schema.Index{Name: name, Unique: unique, Columns: p.indexColumns()}
	t.Indexes = append(t.Indexes, idx)
	p.note(fmt.Sprintf("CREATE INDEX %s ON %s", name, key))
	p.skipStatement()
}

// indexColumns parses "( col [, col] )", collecting plain column names and
// skipping expression entries.
func (p *parser) indexColumns() []string {
	var cols []string
	if !p.acceptPunct("(") {
		return cols
	}
	for !p.atEnd() {
		if p.acceptPunct(")") {
			break
		}
		// Plain column iff a bare identifier not followed by '(' (func call) or '.'.
		if p.cur().kind == tWord {
			nxt := p.peek(1)
			if nxt.kind != tPunct || (nxt.val != "(" && nxt.val != ".") {
				cols = append(cols, p.cur().val)
			}
		}
		p.skipElement()
		if p.acceptPunct(",") {
			continue
		}
		if p.acceptPunct(")") {
			break
		}
		break
	}
	return cols
}

// --- DROP ------------------------------------------------------------------

func (p *parser) parseDrop() {
	switch {
	case p.acceptWord("table"):
		p.acceptIfExists()
		for _, key := range p.dropNameList() {
			if _, ok := p.schema.Tables[key]; ok {
				delete(p.schema.Tables, key)
				p.note("DROP TABLE " + key)
			} else {
				p.warn("DROP TABLE %s: not found", key)
			}
		}
	case p.acceptWord("index"):
		p.acceptWord("concurrently")
		p.acceptIfExists()
		for _, parts := range p.dropNamePartsList() {
			bare := parts[len(parts)-1] // index name (drop any schema qualifier)
			if p.schema.DropIndexByName(bare) {
				p.note("DROP INDEX " + bare)
			} else {
				p.warn("DROP INDEX %s: not found", bare)
			}
		}
	case p.acceptWord("schema"):
		p.acceptIfExists()
		for _, parts := range p.dropNamePartsList() {
			name := parts[len(parts)-1]
			if n := p.schema.DropSchema(name); n > 0 {
				p.note(fmt.Sprintf("DROP SCHEMA %s (%d tables)", name, n))
			} else {
				p.warn("DROP SCHEMA %s: no tables", name)
			}
		}
	default:
		p.note("skipped DROP " + p.cur().val)
	}
	p.skipStatement()
}

// dropNameList parses a comma-separated list of qualified relation names,
// stopping at CASCADE/RESTRICT or the statement end.
func (p *parser) dropNameList() []string {
	var keys []string
	for _, parts := range p.dropNamePartsList() {
		keys = append(keys, qualifiedKey(parts))
	}
	return keys
}

func (p *parser) dropNamePartsList() [][]string {
	var out [][]string
	for !p.atEnd() && !p.isPunct(";") {
		if p.isWord("cascade") || p.isWord("restrict") {
			break
		}
		parts := p.nameParts()
		if len(parts) == 0 {
			break
		}
		out = append(out, parts)
		if !p.acceptPunct(",") {
			break
		}
	}
	return out
}

// --- shared clause helpers -------------------------------------------------

func (p *parser) acceptIfNotExists() {
	if p.isWord("if") && p.peek(1).kind == tWord && p.peek(1).val == "not" {
		p.advance() // if
		p.advance() // not
		p.acceptWord("exists")
	}
}

func (p *parser) acceptIfExists() {
	if p.isWord("if") && p.peek(1).kind == tWord && p.peek(1).val == "exists" {
		p.advance() // if
		p.advance() // exists
	}
}

// optColumnList parses "( a [, b] )" returning the bare names, or nil if the
// current token isn't '('.
func (p *parser) optColumnList() []string {
	if !p.acceptPunct("(") {
		return nil
	}
	var cols []string
	for !p.atEnd() {
		if p.acceptPunct(")") {
			break
		}
		if p.cur().kind == tWord {
			cols = append(cols, p.cur().val)
		}
		p.advance()
		if p.acceptPunct(",") {
			continue
		}
		if p.acceptPunct(")") {
			break
		}
	}
	return cols
}

// parseType reads a (possibly multi-word) type name with optional length/
// precision modifiers and array markers, returning a normalized spelling.
func (p *parser) parseType() string {
	if p.cur().kind != tWord {
		return ""
	}
	words := []string{p.cur().val}
	base := p.cur().val
	p.advance()

	switch base {
	case "double":
		if p.acceptWord("precision") {
			words = append(words, "precision")
		}
	case "national":
		if p.isWord("character") || p.isWord("char") {
			words = append(words, p.cur().val)
			p.advance()
		}
		if p.acceptWord("varying") {
			words = append(words, "varying")
		}
	case "character", "char", "bit":
		if p.acceptWord("varying") {
			words = append(words, "varying")
		}
	case "timestamp", "time":
		if p.isWord("with") || p.isWord("without") {
			words = append(words, p.cur().val)
			p.advance()
			if p.acceptWord("time") {
				words = append(words, "time")
			}
			if p.acceptWord("zone") {
				words = append(words, "zone")
			}
		}
	}

	typ := normalizeType(strings.Join(words, " "))

	if p.isPunct("(") {
		if mods, ok := p.captureParenGroup(); ok {
			typ += "(" + strings.ReplaceAll(mods, " ", "") + ")"
		}
	}
	for p.isPunct("[") {
		p.advance()
		for !p.isPunct("]") && !p.atEnd() {
			p.advance()
		}
		p.acceptPunct("]")
		typ += "[]"
	}
	return typ
}

// normalizeType maps PostgreSQL internal/alias type names to conventional SQL
// spellings for friendlier output.
func normalizeType(t string) string {
	switch t {
	case "int4", "integer":
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
	case "bpchar", "character":
		return "char"
	case "character varying":
		return "varchar"
	case "timestamp without time zone":
		return "timestamp"
	case "timestamp with time zone":
		return "timestamptz"
	case "time without time zone":
		return "time"
	case "time with time zone":
		return "timetz"
	default:
		return t
	}
}

// captureExpr captures the verbatim source text of an expression (e.g. a DEFAULT
// value), whitespace-collapsed, stopping at a top-level element terminator or a
// following inline-constraint keyword.
func (p *parser) captureExpr() string {
	if p.isTerminator(0) {
		return ""
	}
	startOff := p.cur().start
	endOff := startOff
	depth := 0
	first := true
	for !p.atEnd() {
		t := p.cur()
		if t.kind == tPunct {
			switch t.val {
			case "(", "[":
				depth++
			case ")", "]":
				if depth == 0 {
					goto done
				}
				depth--
			case ",", ";":
				if depth == 0 {
					goto done
				}
			}
		} else if depth == 0 && !first && t.kind == tWord && !t.quoted && inlineConstraintKeyword(t.val) {
			goto done
		}
		endOff = t.end
		first = false
		p.advance()
	}
done:
	return collapseWS(p.src[startOff:endOff])
}

// captureParenGroup expects the current token to be '(' and returns the
// whitespace-collapsed inner text, consuming through the matching ')'.
func (p *parser) captureParenGroup() (string, bool) {
	if !p.isPunct("(") {
		return "", false
	}
	p.advance() // '('
	startOff := p.cur().start
	depth := 1
	for !p.atEnd() {
		t := p.cur()
		if t.kind == tPunct {
			switch t.val {
			case "(", "[":
				depth++
			case ")", "]":
				depth--
				if depth == 0 {
					text := collapseWS(p.src[startOff:t.start])
					p.advance() // ')'
					return text, true
				}
			}
		}
		p.advance()
	}
	return "", false
}

// collapseWS trims and collapses internal whitespace runs to single spaces.
func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// --- FK resolution ---------------------------------------------------------

// resolveForeignKeys warns about FKs whose referenced table is absent. Non-fatal.
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

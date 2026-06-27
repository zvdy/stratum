package parser

import "testing"

// FuzzParseSQL exercises the parser's core promise: for any input it either
// parses or returns an error, but never panics. Running `go test ./...` executes
// only the seed corpus below (a fast regression guard); `go test -fuzz=FuzzParseSQL`
// explores beyond it.
func FuzzParseSQL(f *testing.F) {
	seeds := []string{
		"",
		"   \n\t ",
		"-- only a comment",
		"/* block */",
		"CREATE TABLE t (id serial PRIMARY KEY, name text NOT NULL);",
		"CREATE TABLE a.b (id int REFERENCES a.c (id));",
		"ALTER TABLE t ADD COLUMN x int DEFAULT 0;",
		"ALTER TABLE t DROP CONSTRAINT pk_t;",
		"ALTER TABLE t RENAME TO u;",
		"CREATE UNIQUE INDEX ix ON t (a, b);",
		"DROP INDEX ix; DROP TABLE t; DROP SCHEMA s CASCADE;",
		"DO $$ BEGIN PERFORM 1; END $$;",
		"INSERT INTO t VALUES (1, 'x');",
		"CREATE TABLE", // truncated / malformed recognized statement
		"CREATE TABLE t (",
		"CREATE TABLE t (id int GENERATED ALWAYS AS IDENTITY PRIMARY KEY);",
		"CREATE TABLE t (c text COLLATE \"C\" CHECK (c <> ''));",
		"((((((((((",
		"$tag$ unterminated",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, sql string) {
		// A returned error is acceptable; a panic is not. The defer here turns a
		// panic into a test failure with the offending input attached.
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ParseSQL panicked on %q: %v", sql, r)
			}
		}()
		_, _, _ = ParseSQL(sql, Options{})
	})
}

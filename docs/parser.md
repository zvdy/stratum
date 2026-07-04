# Parser

stratum's parser is a small, hand-written, pure-Go recursive-descent parser
over the subset of PostgreSQL DDL an ERD needs. It is deliberately defensive:
statements and clauses it does not model are skipped rather than failing, so
real-world migrations containing DML, `DO $$ … $$` blocks, functions, and
extension setup parse cleanly. The parser never panics — a promise backed by a
fuzz target (`FuzzParseSQL`) in the test suite.

## What it understands

- `CREATE TABLE` — columns, inline and table-level primary keys, foreign keys
  (`REFERENCES`), `UNIQUE`, `CHECK`, `NOT NULL`, `DEFAULT`; composite and
  self-referential keys; `serial`/`bigserial` and `GENERATED … AS IDENTITY`
  columns (treated as `NOT NULL`); `COLLATE`; multi-word types
  (`double precision`, `character varying(n)`, `timestamp with time zone`,
  arrays)
- `ALTER TABLE` — add/drop/rename column, alter column type, set/drop default,
  set/drop `NOT NULL`, add constraint (FK / UNIQUE / CHECK), drop constraint
  (FK, UNIQUE, named CHECK, named PRIMARY KEY), rename table
- `CREATE [UNIQUE] INDEX` and `DROP INDEX`
- `DROP TABLE` and `DROP SCHEMA`
- Multiple schemas (`public.users`, etc.); unqualified names default to
  `public`

DML (`INSERT`/`UPDATE`/`DELETE`), procedural `DO $$ … $$` blocks / function
bodies, comments, and other non-structural statements are skipped (their `;`
inside dollar-quotes won't break parsing). Unrecognized statements are skipped,
never fatal.

## Relationship cardinality

Cardinality in the ERD reflects the declared constraints rather than a fixed
symbol:

| FK columns | Mermaid | Meaning |
| --- | --- | --- |
| `NOT NULL` | `}o--\|\|` | mandatory many-to-one |
| nullable | `}o--o\|` | optional many-to-one |
| `NOT NULL` + unique | `\|o--\|\|` | mandatory one-to-one |
| nullable + unique | `\|o--o\|` | optional one-to-one |

A foreign key is considered *unique* when its column set is exactly covered by
a unique constraint or unique index.

## Design notes

- **Replay model** — migrations are applied in order to an in-memory schema
  IR; the final state is rendered. `DROP`/`RENAME`/`ALTER` therefore behave
  like they do in a real database: what you see is the schema as it exists
  *after* the last migration.
- **Lexer first** — a permissive tokenizer handles the parts that affect
  statement boundaries (line/block comments, quoted identifiers, string
  literals, and dollar-quoted bodies), so the parser can skip anything it
  doesn't model without desyncing.
- **Deterministic output** — tables and relations are sorted; identical input
  yields byte-identical output, enabling `--check`.

## Known limitations

- `DROP INDEX` matches by bare index name across schemas.
- Inline `REFERENCES … DEFERRABLE NOT NULL` ordering can drop the trailing
  `NOT NULL`.
- `CREATE TABLE (LIKE other)` parses `LIKE` as a column.

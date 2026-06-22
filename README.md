# stratum

`stratum` is a Go CLI that **statically** parses PostgreSQL migration `.sql`
files and generates a [Mermaid](https://mermaid.js.org/) ERD embedded in a
Markdown document. It opens **no database connection** and makes **no network
calls** — every fact in the diagram is derived from the migration files
themselves, so it runs fully offline (e.g. in a GitHub Actions runner).

It works by replaying your migrations in order against an in-memory schema model
and rendering the result. Parsing uses
[`pg_query_go`](https://github.com/pganalyze/pg_query_go) (the real PostgreSQL
grammar), so it understands the SQL your database actually accepts.

## What it understands

- `CREATE TABLE` — columns, inline/■table-level primary keys, foreign keys
  (`REFERENCES`), `UNIQUE`, `CHECK`, `NOT NULL`, `DEFAULT`; inline and
  table-level constraints
- `ALTER TABLE` — add/drop/rename column, alter column type, add FK / UNIQUE /
  CHECK constraint, rename table
- `CREATE [UNIQUE] INDEX`
- `DROP TABLE`
- Multiple schemas (`public.users`, etc.); unqualified names default to `public`

DML (`INSERT`/`UPDATE`/`DELETE`), procedural `DO $$ … $$` blocks, comments, and
other non-structural statements are skipped silently. Unknown statement types
produce a warning and are skipped — the parser never panics.

## Install / build

```bash
go build -o stratum ./cmd/stratum
```

> Note: `pg_query_go` uses cgo, so a C compiler (gcc/clang) is required to build.

## Usage

```bash
stratum --migrations-path db/migrations --output SCHEMA.md
```

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--migrations-path` | _(required)_ | Path to migrations directory |
| `--output` | `SCHEMA.md` | Output file path |
| `--schema` | `public` | Filter output to this schema name |
| `--recursive` | `false` | Discover `.sql` files recursively |
| `--push` | `false` | `git add` + `commit` + `push` the output file |
| `--commit-message` | `chore: update schema docs` | Commit message used with `--push` |
| `--verbose` | `false` | Log each parsed statement |

The tool exits non-zero if any file fails to parse. It warns (non-fatal) on
unknown statement types, dangling foreign keys, and empty migration files.

## Migration ordering

Files are ordered by filename, supporting both numeric prefixes (`001_`, `002_`)
and 14-digit timestamp prefixes (`20240101120000_`). Numeric prefixes are
compared as numbers, so `2_` sorts before `10_`.

To pin an explicit order, drop a `migrations.yaml` in the migrations directory:

```yaml
migrations:
  - 003_first.sql
  - 001_second.sql
  - 002_third.sql
```

When present, the manifest order is used verbatim.

## GitHub Action

```yaml
- uses: actions/checkout@v4
- uses: zvdy/stratum@v1
  with:
    migrations-path: db/migrations
    output: SCHEMA.md
    schema: public
    push: "true"
```

See [`action.yml`](./action.yml) for all inputs.

## Example

A complete, multi-schema e-commerce example lives in
[`examples/ecommerce/`](./examples/ecommerce/): five migrations (plus a
`migrations.yaml` manifest) covering PKs/FKs, composite keys, unique indexes,
checks, `ALTER` evolution, a `DROP`, and a separate `auth` schema. The committed
[`SCHEMA.md`](./examples/ecommerce/SCHEMA.md) (public) and
[`SCHEMA.auth.md`](./examples/ecommerce/SCHEMA.auth.md) (`--schema auth`) are the
generated output:

```bash
stratum --migrations-path examples/ecommerce/migrations --output examples/ecommerce/SCHEMA.md
stratum --migrations-path examples/ecommerce/migrations --schema auth --output examples/ecommerce/SCHEMA.auth.md
```

The [`Example ERD`](./.github/workflows/example.yml) workflow runs the composite
action against this example on every push/PR.

## Docker

```bash
docker build -t stratum .
docker run --rm -v "$PWD:/work" -w /work stratum \
  --migrations-path db/migrations --output SCHEMA.md
```

## Development

```bash
go test ./...
```

Fixtures live under `testdata/`.

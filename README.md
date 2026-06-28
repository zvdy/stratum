<p align="center">
  <img src="docs/stratum-logo.png" alt="stratum logo" width="180" height="180">
</p>

<h1 align="center">stratum</h1>

<p align="center">
  <em>Static PostgreSQL migrations → Mermaid ERD — no database, no cgo.</em>
</p>

<p align="center">
  <a href="https://github.com/zvdy/stratum/actions/workflows/ci.yml">
    <img src="https://github.com/zvdy/stratum/actions/workflows/ci.yml/badge.svg" alt="CI">
  </a>
</p>

`stratum` is a Go CLI that **statically** parses PostgreSQL migration `.sql`
files and generates a [Mermaid](https://mermaid.js.org/) ERD embedded in a
Markdown document. It opens **no database connection** and makes **no network
calls** — every fact in the diagram is derived from the migration files
themselves, so it runs fully offline (e.g. in a GitHub Actions runner).

It works by replaying your migrations in order against an in-memory schema model
and rendering the result. Parsing uses a small, **hand-written pure-Go** SQL DDL
parser (no cgo, no third-party parser) that models the subset of PostgreSQL DDL
an ERD needs and defensively skips everything else.

Relationship cardinality in the ERD reflects the constraints: a foreign key
renders as mandatory (`}o--||`) when its columns are `NOT NULL` and optional
(`}o--o|`) when nullable, and collapses to one-to-one (`|o--…`) when the FK
columns are covered by a unique constraint.

## What it understands

- `CREATE TABLE` — columns, inline and table-level primary keys, foreign keys
  (`REFERENCES`), `UNIQUE`, `CHECK`, `NOT NULL`, `DEFAULT`; composite and
  self-referential keys; `serial`/`bigserial` and `GENERATED … AS IDENTITY`
  columns (treated as `NOT NULL`); `COLLATE`; multi-word types
  (`double precision`, `character varying(n)`, `timestamp with time zone`,
  arrays)
- `ALTER TABLE` — add/drop/rename column, alter column type, set/drop default,
  add constraint (FK / UNIQUE / CHECK), drop constraint, rename table
- `CREATE [UNIQUE] INDEX` and `DROP INDEX`
- `DROP TABLE` and `DROP SCHEMA`
- Multiple schemas (`public.users`, etc.); unqualified names default to `public`

DML (`INSERT`/`UPDATE`/`DELETE`), procedural `DO $$ … $$` blocks / function
bodies, comments, and other non-structural statements are skipped (their `;`
inside dollar-quotes won't break parsing). Unrecognized statements are skipped,
never fatal; the parser never panics.

## Install / build

```bash
go build -o stratum ./cmd/stratum
```

> Pure Go — no C compiler needed. `CGO_ENABLED=0` produces a fully static binary.

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
| `--check` | `false` | Verify the output file is up to date; exit non-zero if it would change (writes nothing) |
| `--verbose` | `false` | Log each parsed statement |

The tool exits non-zero if any file fails to parse. It warns (non-fatal) on
unknown statement types, dangling foreign keys, and empty migration files.

### Keeping docs fresh in CI

Use `--check` as a gate so a pull request fails until its schema docs are
regenerated (the volatile timestamp line is ignored, so only schema content
counts):

```bash
stratum --migrations-path db/migrations --output SCHEMA.md --check
```

This is the read-only alternative to `--push`: no bot commits, the author
regenerates and commits `SCHEMA.md` themselves.

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

## License

[MIT](./LICENSE) © zvdy

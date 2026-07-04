# Usage

## Install / build

```bash
go build -o stratum ./cmd/stratum
```

Pure Go — no C compiler needed. `CGO_ENABLED=0` produces a fully static binary.

## Running

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

## Keeping docs fresh in CI

Use `--check` as a gate so a pull request fails until its schema docs are
regenerated (the volatile timestamp line is ignored, so only schema content
counts):

```bash
stratum --migrations-path db/migrations --output SCHEMA.md --check
```

This is the read-only alternative to `--push`: no bot commits — the author
regenerates and commits `SCHEMA.md` themselves.

## Migration ordering

Files are ordered by filename, supporting both numeric prefixes (`001_`,
`002_`) and 14-digit timestamp prefixes (`20240101120000_`). Numeric prefixes
are compared as numbers, so `2_` sorts before `10_`.

To pin an explicit order, drop a `migrations.yaml` in the migrations directory:

```yaml
migrations:
  - 003_first.sql
  - 001_second.sql
  - 002_third.sql
```

When present, the manifest order is used verbatim.

## Docker

```bash
docker build -t stratum .
docker run --rm -v "$PWD:/work" -w /work stratum \
  --migrations-path db/migrations --output SCHEMA.md
```

# GitHub Action

stratum ships as a composite GitHub Action that builds the binary from source
(pure Go, no C toolchain) and runs it against your migrations — entirely
offline, on the runner.

## Generate and push schema docs

```yaml
- uses: actions/checkout@v4
- uses: zvdy/stratum@v1
  with:
    migrations-path: db/migrations
    output: SCHEMA.md
    schema: public
    push: "true"
```

With `push: "true"` the action commits and pushes the regenerated `SCHEMA.md`
back to the branch (the workflow needs `contents: write` permission and a
checkout with credentials).

## Gate pull requests instead

If you'd rather fail the PR than push bot commits, use `check` — the job exits
non-zero when `SCHEMA.md` is stale, and the author regenerates it themselves:

```yaml
- uses: actions/checkout@v4
- uses: zvdy/stratum@v1
  with:
    migrations-path: db/migrations
    output: SCHEMA.md
    check: "true"
```

## Inputs

| Input | Default | Description |
| --- | --- | --- |
| `migrations-path` | _(required)_ | Path to the migrations directory |
| `output` | `SCHEMA.md` | Output file path |
| `schema` | `public` | Filter output to this schema name |
| `recursive` | `"false"` | Discover `.sql` files recursively |
| `push` | `"false"` | `git add` + `commit` + `push` the output file |
| `commit-message` | `chore: update schema docs` | Commit message used with `push` |
| `check` | `"false"` | Fail (without writing) if the output file is out of date |
| `verbose` | `"false"` | Log each parsed statement |

See [`action.yml`](https://github.com/zvdy/stratum/blob/main/action.yml) for
the source of truth.

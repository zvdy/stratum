// Command stratum statically parses PostgreSQL migration .sql files and writes a
// Mermaid ERD embedded in a Markdown document. It opens no database connection
// and makes no network calls — all schema facts are derived from file contents.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/zvdy/stratum/internal/migration"
	"github.com/zvdy/stratum/internal/parser"
	"github.com/zvdy/stratum/internal/render"
)

type options struct {
	migrationsPath string
	output         string
	schemaName     string
	recursive      bool
	push           bool
	commitMessage  string
	verbose        bool
}

func main() {
	opts := &options{}

	root := &cobra.Command{
		Use:           "stratum",
		Short:         "Generate a Mermaid ERD from PostgreSQL migration files",
		Long:          "stratum statically parses PostgreSQL migration .sql files and renders a Mermaid ERD inside a Markdown document. No database connection is used.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(opts)
		},
	}

	f := root.Flags()
	f.StringVar(&opts.migrationsPath, "migrations-path", "", "path to migrations directory (required)")
	f.StringVar(&opts.output, "output", "SCHEMA.md", "output file path")
	f.StringVar(&opts.schemaName, "schema", "public", "filter output to this schema name")
	f.BoolVar(&opts.recursive, "recursive", false, "discover .sql files recursively")
	f.BoolVar(&opts.push, "push", false, "git add + commit + push the output file")
	f.StringVar(&opts.commitMessage, "commit-message", "chore: update schema docs", "commit message used with --push")
	f.BoolVar(&opts.verbose, "verbose", false, "log each parsed statement")
	_ = root.MarkFlagRequired("migrations-path")

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(opts *options) error {
	files, err := migration.Discover(opts.migrationsPath, opts.recursive)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "warning: no .sql files found under %s\n", opts.migrationsPath)
	}

	parseOpts := parser.Options{Verbose: opts.verbose}
	if opts.verbose {
		parseOpts.OnStatement = func(file, desc string) {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", file, desc)
		}
	}

	sch, warnings, err := parser.ParseFiles(files, parseOpts)
	if err != nil {
		return err // fatal: parse failure => non-zero exit
	}
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w.String())
	}

	filtered := sch.Filter(opts.schemaName)
	doc := render.Markdown(filtered, opts.migrationsPath, time.Now())

	if err := os.WriteFile(opts.output, []byte(doc), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", opts.output, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%d tables)\n", opts.output, len(filtered.Tables))

	if opts.push {
		if err := gitPush(opts.output, opts.commitMessage); err != nil {
			return err
		}
	}
	return nil
}

// gitPush stages, commits and pushes the output file. It is the only part of the
// program that shells out, and only runs under --push.
func gitPush(file, message string) error {
	steps := [][]string{
		{"add", file},
		{"commit", "-m", message},
		{"push"},
	}
	for _, step := range steps {
		cmd := exec.Command("git", step...)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// `git commit` fails when there is nothing to commit; treat that as
			// a no-op rather than an error.
			if step[0] == "commit" {
				fmt.Fprintln(os.Stderr, "nothing to commit; skipping push")
				return nil
			}
			return fmt.Errorf("git %s: %w", step[0], err)
		}
	}
	return nil
}

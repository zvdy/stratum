// Package migration discovers and orders PostgreSQL migration files on disk.
// It performs no parsing — it only decides which files to feed to the parser and
// in what order. Ordering supports numeric prefixes (001_, 2_), 14-digit
// timestamp prefixes (20240101120000_), and an optional migrations.yaml manifest
// that lists an explicit order.
package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestName is the conventional filename for an explicit ordering manifest.
const ManifestName = "migrations.yaml"

// manifest is the on-disk shape of migrations.yaml.
type manifest struct {
	Migrations []string `yaml:"migrations"`
}

// Discover returns the ordered list of migration file paths under dir.
//
// If a migrations.yaml manifest exists in dir it takes precedence and its order
// is used verbatim (paths resolved relative to dir). Otherwise all .sql files
// are collected (recursively when recursive is true) and sorted by their
// filename prefix.
func Discover(dir string, recursive bool) ([]string, error) {
	if m, ok, err := loadManifest(dir); err != nil {
		return nil, err
	} else if ok {
		return m, nil
	}

	files, err := collectSQLFiles(dir, recursive)
	if err != nil {
		return nil, err
	}
	sortByPrefix(files)
	return files, nil
}

// loadManifest reads dir/migrations.yaml if present, returning the resolved,
// ordered absolute-ish paths. The second return reports whether a manifest was
// found.
func loadManifest(dir string) ([]string, bool, error) {
	path := filepath.Join(dir, ManifestName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading manifest %s: %w", path, err)
	}

	var m manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, false, fmt.Errorf("parsing manifest %s: %w", path, err)
	}
	if len(m.Migrations) == 0 {
		return nil, false, fmt.Errorf("manifest %s lists no migrations", path)
	}

	out := make([]string, 0, len(m.Migrations))
	for _, rel := range m.Migrations {
		out = append(out, filepath.Join(dir, rel))
	}
	return out, true, nil
}

// collectSQLFiles lists .sql files under dir. When recursive is false only the
// top-level directory is scanned.
func collectSQLFiles(dir string, recursive bool) ([]string, error) {
	var files []string

	if recursive {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && isSQL(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", dir, err)
		}
		return files, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() && isSQL(e.Name()) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files, nil
}

func isSQL(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".sql")
}

// sortByPrefix orders files by their leading numeric prefix when both files have
// one (comparing as big integers so 001 < 010 < 2 is impossible — 2 < 10), and
// falls back to lexicographic comparison of the base filename otherwise. The
// sort is stable.
func sortByPrefix(files []string) {
	sort.SliceStable(files, func(i, j int) bool {
		ni := filepath.Base(files[i])
		nj := filepath.Base(files[j])

		pi, oki := numericPrefix(ni)
		pj, okj := numericPrefix(nj)

		if oki && okj {
			if len(pi) != len(pj) {
				return len(pi) < len(pj) // fewer digits => smaller number
			}
			if pi != pj {
				return pi < pj // equal length => lexical == numeric
			}
		}
		return ni < nj
	})
}

// numericPrefix returns the leading run of digits in name (with leading zeros
// stripped for length comparison) and whether one was present.
func numericPrefix(name string) (string, bool) {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 0 {
		return "", false
	}
	digits := name[:i]
	trimmed := strings.TrimLeft(digits, "0")
	if trimmed == "" {
		trimmed = "0"
	}
	return trimmed, true
}

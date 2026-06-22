package migration

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeFiles creates empty files named by the given basenames in dir.
func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("-- noop\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

func TestDiscoverOrdering(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  []string
	}{
		{
			name:  "numeric prefixes sort numerically not lexically",
			files: []string{"010_j.sql", "002_b.sql", "001_a.sql", "100_z.sql"},
			want:  []string{"001_a.sql", "002_b.sql", "010_j.sql", "100_z.sql"},
		},
		{
			name:  "unpadded numeric prefixes",
			files: []string{"10_x.sql", "2_y.sql", "1_w.sql"},
			want:  []string{"1_w.sql", "2_y.sql", "10_x.sql"},
		},
		{
			name:  "timestamp prefixes",
			files: []string{"20240101120000_b.sql", "20230101120000_a.sql", "20240101130000_c.sql"},
			want:  []string{"20230101120000_a.sql", "20240101120000_b.sql", "20240101130000_c.sql"},
		},
		{
			name:  "non-sql files ignored",
			files: []string{"002_b.sql", "readme.txt", "001_a.sql"},
			want:  []string{"001_a.sql", "002_b.sql"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, tt.files...)
			got, err := Discover(dir, false)
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if !reflect.DeepEqual(basenames(got), tt.want) {
				t.Errorf("got %v, want %v", basenames(got), tt.want)
			}
		})
	}
}

func TestDiscoverRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, "001_a.sql")
	writeFiles(t, sub, "002_b.sql")

	// Non-recursive sees only the top level.
	got, err := Discover(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"001_a.sql"}; !reflect.DeepEqual(basenames(got), want) {
		t.Errorf("non-recursive: got %v, want %v", basenames(got), want)
	}

	// Recursive sees both.
	got, err = Discover(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"001_a.sql", "002_b.sql"}; !reflect.DeepEqual(basenames(got), want) {
		t.Errorf("recursive: got %v, want %v", basenames(got), want)
	}
}

func TestDiscoverManifestOverridesSort(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "001_a.sql", "002_b.sql", "003_c.sql")

	manifest := "migrations:\n  - 003_c.sql\n  - 001_a.sql\n  - 002_b.sql\n"
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(dir, false)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []string{"003_c.sql", "001_a.sql", "002_b.sql"}
	if !reflect.DeepEqual(basenames(got), want) {
		t.Errorf("manifest order: got %v, want %v", basenames(got), want)
	}
}

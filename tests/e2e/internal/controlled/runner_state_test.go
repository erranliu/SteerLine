package controlled

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunnerFixtureReadFailureDoesNotBecomeEmptyState(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name      string
		prepare   func(string) error
		wantError bool
	}{
		{"missing", func(string) error { return nil }, false},
		{"unreadable-directory", func(path string) error { return os.Mkdir(path, 0700) }, true},
		{"corrupt", func(path string) error { return os.WriteFile(path, []byte(`{`), 0600) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.name)
			if err := tc.prepare(path); err != nil {
				t.Fatal(err)
			}
			state := RunnerFixture{}
			if err := readRunnerFixture(path, &state); (err != nil) != tc.wantError {
				t.Fatalf("read: %v", err)
			}
			if tc.wantError {
				if _, err := os.Stat(path); err != nil {
					t.Fatal("original evidence disappeared", err)
				}
			}
		})
	}
}

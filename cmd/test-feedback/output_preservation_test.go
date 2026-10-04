package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunRejectsExistingReportsBeforeStartingTests(t *testing.T) {
	for _, selection := range [][]string{{"-workers=3"}, {"-level=L1"}, {"-level=L2"}, {"-build-only"}, {"--", "./missing-package"}} {
		t.Run(selection[0], func(t *testing.T) {
			out := t.TempDir()
			path := filepath.Join(out, "summary.json")
			original := []byte("original failed report\n")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			if code := run(append([]string{"-out=" + out}, selection...)); code != 2 {
				t.Fatalf("existing report accepted: exit=%d", code)
			}
			content, err := os.ReadFile(path)
			if err != nil || string(content) != string(original) {
				t.Fatalf("original report changed: %q (%v)", content, err)
			}
			files, err := os.ReadDir(out)
			if err != nil || len(files) != 1 {
				t.Fatalf("tests wrote into the original evidence directory: %v (%v)", files, err)
			}
		})
	}
}

package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoadmapPublicationStateCanReopenWithoutRelaxingDirectoryGuard(t *testing.T) {
	for _, name := range []string{"published-checkout", "file-instead-of-directory", "unknown-runtime-root"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "roadmap-publications")
			if name == "file-instead-of-directory" {
				if err := os.WriteFile(target, []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if name == "unknown-runtime-root" {
					target = filepath.Join(root, "unknown-runtime")
				}
				if err := os.MkdirAll(filepath.Join(target, "original-publication", ".git"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			err := ensureNativeStateFilesAllowed(root, nil)
			if (err == nil) != (name == "published-checkout") {
				t.Fatalf("unexpected native state acceptance: %v", err)
			}
		})
	}
}

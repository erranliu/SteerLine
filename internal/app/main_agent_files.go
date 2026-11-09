package app

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/pathguard"
)

func makePublicationParent(dir string) error {
	resolved, err := pathguard.ResolveProspective(dir)
	if err != nil || canonicalPath(resolved) != canonicalPath(dir) {
		return errors.New("publication directory is redirected")
	}
	return os.MkdirAll(filepath.Dir(dir), 0700)
}
func publicationDirectoryExists(dir string) bool { _, err := os.Lstat(dir); return !os.IsNotExist(err) }

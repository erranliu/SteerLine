package runnerclient

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// VerifyFileMaterials checks uploaded files separately from the Git manifest.
// The caller uses the original persisted request, never current project data.
func VerifyFileMaterials(expected []runnercontract.Document, actual []runnercontract.Material) error {
	files := map[string]string{}
	for _, file := range expected {
		if file.Kind != "file" || file.Content == nil || file.Commit != "" || files[file.Path] != "" {
			return errors.New("invalid original file materials")
		}
		sum := sha256.Sum256([]byte(*file.Content))
		files[file.Path] = hex.EncodeToString(sum[:])
	}
	if len(files) == 0 || len(actual) != len(files) {
		return errors.New("frozen file material manifest is incomplete")
	}
	seen := map[string]bool{}
	for _, file := range actual {
		digest, exists := files[file.Path]
		if !exists || seen[file.Path] || file.Kind != "file" || digest != file.Digest || file.Commit != "" ||
			!filepath.IsAbs(file.ExecutionLocation) || file.ExecutionLocation != file.SnapshotPath {
			return fmt.Errorf("frozen file material differs: %s", file.Path)
		}
		seen[file.Path] = true
	}
	return nil
}

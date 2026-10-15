package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	"github.com/erranliu/SteerLine/internal/pathguard"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
)

func preflightNativeStatePath(c Config) error {
	if c.Identity() == "" {
		if err := c.ResolveIdentity(context.Background()); err != nil {
			return err
		}
	}
	if err := ensureNativeRepositoryHasNoBeads(c.Repository); err != nil {
		return err
	}
	if err := ensureRunDirOutsideRepository(c); err != nil {
		return err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	for _, entry := range []struct{ name, path string }{
		{"run directory", c.RunDir},
		{"local API state", filepath.Join(c.RunDir, "local-api")},
		{"database", path},
		{"database WAL", path + "-wal"},
		{"database shared memory", path + "-shm"},
		{"database journal", path + "-journal"},
		{"control root", filepath.Join(c.RunDir, "control")},
		{"control requests", filepath.Join(c.RunDir, "control", "requests")},
		{"control receipts", filepath.Join(c.RunDir, "control", "receipts")},
		{"service root", filepath.Join(c.RunDir, "service")},
		{"service startups", filepath.Join(c.RunDir, "service", "startups")},
		{"Runner state", filepath.Join(c.RunDir, "runner")},
	} {
		if err := ensureNativePathWithin(c.RunDir, entry.path, entry.name); err != nil {
			return err
		}
		if c.RuntimeHome != "" {
			if err := ensureNativePathWithin(c.RuntimeHome, entry.path, entry.name+" application home"); err != nil {
				return err
			}
		}
	}
	if err := pathguard.RejectSharedSQLiteFiles(path); err != nil {
		return nativeErr("authority_mismatch", err.Error())
	}
	if _, err := os.Stat(path); err == nil {
		if err := ensureNativeStateFilesAllowed(c.RunDir, c.WorktreeMetadataDirectories); err != nil {
			return err
		}
		if c.ValidateNativeStore == nil {
			return errors.New("application native store validator is required")
		}
		return c.ValidateNativeStore(context.Background(), c)
	} else if !os.IsNotExist(err) {
		return err
	}
	return ensureNativeStateClean(c.RunDir, c.Repository)
}

func ensureNativePathWithin(root, path, description string) error {
	if !pathguard.Within(root, path) {
		return nativeErr("authority_mismatch", description+" resolves outside its managed root")
	}
	return nil
}

func PreflightNativeStatePath(c Config) error { return preflightNativeStatePath(c) }

func EnsureNativeStateClean(runDir, repository string) error {
	return ensureNativeStateClean(runDir, repository)
}

func ensureRunDirOutsideRepository(c Config) error {
	if c.RunDir == "" || c.Repository == "" {
		return nativeErr("authority_mismatch", "repository and run_dir are required")
	}
	resolvedRun, err := resolveProspectivePath(c.RunDir)
	if err != nil {
		return err
	}
	resolvedRepo, err := filepath.EvalSymlinks(c.Repository)
	if err != nil {
		return err
	}
	if inside(resolvedRepo, resolvedRun) {
		return nativeErr("authority_mismatch", "native state directory must be outside the repository")
	}
	if c.EnsureRunDirOutsideRepo == nil {
		return errors.New("application run-directory validator is required")
	}
	if err := c.EnsureRunDirOutsideRepo(context.Background(), c); err != nil {
		return err
	}
	return nil
}

func EnsureRunDirOutsideRepository(c Config) error {
	return ensureRunDirOutsideRepository(c)
}

func ensureNativeRepositoryHasNoBeads(repository string) error {
	for _, name := range []string{".beads", ".beads.db", "beads.db"} {
		if _, err := os.Lstat(filepath.Join(repository, name)); err == nil {
			return nativeErr("authority_mismatch", "native project cannot initialize while Beads state exists; preserve it for manual recovery")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func ensureNativeStateClean(runDir, repository string) error {
	if err := ensureNativeRepositoryHasNoBeads(repository); err != nil {
		return err
	}
	entries, err := os.ReadDir(runDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryPath := filepath.Join(runDir, entry.Name())
		if err := ensureNativePathWithin(runDir, entryPath, "native state entry"); err != nil {
			return err
		}
		if nativeAtomicTemp(entry.Name()) {
			continue
		}
		switch entry.Name() {
		case "service":
			if err := validateNativeServiceTree(runDir, repository); err != nil {
				return err
			}
		case "steerline.lock":
			if err := validateNativeRunLock(filepath.Join(runDir, entry.Name())); err != nil {
				return err
			}
		default:
			return nativeErr("authority_mismatch", "native initialization requires a clean state directory; found "+entry.Name())
		}
	}
	return nil
}

func ensureNativeStateFilesAllowed(runDir string, worktreeMetadata []string) error {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryPath := filepath.Join(runDir, entry.Name())
		if err := ensureNativePathWithin(runDir, entryPath, "native state entry"); err != nil {
			return err
		}
		if nativeAtomicTemp(entry.Name()) {
			info, statErr := os.Lstat(entryPath)
			if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return nativeErr("authority_mismatch", "native atomic projection temp is not a regular file")
			}
			continue
		}
		switch entry.Name() {
		case "steerline.db", "steerline.db-wal", "steerline.db-shm":
		case "steerline.lock":
			if err := validateNativeRunLock(filepath.Join(runDir, entry.Name())); err != nil {
				return err
			}
		case "service":
			if err := validateNativeServiceTree(runDir, ""); err != nil {
				return err
			}
		case "local-api", "runner", "application-commands":
			info, statErr := os.Lstat(entryPath)
			if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nativeErr("authority_mismatch", entry.Name()+" state must be a plain directory inside native run_dir")
			}
		case "control":
			if err := validateNativeControlTree(runDir); err != nil {
				return err
			}
		default:
			return nativeErr("authority_mismatch", "native state directory contains unsupported legacy or runtime data: "+entry.Name())
		}
	}
	return nil
}

func validateNativeRunLock(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != 0 {
		return nativeErr("authority_mismatch", "native service lock path is not an empty regular file")
	}
	return nil
}

func nativeAtomicTemp(name string) bool {
	return strings.HasPrefix(name, ".steerline-") && strings.HasSuffix(name, ".tmp")
}

func validateNativeControlTree(runDir string) error {
	root := filepath.Join(runDir, "control")
	if err := ensureNativePathWithin(runDir, root, "native control root"); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nativeErr("authority_mismatch", "native control protocol root must be a real directory")
	}
	roots, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, section := range roots {
		if !section.IsDir() || section.Name() != "requests" && section.Name() != "receipts" {
			return nativeErr("authority_mismatch", "native control directory contains unsupported data")
		}
		sectionPath := filepath.Join(root, section.Name())
		if err := ensureNativePathWithin(root, sectionPath, "native control section"); err != nil {
			return err
		}
		sectionInfo, statErr := os.Lstat(sectionPath)
		if statErr != nil || !sectionInfo.IsDir() || sectionInfo.Mode()&os.ModeSymlink != 0 {
			return nativeErr("authority_mismatch", "native control section must be a real directory")
		}
		entries, readErr := os.ReadDir(sectionPath)
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			if nativeAtomicTemp(entry.Name()) {
				continue
			}
			if section.Name() == "requests" {
				if !executionbackend.StableExecutionID(entry.Name()) || !entry.IsDir() {
					return nativeErr("authority_mismatch", "native control request has an invalid identity")
				}
				dir := filepath.Join(sectionPath, entry.Name())
				if err := ensureNativePathWithin(sectionPath, dir, "native control request"); err != nil {
					return err
				}
				children, childErr := os.ReadDir(dir)
				if childErr != nil || len(children) > 1 {
					return nativeErr("authority_mismatch", "native control request contains unsupported files")
				}
				for _, child := range children {
					if nativeAtomicTemp(child.Name()) {
						continue
					}
					if child.Name() != "request.json" || child.IsDir() {
						return nativeErr("authority_mismatch", "native control request contains unsupported files")
					}
					path := filepath.Join(dir, child.Name())
					if err := ensureNativePathWithin(dir, path, "native control request file"); err != nil {
						return err
					}
					request, parseErr := readControlRequest(path)
					if parseErr != nil || request.RequestID != entry.Name() {
						return nativeErr("authority_mismatch", "native control request is invalid or mismatched")
					}
				}
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".json") || !executionbackend.StableExecutionID(strings.TrimSuffix(entry.Name(), ".json")) || entry.IsDir() {
				return nativeErr("authority_mismatch", "native control receipt has an invalid identity")
			}
			path := filepath.Join(sectionPath, entry.Name())
			if err := ensureNativePathWithin(sectionPath, path, "native control receipt"); err != nil {
				return err
			}
			data, readErr := readBoundedFile(path, maxControlRequestBytes)
			var receipt productapi.ControlReceipt
			if readErr != nil || decode(data, &receipt) != nil || receipt.RequestID != strings.TrimSuffix(entry.Name(), ".json") || receipt.Version != ControlProtocolVersion {
				return nativeErr("authority_mismatch", "native control receipt is invalid or mismatched")
			}
		}
	}
	return nil
}

func validateNativeRunDirectories(root string, metadata []string) error {
	if err := ensureNativePathWithin(filepath.Dir(root), root, "native run/worktree root"); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nativeErr("authority_mismatch", "native run/worktree state root must be a real directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		childPath := filepath.Join(root, entry.Name())
		if err := ensureNativePathWithin(root, childPath, "native run/worktree directory"); err != nil {
			return err
		}
		childInfo, statErr := os.Lstat(childPath)
		if statErr != nil || !childInfo.IsDir() || childInfo.Mode()&os.ModeSymlink != 0 {
			return nativeErr("authority_mismatch", "native run/worktree identity is not a real directory: "+entry.Name())
		}
		if slices.Contains(metadata, entry.Name()) {
			// The native host owns this administrative directory. Preserve it and
			// never interpret them as active workflow or execution identities.
			continue
		}
		if !executionbackend.StableExecutionID(entry.Name()) {
			return nativeErr("authority_mismatch", "native run/worktree state contains an unknown identity: "+entry.Name())
		}
	}
	return nil
}

func validateNativeServiceTree(runDir, repository string) error {
	root := filepath.Join(runDir, "service")
	if err := ensureNativePathWithin(runDir, root, "native service root"); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nativeErr("authority_mismatch", "native service state must be a real directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if len(entries) != 1 || entries[0].Name() != "startups" || !entries[0].IsDir() {
		return nativeErr("authority_mismatch", "native service state contains unsupported control or legacy data")
	}
	startupRoot := filepath.Join(root, "startups")
	if err := ensureNativePathWithin(root, startupRoot, "native startup root"); err != nil {
		return err
	}
	startupInfo, err := os.Lstat(startupRoot)
	if err != nil || !startupInfo.IsDir() || startupInfo.Mode()&os.ModeSymlink != 0 {
		return nativeErr("authority_mismatch", "native startup records must be kept in a real startup directory")
	}
	startups, err := os.ReadDir(startupRoot)
	if err != nil {
		return err
	}
	for _, startup := range startups {
		if !executionbackend.StableExecutionID(startup.Name()) || !startup.IsDir() {
			return nativeErr("authority_mismatch", "native startup directory contains an unknown identity: "+startup.Name())
		}
		dir := filepath.Join(startupRoot, startup.Name())
		if err := ensureNativePathWithin(startupRoot, dir, "native startup directory"); err != nil {
			return err
		}
		children, readErr := os.ReadDir(dir)
		if readErr != nil {
			return nativeErr("authority_mismatch", "native startup record has an unsupported file set")
		}
		stableChildren := make([]os.DirEntry, 0, len(children))
		for _, child := range children {
			if !nativeAtomicTemp(child.Name()) {
				stableChildren = append(stableChildren, child)
			}
		}
		if len(stableChildren) < 2 || len(stableChildren) > 3 {
			return nativeErr("authority_mismatch", "native startup record has an unsupported file set")
		}
		seenRecord, seenLog, seenProcess := false, false, false
		for _, child := range stableChildren {
			childPath := filepath.Join(dir, child.Name())
			if err := ensureNativePathWithin(dir, childPath, "native startup file"); err != nil {
				return err
			}
			childInfo, statErr := os.Lstat(childPath)
			if statErr != nil || !childInfo.Mode().IsRegular() || childInfo.Mode()&os.ModeSymlink != 0 {
				return nativeErr("authority_mismatch", "native startup record contains a non-regular file")
			}
			switch child.Name() {
			case "startup.json":
				seenRecord = true
				data, readErr := os.ReadFile(childPath)
				var record StartupRecord
				if readErr != nil || decode(data, &record) != nil || record.Version != 1 || record.StartupID != startup.Name() || record.Status != "starting" && record.Status != "started" && record.Status != "failed" || canonical(record.RunDir) != canonical(runDir) {
					return nativeErr("authority_mismatch", "native startup record is unsupported or belongs to another state directory")
				}
				if repository != "" && canonical(record.RepositoryPath) != canonical(repository) {
					return nativeErr("authority_mismatch", "native startup record belongs to another repository")
				}
			case "service.log":
				seenLog = true
			case "process.json":
				seenProcess = true
				data, readErr := os.ReadFile(childPath)
				var process startupProcessRecord
				if readErr != nil || decode(data, &process) != nil || process.Version != 1 || process.StartupID != startup.Name() {
					return nativeErr("authority_mismatch", "native startup process record is unsupported or mismatched")
				}
			default:
				return nativeErr("authority_mismatch", "native startup directory contains unsupported data: "+child.Name())
			}
		}
		if !seenRecord || !seenLog || seenProcess && !seenRecord {
			return nativeErr("authority_mismatch", "native startup record or diagnostic log is missing")
		}
	}
	return nil
}

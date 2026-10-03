package steerline

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
)

func validateNativeStateLocation(c Config) error {
	if c.RepositoryStateKey == "" || c.StateRoot == "" {
		return nativeErr("authority_mismatch", "application configuration lacks its fixed repository state location")
	}
	if err := c.ResolveIdentity(context.Background()); err != nil {
		return err
	}
	if c.RepositoryStateKey != RepositoryStateKey(c.Identity()) {
		return nativeErr("authority_mismatch", "repository state key differs from the resolved repository identity")
	}
	expected := filepath.Join(c.StateRoot, c.RepositoryStateKey)
	if canonical(c.RunDir) != canonical(expected) {
		return nativeErr("authority_mismatch", "native tasks require the repository's stable shared state directory; remove the custom run_dir setting")
	}
	return nil
}

// RepositoryStateKey derives a pure storage identity from an explicit Git
// common directory. Application roots and default paths remain external.
func RepositoryStateKey(identity string) string {
	common := identity
	if resolved, err := filepath.EvalSymlinks(common); err == nil {
		common = resolved
	}
	name := filepath.Base(common)
	if strings.EqualFold(name, ".git") {
		name = filepath.Base(filepath.Dir(common))
	}
	name = strings.Trim(regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(name, "-"), "-.")
	if name == "" {
		name = "repo"
	}
	return name + "-" + shortHash(canonical(common))[:12]
}

func cleanupNewNativeDB(path string) {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(path + suffix)
	}
}

func openBoundNativeStore(ctx context.Context, c Config, path string) (_ *SQLiteStore, retErr error) {
	db, err := OpenSQLiteReader(path)
	if err != nil {
		return nil, nativeErr("unsupported_schema", "existing database is not a readable current SQLite database")
	}
	defer func() {
		if retErr != nil {
			_ = db.Close()
		}
	}()
	if err = validateRetiredWholeRunResultCommands(ctx, db.db); err != nil {
		return nil, err
	}
	if err = validateNativeSQLiteRuntimeProjection(ctx, db.db); err != nil {
		return nil, err
	}
	authority, err := db.NativeAuthority(ctx, c.Identity())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nativeErr("authority_mismatch", "database has no native authority for this project; preserve existing state")
	}
	if err != nil {
		return nil, nativeErr("unsupported_schema", "database does not contain the native task schema")
	}
	target, err := configuredRepositoryTarget(ctx, c)
	if err != nil {
		return nil, err
	}
	if authority.Authority != "native" || authority.Repository != target || authority.Remote != c.Remote || authority.Branch != c.Branch {
		return nil, nativeErr("authority_mismatch", "configuration repository, remote or branch differs from persisted native authority")
	}
	db.nativeBinding = &authority
	snapshot, err := db.Query(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	for _, task := range snapshot.Tasks {
		if task.Source != "native" || task.ExternalProvider != "" || task.ExternalProject != "" || task.ExternalID != "" || task.IdentityMapDigest != "" {
			return nil, nativeErr("authority_mismatch", "native center contains a task identity from an unsupported source")
		}
	}
	return db, nil
}

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
	if _, err := os.Stat(path); err == nil {
		if err := ensureNativeStateFilesAllowed(c.RunDir); err != nil {
			return err
		}
		reader, err := openBoundNativeStore(context.Background(), c, path)
		if err != nil {
			return err
		}
		return reader.Close()
	} else if !os.IsNotExist(err) {
		return err
	}
	return ensureNativeStateClean(c.RunDir, c.Repository)
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
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	worktrees, err := r.Output(context.Background(), c.Git, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nativeErr("authority_mismatch", "cannot verify native state against linked Git worktrees")
	}
	for _, record := range strings.Split(string(worktrees), "\x00") {
		if strings.HasPrefix(record, "worktree ") && inside(strings.TrimPrefix(record, "worktree "), resolvedRun) {
			return nativeErr("authority_mismatch", "native state directory must be outside every linked repository worktree")
		}
	}
	return nil
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
		if nativeAtomicTemp(entry.Name()) {
			continue
		}
		switch entry.Name() {
		case "service":
			if err := validateNativeServiceTree(runDir, repository); err != nil {
				return err
			}
		case "execution":
			if err := validateNativeExecutionArtifacts(filepath.Join(runDir, entry.Name())); err != nil {
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

func ensureNativeStateFilesAllowed(runDir string) error {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if nativeAtomicTemp(entry.Name()) {
			info, statErr := os.Lstat(filepath.Join(runDir, entry.Name()))
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
		case "control":
			if err := validateNativeControlTree(runDir); err != nil {
				return err
			}
		case "runs", "worktrees":
			if err := validateNativeRunDirectories(filepath.Join(runDir, entry.Name())); err != nil {
				return err
			}
		case "execution":
			if err := validateNativeExecutionArtifacts(filepath.Join(runDir, entry.Name())); err != nil {
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
				if !stableExecutionID(entry.Name()) || !entry.IsDir() {
					return nativeErr("authority_mismatch", "native control request has an invalid identity")
				}
				dir := filepath.Join(sectionPath, entry.Name())
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
					request, parseErr := readControlRequest(path)
					if parseErr != nil || request.RequestID != entry.Name() {
						return nativeErr("authority_mismatch", "native control request is invalid or mismatched")
					}
				}
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".json") || !stableExecutionID(strings.TrimSuffix(entry.Name(), ".json")) || entry.IsDir() {
				return nativeErr("authority_mismatch", "native control receipt has an invalid identity")
			}
			data, readErr := readBoundedFile(filepath.Join(sectionPath, entry.Name()), maxControlRequestBytes)
			var receipt ControlReceipt
			if readErr != nil || decode(data, &receipt) != nil || receipt.RequestID != strings.TrimSuffix(entry.Name(), ".json") || receipt.Version != ControlProtocolVersion {
				return nativeErr("authority_mismatch", "native control receipt is invalid or mismatched")
			}
		}
	}
	return nil
}

func validateNativeExecutionArtifacts(root string) error {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nativeErr("authority_mismatch", "native execution evidence root must be a real directory")
	}
	children, err := os.ReadDir(root)
	if err != nil || len(children) != 1 || children[0].Name() != "v1" || !children[0].IsDir() {
		return nativeErr("authority_mismatch", "native execution evidence contains an unsupported format")
	}
	v1 := filepath.Join(root, "v1")
	for _, folder := range []string{"capabilities", "evidence"} {
		dir := filepath.Join(v1, folder)
		entries, readErr := os.ReadDir(dir)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			if nativeAtomicTemp(entry.Name()) {
				continue
			}
			if entry.IsDir() || !regexp.MustCompile(`^[0-9a-f]{64}\.json$`).MatchString(entry.Name()) {
				return nativeErr("authority_mismatch", "native execution evidence contains an unknown file")
			}
			path := filepath.Join(dir, entry.Name())
			fileInfo, statErr := os.Lstat(path)
			if statErr != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 {
				return nativeErr("authority_mismatch", "native execution evidence must use regular files")
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if folder == "evidence" {
				sum := sha256.Sum256(data)
				if entry.Name() != hex.EncodeToString(sum[:])+".json" {
					return nativeErr("authority_mismatch", "native execution evidence digest differs from its filename")
				}
				continue
			}
			var report executionapi.CapabilityReport
			if decode(data, &report) != nil || report.Version != 1 || entry.Name() != capabilityReportKey(report)+".json" {
				return nativeErr("authority_mismatch", "native capability report identity or digest is invalid")
			}
			if _, verifyErr := (executionRecords{root: filepath.Dir(root)}).loadCapabilityReport(report); verifyErr != nil {
				return nativeErr("authority_mismatch", "native capability report evidence is invalid: "+verifyErr.Error())
			}
		}
	}
	return nil
}

func validateNativeSQLiteRuntimeProjection(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT payload FROM projection`)
	if err != nil {
		return nativeErr("unsupported_schema", "cannot inspect the saved runtime projection without mutation")
	}
	defer rows.Close()
	for rows.Next() {
		var payload []byte
		if err = rows.Scan(&payload); err != nil {
			return nativeErr("unsupported_schema", "cannot read the saved runtime projection without mutation")
		}
		var checkpoint ServiceCheckpoint
		if err = decode(payload, &checkpoint); err != nil || checkpoint.SchemaVersion != WorkerFlowRuntimeSchema || checkpoint.Repository.Identity == "" {
			return nativeErr("unsupported_schema", "unsupported service checkpoint; preserve the database")
		}

	}
	if err = rows.Err(); err != nil {
		return nativeErr("unsupported_schema", "cannot inspect the saved runtime projection without mutation")
	}
	return nil
}

func validateNativeRunDirectories(root string) error {
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
		if !stableExecutionID(entry.Name()) || !entry.IsDir() {
			return nativeErr("authority_mismatch", "native run/worktree state contains an unknown identity: "+entry.Name())
		}
		childInfo, statErr := os.Lstat(filepath.Join(root, entry.Name()))
		if statErr != nil || !childInfo.IsDir() || childInfo.Mode()&os.ModeSymlink != 0 {
			return nativeErr("authority_mismatch", "native run/worktree identity is not a real directory: "+entry.Name())
		}
	}
	return nil
}

func validateNativeServiceTree(runDir, repository string) error {
	root := filepath.Join(runDir, "service")
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
	startupInfo, err := os.Lstat(startupRoot)
	if err != nil || !startupInfo.IsDir() || startupInfo.Mode()&os.ModeSymlink != 0 {
		return nativeErr("authority_mismatch", "native startup records must be kept in a real startup directory")
	}
	startups, err := os.ReadDir(startupRoot)
	if err != nil {
		return err
	}
	for _, startup := range startups {
		if !stableExecutionID(startup.Name()) || !startup.IsDir() {
			return nativeErr("authority_mismatch", "native startup directory contains an unknown identity: "+startup.Name())
		}
		dir := filepath.Join(startupRoot, startup.Name())
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

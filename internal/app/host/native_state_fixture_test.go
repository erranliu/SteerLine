package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

// These test-only helpers prepare SQLite state for service-host fixtures. App
// project construction is covered through the real app entry point in tests/integration.
func openHostFixtureNativeProject(ctx context.Context, c *execution.Config) (*sqlite.SQLiteStore, error) {
	if c == nil {
		return nil, nativeErr("authority_mismatch", "native task project config is required")
	}
	if c.Repository == "" || c.Remote == "" || c.Branch == "" {
		return nil, errors.New("repository, remote and branch are required")
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := resolveHostTestIdentity(ctx, c); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); err == nil {
		return openBoundNativeStore(ctx, *c, path)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	lock, err := LockRepository(c.Identity())
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err = ensureNativeStateClean(c.RunDir, c.Repository, fixtureCapabilityValidation{}); err != nil {
		return nil, err
	}
	target, err := c.TaskRepositoryTarget(ctx, testProcessHost{})
	if err != nil {
		return nil, err
	}
	writer, err := sqlite.OpenSQLiteStore(path)
	if err != nil {
		return nil, err
	}
	authority := executionmodel.ProjectTaskAuthority{ProjectIdentity: c.Identity(), Authority: "native", AuthorityVersion: 1, GraphRevision: 0, Repository: target, Remote: c.Remote, Branch: c.Branch, DispatchMode: "manual", DispatchPaused: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := writer.InitializeProject(ctx, authority); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return openBoundNativeStore(ctx, *c, path)
}

func openHostFixtureNativeReader(ctx context.Context, c *execution.Config) (*sqlite.SQLiteStore, error) {
	if c == nil {
		return nil, nativeErr("authority_mismatch", "native task reader config is required")
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := resolveHostTestIdentity(ctx, c); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return openBoundNativeStore(ctx, *c, path)
}

func openHostFixtureNativeCommandStore(ctx context.Context, c *execution.Config) (*sqlite.SQLiteStore, error) {
	if c == nil {
		return nil, nativeErr("authority_mismatch", "native task command config is required")
	}
	if c.Repository == "" || c.Remote == "" || c.Branch == "" {
		return nil, errors.New("repository, remote and branch are required")
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := resolveHostTestIdentity(ctx, c); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	reader, err := openBoundNativeStore(ctx, *c, path)
	if err != nil {
		return nil, err
	}
	_ = reader.Close()
	writer, err := sqlite.OpenSQLiteStore(path)
	if err != nil {
		return nil, err
	}
	if _, err := writer.NativeAuthority(ctx, c.Identity()); err != nil {
		_ = writer.Close()
		if errors.Is(err, storageports.ErrNotFound) {
			return nil, nativeErr("authority_mismatch", "native project binding changed before command admission")
		}
		return nil, err
	}
	return writer, nil
}

func openBoundNativeStore(ctx context.Context, c execution.Config, path string) (_ *sqlite.SQLiteStore, retErr error) {
	reader, err := sqlite.OpenSQLiteReader(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = reader.Close()
		}
	}()
	if err := reader.ValidateStoredFormat(ctx); err != nil {
		return nil, err
	}
	authority, err := reader.NativeAuthority(ctx, c.Identity())
	if errors.Is(err, storageports.ErrNotFound) {
		return nil, nativeErr("authority_mismatch", "database has no native authority for this project; preserve existing state")
	}
	if err != nil {
		return nil, err
	}
	target, err := c.TaskRepositoryTarget(ctx, testProcessHost{})
	if err != nil {
		return nil, err
	}
	if authority.Authority != "native" || authority.Repository != target || authority.Remote != c.Remote || authority.Branch != c.Branch {
		return nil, nativeErr("authority_mismatch", "configuration repository, remote or branch differs from persisted native authority")
	}
	reader.BindAuthority(authority)
	state, err := reader.ReadExecutionStateSnapshot(ctx, c.Identity())
	if err != nil {
		return nil, err
	}
	for _, task := range state.Graph.Tasks {
		if task.Source != "native" || task.ExternalProvider != "" || task.ExternalProject != "" || task.ExternalID != "" || task.IdentityMapDigest != "" {
			return nil, nativeErr("authority_mismatch", "native center contains a task identity from an unsupported source")
		}
	}
	return reader, nil
}

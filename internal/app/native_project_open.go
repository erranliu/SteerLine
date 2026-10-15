package app

import (
	"context"
	"os"
	"path/filepath"
	"time"

	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/pathguard"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"github.com/erranliu/SteerLine/internal/taskrunner"
)

func LoadDatabaseRuntime(c Config) (*reconstruction.Runtime, error) {
	if err := c.RequireNativeTaskAuthority(); err != nil {
		return nil, err
	}
	if c.Identity() == "" {
		if err := c.ResolveIdentity(context.Background()); err != nil {
			return nil, err
		}
	}
	reader, err := OpenNativeTaskReader(context.Background(), &c)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	snapshot, err := reader.ReadRunnerSnapshot(context.Background(), c.Identity())
	if err != nil {
		return nil, err
	}
	if snapshot.Checkpoint == nil {
		return nil, storageports.ErrNotFound
	}
	view := taskrunner.Project(snapshot, *snapshot.Checkpoint)
	return &view, nil
}

func OpenNativeTaskProject(ctx context.Context, c *Config) (*sqlite.SQLiteStore, error) {
	if c == nil {
		return nil, taskError("authority_mismatch", "native project configuration is required")
	}
	if c.RequireNativeTaskAuthority() != nil {
		return nil, taskError("authority_mismatch", "configuration must explicitly select task_authority=native")
	}
	if err := c.ValidateTaskCommandConfiguration(); err != nil {
		return nil, err
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	if err := PreflightNativeStatePath(*c); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); err == nil {
		return openBoundNativeReader(ctx, *c, path)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	lock, err := steerline.LockRepository(c.Identity())
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := PreflightNativeStatePath(*c); err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil {
		return openBoundNativeReader(ctx, *c, path)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := steerline.EnsureNativeStateClean(c.RunDir, c.Repository); err != nil {
		return nil, err
	}
	target, err := c.TaskRepositoryTarget(ctx)
	if err != nil {
		return nil, err
	}
	if err := pathguard.RejectSharedSQLiteFiles(path); err != nil {
		return nil, err
	}
	writer, err := sqlite.OpenSQLiteStore(path)
	if err != nil {
		return nil, err
	}
	authority := executionmodel.ProjectTaskAuthority{ProjectIdentity: c.Identity(), Authority: "native", AuthorityVersion: 1, GraphRevision: 0, Repository: target, Remote: c.Remote, Branch: c.Branch, DispatchMode: "manual", DispatchPaused: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err = writer.InitializeProject(ctx, authority); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	return openBoundNativeReader(ctx, *c, path)
}

func OpenNativeTaskReader(ctx context.Context, c *Config) (*sqlite.SQLiteStore, error) {
	if c == nil || c.RequireNativeTaskAuthority() != nil {
		return nil, taskError("authority_mismatch", "native task reader requires task_authority=native")
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return openBoundNativeReader(ctx, *c, path)
}

func OpenNativeTaskCommandStore(ctx context.Context, c *Config) (*sqlite.SQLiteStore, error) {
	if c == nil || c.RequireNativeTaskAuthority() != nil {
		return nil, taskError("authority_mismatch", "native task command requires task_authority=native")
	}
	if c.ExecutionDirectory == nil {
		return nil, taskError("unsupported_profile", "execution directory is required")
	}
	for _, profile := range c.AvailableExecutionProfiles() {
		if _, _, err := c.ExecutionDirectory.NormalizeProfileInputs(profile.ImplementationConfigs, profile.Host, profile.Agent, execution.ConfigurationProjectIdentity(c.executionBase()), profile.ID, profile.AcceptedConfigVersion); err != nil {
			return nil, err
		}
	}
	if err := c.ValidateTaskCommandConfiguration(); err != nil {
		return nil, err
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	reader, err := openBoundNativeReader(ctx, *c, path)
	if err != nil {
		return nil, err
	}
	_ = reader.Close()
	if err := pathguard.RejectSharedSQLiteFiles(path); err != nil {
		return nil, err
	}
	writer, err := sqlite.OpenSQLiteStore(path)
	if err != nil {
		return nil, err
	}
	if err := validateWriterBinding(ctx, *c, writer); err != nil {
		_ = writer.Close()
		return nil, err
	}
	return writer, nil
}

func openBoundNativeReader(ctx context.Context, c Config, path string) (_ *sqlite.SQLiteStore, retErr error) {
	if err := validateRuntimeHomePaths(c); err != nil {
		return nil, err
	}
	reader, err := sqlite.OpenSQLiteReader(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			_ = reader.Close()
		}
	}()
	if err = validateWriterBinding(ctx, c, reader); err != nil {
		return nil, err
	}
	return reader, nil
}

func validateWriterBinding(ctx context.Context, c Config, store *sqlite.SQLiteStore) error {
	if err := store.ValidateStoredFormat(ctx); err != nil {
		return err
	}
	graph, err := store.TaskGraphSnapshot(ctx, c.Identity())
	if err != nil {
		return err
	}
	authority := graph.Authority
	target, err := c.TaskRepositoryTarget(ctx)
	if err != nil {
		return err
	}
	if authority.ProjectIdentity != c.Identity() || authority.Authority != "native" || authority.Repository != target || authority.Remote != c.Remote || authority.Branch != c.Branch {
		return taskError("authority_mismatch", "configuration repository, remote or branch differs from persisted native authority")
	}
	store.BindAuthority(authority)
	for _, task := range graph.Tasks {
		if task.Source != "native" || task.ExternalProvider != "" || task.ExternalProject != "" || task.ExternalID != "" || task.IdentityMapDigest != "" {
			return taskError("authority_mismatch", "native center contains a task identity from an unsupported source")
		}
	}
	return nil
}

func taskError(code, message string) error { return &taskmodel.Error{Code: code, Message: message} }

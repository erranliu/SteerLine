package steerlineapp

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/execution"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	"github.com/erranliu/SteerLine/internal/execution/reconstruction"
	"github.com/erranliu/SteerLine/internal/steerline"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

func configureNativeStorage(c *execution.Config) {
	if c.ReadRuntime == nil {
		c.ReadRuntime = func(_ context.Context, config execution.Config) (*steerline.Runtime, error) {
			return LoadDatabaseRuntime(config)
		}
	}
	if c.ReadPersistedExecutionState == nil {
		c.ReadPersistedExecutionState = func(ctx context.Context, config execution.Config) (storageports.ExecutionStateSnapshot, error) {
			reader, err := OpenNativeTaskReader(ctx, &config)
			if err != nil {
				return storageports.ExecutionStateSnapshot{}, err
			}
			defer reader.Close()
			return reader.ReadExecutionStateSnapshot(ctx, config.Identity())
		}
	}
	if c.ValidateNativeStore == nil {
		c.ValidateNativeStore = func(ctx context.Context, config execution.Config) error {
			path := filepath.Join(config.RunDir, "steerline.db")
			reader, err := openBoundNativeReader(ctx, config, path)
			if err != nil {
				return err
			}
			return reader.Close()
		}
	}
	if c.EnsureNativeProject == nil {
		c.EnsureNativeProject = func(ctx context.Context, config execution.Config) error {
			store, err := OpenNativeTaskProject(ctx, &config)
			if err != nil {
				return err
			}
			return store.Close()
		}
	}
}

func LoadDatabaseRuntime(c execution.Config) (*steerline.Runtime, error) {
	configureNativeStorage(&c)
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
	snapshot, err := reader.ReadExecutionStateSnapshot(context.Background(), c.Identity())
	if err != nil {
		return nil, err
	}
	if snapshot.Checkpoint == nil {
		return nil, storageports.ErrNotFound
	}
	view, err := reconstruction.BuildFromStorage(snapshot, *snapshot.Checkpoint)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func OpenNativeTaskProject(ctx context.Context, c *execution.Config) (*sqlite.SQLiteStore, error) {
	if c == nil {
		return nil, taskError("authority_mismatch", "native project configuration is required")
	}
	configureNativeStorage(c)
	if c.RequireNativeTaskAuthority() != nil {
		return nil, taskError("authority_mismatch", "configuration must explicitly select task_authority=native")
	}
	if err := c.ValidateTaskCommandConfiguration(); err != nil {
		return nil, err
	}
	if err := steerline.ValidateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	if err := steerline.PreflightNativeStatePath(*c); err != nil {
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
	if err := steerline.PreflightNativeStatePath(*c); err != nil {
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

func OpenNativeTaskReader(ctx context.Context, c *execution.Config) (*sqlite.SQLiteStore, error) {
	if c == nil || c.RequireNativeTaskAuthority() != nil {
		return nil, taskError("authority_mismatch", "native task reader requires task_authority=native")
	}
	configureNativeStorage(c)
	if err := steerline.ValidateNativeStateLocation(*c); err != nil {
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

func OpenNativeTaskCommandStore(ctx context.Context, c *execution.Config) (*sqlite.SQLiteStore, error) {
	if c == nil || c.RequireNativeTaskAuthority() != nil {
		return nil, taskError("authority_mismatch", "native task command requires task_authority=native")
	}
	configureNativeStorage(c)
	if err := c.ValidateTaskCommandConfiguration(); err != nil {
		return nil, err
	}
	if err := steerline.ValidateNativeStateLocation(*c); err != nil {
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

func openBoundNativeReader(ctx context.Context, c execution.Config, path string) (_ *sqlite.SQLiteStore, retErr error) {
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

func validateWriterBinding(ctx context.Context, c execution.Config, store *sqlite.SQLiteStore) error {
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

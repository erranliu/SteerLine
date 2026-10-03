package steerline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline/persistence"
)

func OpenNativeTaskProject(ctx context.Context, c *Config) (persistence.Connection, error) {
	if c == nil {
		return nil, nativeErr("authority_mismatch", "native project configuration is required")
	}
	if requireNativeTaskAuthority(*c) != nil {
		return nil, nativeErr("authority_mismatch", "configuration must explicitly select task_authority=native")
	}
	if err := validateWorkflowConfig(c, nil); err != nil {
		return nil, err
	}
	if c.Persistence == nil {
		return nil, nativeErr("authority_mismatch", "persistence factory is required")
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	if err := preflightNativeStatePath(*c); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	if _, err := os.Stat(path); err == nil {
		// Existing readers must coexist with the repository's single writer.
		return openBoundNativeStore(ctx, *c, path)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	lock, err := LockRepository(c.Identity())
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if err := preflightNativeStatePath(*c); err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return openBoundNativeStore(ctx, *c, path)
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	}
	if err := ensureNativeStateClean(c.RunDir, c.Repository); err != nil {
		return nil, err
	}
	targetRepository, err := configuredRepositoryTarget(ctx, *c)
	if err != nil {
		return nil, err
	}
	db, err := c.Persistence.OpenWriter(path)
	if err != nil {
		return nil, err
	}
	authority := ProjectTaskAuthority{ProjectIdentity: c.Identity(), Authority: "native", AuthorityVersion: 1, GraphRevision: 0, Repository: targetRepository, Remote: c.Remote, Branch: c.Branch, DispatchMode: "manual", DispatchPaused: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	var initializer persistence.Projects = db
	err = initializer.InitializeProject(ctx, authority)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = db.Close(); err != nil {
		return nil, err
	}
	return openBoundNativeStore(ctx, *c, path)
}

// OpenNativeTaskReader opens an existing native authority without creating
// project state. Every read is bound to the configured repository and target.
func OpenNativeTaskReader(ctx context.Context, c *Config) (persistence.Connection, error) {
	if c == nil || c.Persistence == nil || requireNativeTaskAuthority(*c) != nil {
		return nil, nativeErr("authority_mismatch", "native task reader requires task_authority=native")
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
	return openBoundNativeStore(ctx, *c, path)
}

// OpenNativeTaskCommandStore opens the repository's existing native database
// for a bounded command. The caller must hold the repository identity lock,
// and the project must already have been initialized and bound.
func OpenNativeTaskCommandStore(ctx context.Context, c *Config) (persistence.Connection, error) {
	if c == nil || requireNativeTaskAuthority(*c) != nil {
		return nil, nativeErr("authority_mismatch", "native task command requires task_authority=native")
	}
	if err := validateWorkflowConfig(c, nil); err != nil {
		return nil, err
	}
	if c.Persistence == nil {
		return nil, nativeErr("authority_mismatch", "persistence factory is required")
	}
	if err := validateNativeStateLocation(*c); err != nil {
		return nil, err
	}
	if err := c.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	path := filepath.Join(c.RunDir, "steerline.db")
	reader, err := openBoundNativeStore(ctx, *c, path)
	if err != nil {
		return nil, err
	}
	_ = reader.Close()
	writer, err := c.Persistence.OpenWriter(path)
	if err != nil {
		return nil, err
	}
	authority, err := writer.NativeAuthority(ctx, c.Identity())
	if err != nil {
		_ = writer.Close()
		if errors.Is(err, persistence.ErrNotFound) {
			return nil, nativeErr("authority_mismatch", "native project binding changed before command admission")
		}
		return nil, err
	}
	if authority.Authority != "native" || authority.Remote != c.Remote || authority.Branch != c.Branch {
		_ = writer.Close()
		return nil, nativeErr("authority_mismatch", "native project binding changed before command admission")
	}
	target, err := configuredRepositoryTarget(ctx, *c)
	if err != nil {
		_ = writer.Close()
		return nil, err
	}
	if !sameRepositoryTarget(authority.Repository, target) {
		_ = writer.Close()
		return nil, nativeErr("authority_mismatch", "repository target differs from the persisted native project")
	}
	writer.BindAuthority(authority)
	return writer, nil
}

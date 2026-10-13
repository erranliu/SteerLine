package app

import (
	"context"
	"errors"

	"github.com/erranliu/SteerLine/internal/console"
	"github.com/erranliu/SteerLine/internal/consoleapp"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

// PrepareConsole constructs each project's sole product adapter outside the UI.
func consolePaths() consoleapp.Paths {
	info := CurrentRuntimeIdentity("")
	return consoleapp.Paths{Home: HomeDir(), Bin: BinDir(), Executable: info.Executable, SourceCommit: info.SourceCommit, SourceDirty: info.SourceDirty}
}
func PrepareConsole(ctx context.Context, configPath, runID string) ([]api.ProjectRef, console.Options, error) {
	return consoleapp.Prepare(ctx, configPath, runID, consolePaths())
}
func RunConsole(ctx context.Context, configPath, runID string) (err error) {
	refs, options, err := PrepareConsole(ctx, configPath, runID)
	if err != nil {
		return err
	}
	defer func() {
		for _, c := range options.Clients {
			err = errors.Join(err, c.Close())
		}
	}()
	return console.Run(ctx, refs, options)
}

// LoadProjects reads either one explicit config or all configs under etc.
// Invalid project files stay in the list so their errors remain visible.
func LoadConsoleProjects(configPath string) ([]api.ProjectRef, error) {
	return consoleapp.Load(context.Background(), configPath, consolePaths())
}

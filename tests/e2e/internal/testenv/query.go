package testenv

import (
	"context"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
)

func openView(ctx context.Context, database, project string) (storageports.ProjectView, error) {
	return driver.ReadProjectView(ctx, database, project)
}

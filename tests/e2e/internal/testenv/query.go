package testenv

import (
	"context"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/tests/e2e/internal/driver"
)

func openView(ctx context.Context, database, project string) (contracts.ProjectView, error) {
	return driver.ReadProjectView(ctx, database, project)
}

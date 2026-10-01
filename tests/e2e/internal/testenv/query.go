package testenv

import (
	"context"
	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
)

func openView(ctx context.Context, database, project string) (contracts.ProjectView, error) {
	reader, err := steerline.OpenSQLiteReader(database)
	if err != nil {
		return contracts.ProjectView{}, err
	}
	defer reader.Close()
	return steerline.NewQueryService(reader, project).ReadProjectView(ctx, "", 100)
}

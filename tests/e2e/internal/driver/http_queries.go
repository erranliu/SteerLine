package driver

import (
	"context"
	"net/url"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
)

func ReadHTTPProject(ctx context.Context, runDir, project, task string) (api.ProjectSnapshot, error) {
	client, err := httpclient.Open(ctx, runDir, project)
	if err != nil {
		return api.ProjectSnapshot{}, err
	}
	defer client.Close()
	return client.Snapshot(ctx, task)
}

func ReadHTTPRecovery(ctx context.Context, runDir, project, run, execution, attempt string) (api.RecoveryInspection, error) {
	client, err := httpclient.Open(ctx, runDir, project)
	if err != nil {
		return api.RecoveryInspection{}, err
	}
	defer client.Close()
	var result api.RecoveryInspection
	query := url.Values{"execution_id": {execution}, "attempt_id": {attempt}}
	err = client.Get(ctx, "/runs/"+url.PathEscape(run)+"/recovery?"+query.Encode(), &result)
	return result, err
}

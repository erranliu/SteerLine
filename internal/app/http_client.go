package app

import (
	"context"
	"fmt"

	httpapi "github.com/erranliu/SteerLine/internal/product/httpclient"
)

// OpenHTTPClient connects to the already running instance discovered for this
// configured project. It never starts or replaces the service.
func OpenHTTPClient(ctx context.Context, configPath string) (*httpapi.Client, error) {
	config, err := LoadQueryConfig(configPath)
	if err != nil {
		return nil, err
	}
	if err = config.ResolveIdentity(ctx); err != nil {
		return nil, err
	}
	client, err := httpapi.Open(ctx, config.RunDir, config.Identity())
	if err == nil {
		return client, nil
	}
	return nil, fmt.Errorf("local HTTP service unavailable: %w", err)
}

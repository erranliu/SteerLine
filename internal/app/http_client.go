package app

import (
	"context"
	"fmt"

	"github.com/erranliu/SteerLine/internal/product/httpapi"
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
	connection := connectHostService(ctx, config)
	if connection.Diagnostic != "" {
		return nil, fmt.Errorf("existing service state %s: %s; local API: %w", connection.State, connection.Diagnostic, err)
	}
	return nil, err
}

package app

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/erranliu/SteerLine/internal/pathguard"
	"github.com/erranliu/SteerLine/internal/product/applicationclient"
)

func OpenApplicationProject(ctx context.Context, selector, config string) (*applicationclient.Project, error) {
	if selector != "" && config != "" {
		return nil, errors.New("select project with either --project or --config")
	}
	if config != "" {
		absolute, err := filepath.Abs(config)
		if err != nil {
			return nil, err
		}
		if !pathguard.Within(EtcDir(), absolute) || filepath.Dir(absolute) != EtcDir() {
			return nil, errors.New("configuration selector must name a file in this home/etc; use --project")
		}
		selector = filepath.Base(config)
	}
	return applicationclient.OpenProject(ctx, HomeDir(), selector)
}

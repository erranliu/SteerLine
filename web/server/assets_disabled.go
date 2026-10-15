//go:build !webui

package server

import (
	"errors"
	"io/fs"
)

func EmbeddedAssets() (fs.FS, error) {
	return nil, errors.New("Web assets are unavailable: npm --prefix web ci; npm --prefix web run build; go build -tags webui ./cmd/steerline")
}

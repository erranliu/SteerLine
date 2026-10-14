//go:build webui

package server

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

func EmbeddedAssets() (fs.FS, error) { return fs.Sub(embedded, "dist") }

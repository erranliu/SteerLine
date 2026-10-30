package driver

import (
	"github.com/erranliu/SteerLine/internal/app"
)

// DecodeProjectConfig keeps the app-owned file format inside the test adapter.
// Scenarios receive the current value without importing the app implementation.
func DecodeProjectConfig(raw []byte) (app.ProjectConfigInput, error) {
	return app.DecodeProjectConfigInput(raw)
}

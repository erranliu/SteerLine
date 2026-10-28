package driver

import (
	"encoding/json"

	"github.com/erranliu/SteerLine/internal/app"
)

// DecodeProjectConfig keeps the app-owned file format inside the test adapter.
// Scenarios receive the current value without importing the app implementation.
func DecodeProjectConfig(raw []byte) (app.ProjectConfigInput, error) {
	var input app.ProjectConfigInput
	err := json.Unmarshal(raw, &input)
	return input, err
}

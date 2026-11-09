package api

import (
	"context"
	"encoding/json"
)

type TestManagementPort interface {
	PlanTests(context.Context, string) (json.RawMessage, error)
}

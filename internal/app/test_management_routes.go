package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	api "github.com/erranliu/SteerLine/internal/product/api"
	"github.com/erranliu/SteerLine/internal/product/httpclient"
	"github.com/erranliu/SteerLine/internal/product/httpcontract"
)

func (p *runtimeProjectPort) testing(ctx context.Context, method, route string, body json.RawMessage) (any, error) {
	port := p.config.runtimeTests
	if port == nil {
		return nil, &api.Error{Kind: api.Unsupported, Message: "test management unavailable"}
	}
	if method == "GET" {
		return port.TestGeneration(ctx, strings.TrimPrefix(route, "/tests/generations/"))
	}
	var in struct {
		ProjectID  string          `json:"project_id"`
		InstanceID string          `json:"instance_id"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := decodeStrictJSON(body, &in); err != nil {
		return nil, err
	}
	if in.ProjectID != httpcontract.ProjectID(p.config.Identity()) || in.InstanceID != p.Server.Identity(ctx).InstanceID {
		return nil, &httpclient.ClientError{Status: 409, Code: "stale_target", Reason: "generation target differs"}
	}
	if p.stopping.Load() || !p.Server.Identity(ctx).Ready {
		return nil, projectStoppingError()
	}
	switch route {
	case "/tests/stop-generation":
		var cmd api.TestStopGenerationRequest
		if err := decodeStrictJSON(in.Payload, &cmd); err != nil {
			return nil, err
		}
		return port.StopTestGeneration(ctx, cmd)
	}
	return nil, errors.New("unknown generation route")
}

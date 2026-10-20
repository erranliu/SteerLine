package applicationclient

import (
	"context"
	api "github.com/erranliu/SteerLine/internal/product/api"
)

func (p *Project) GenerateTests(ctx context.Context, in api.TestGenerateRequest) (out api.TestGeneration, err error) {
	err = p.task(ctx, "/tests/generate", in, &out)
	return
}
func (p *Project) TestGeneration(ctx context.Context, id string) (out api.TestGeneration, err error) {
	err = p.get(ctx, "/tests/generations/"+id, &out)
	return
}
func (p *Project) StopTestGeneration(ctx context.Context, in api.TestStopGenerationRequest) (out api.TestGeneration, err error) {
	err = p.task(ctx, "/tests/stop-generation", in, &out)
	return
}

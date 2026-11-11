package roadmap

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
	tm "github.com/erranliu/SteerLine/internal/testmanagement/model"
)

func (s *Service) planningMaterials(ctx context.Context, commit string) ([]runnercontract.Document, error) {
	if s.ReadCommitted == nil {
		return nil, errors.New("缺少固定规划材料读取能力")
	}
	read := func(path string) ([]byte, error) { return s.ReadCommitted(ctx, commit, path) }
	entry, err := tm.ReadProjectEntry(read)
	if err != nil {
		return nil, err
	}
	out := []runnercontract.Document{}
	for _, path := range entry.Paths() {
		raw, err := read(path)
		if err != nil {
			return nil, err
		}
		if len(raw) == 0 || len(raw) > model.MaxBytes {
			return nil, fmt.Errorf("规划材料为空或过大：%s", path)
		}
		body := string(raw)
		out = append(out, runnercontract.Document{Kind: "inline", Path: path, Content: &body})
	}
	return out, nil
}
func (s *Service) verifyPlanning(ctx context.Context, r model.Record, commit string) error {
	for _, m := range r.PlanningMaterials {
		raw, err := s.ReadCommitted(ctx, commit, m.Path)
		if err != nil || m.Content == nil || !bytes.Equal(raw, []byte(*m.Content)) {
			return fmt.Errorf("规划材料已变化，请重新解析：%s", m.Path)
		}
	}
	return nil
}
func validateProviders(r model.Record) error {
	providers, err := model.TestProviders(r)
	if err != nil {
		return err
	}
	catalogue, err := TestCatalogue(r)
	if err != nil {
		return err
	}
	for _, d := range catalogue.Tests {
		if providers[d.ID] != "" {
			return fmt.Errorf("新增测试已在固定清单登记：%s", d.ID)
		}
	}
	return nil
}

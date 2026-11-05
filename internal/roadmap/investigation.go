package roadmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/erranliu/SteerLine/internal/roadmap/model"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

// Repository citations are qualified against original Git objects, not hashes
// copied by the model. Input-file citations use the already verified request.
func (s *Service) verifyInvestigation(ctx context.Context, r model.Record, raw []byte, source *runnercontract.RepositorySource) error {
	if s.ReadCommitted == nil || source == nil {
		return errors.New("缺少固定调查文件读取能力")
	}
	var result model.GeneratedDocuments
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	files := map[string]runnercontract.RepositoryFile{}
	for _, file := range source.Files {
		files[file.Path] = file
	}
	for _, ref := range result.Investigation {
		if ref.Source != "repository" {
			continue
		}
		file, exists := files[ref.Path]
		if !exists {
			return fmt.Errorf("固定仓库缺少调查文件：%s", ref.Path)
		}
		body, err := s.ReadCommitted(ctx, r.CodeCommit, ref.Path)
		if err != nil || model.Digest(body) != file.SHA256 || len(body) != file.SizeBytes {
			return fmt.Errorf("调查文件与固定 Git 不一致：%s", ref.Path)
		}
	}
	return nil
}

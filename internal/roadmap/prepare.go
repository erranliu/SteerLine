package roadmap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/erranliu/SteerLine/internal/pathguard"
	"github.com/erranliu/SteerLine/internal/prompts"
)

type PreparedFile struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

// A deterministic content conflict is distinct from an interrupted or failed
// filesystem read when reconciling an original initialization request.
var ErrStaticRuleConflict = errors.New("静态规则内容冲突")

// PrepareStatic is explicit project preparation, never a runtime fallback.
// Existing project guides are owned by the project; platform rules are versioned.
func PrepareStatic(repository string, check bool) ([]PreparedFile, error) {
	if !filepath.IsAbs(repository) {
		return nil, errors.New("项目准备需要仓库绝对路径")
	}
	root, err := pathguard.ResolveProspective(repository)
	if err != nil || !strings.EqualFold(filepath.Clean(root), filepath.Clean(repository)) {
		return nil, errors.New("项目准备目录不可读或存在重定向")
	}
	loaders := []struct {
		path string
		load func() ([]byte, error)
		rule bool
	}{{ParseRulesPath, prompts.RoadmapParseRules, true}, {DocumentRulesPath, prompts.RoadmapDocumentRules, true}, {"SteerlineFlow/spec/SpecGuide.md", prompts.RoadmapSpecGuide, false}}
	result := []PreparedFile{}
	for _, item := range loaders {
		target := filepath.Join(root, filepath.FromSlash(item.path))
		resolved, err := pathguard.ResolveProspective(target)
		if err != nil || !strings.EqualFold(resolved, target) {
			return result, errors.New("静态材料路径存在重定向")
		}
		expected, err := item.load()
		if err != nil {
			return result, err
		}
		actual, err := ReadRegular(root, item.path)
		if err == nil {
			if item.rule && !prompts.SameRules(actual, expected) {
				return result, fmt.Errorf("静态规则与当前版本不一致，需审阅更新：%s：%w", item.path, ErrStaticRuleConflict)
			}
			result = append(result, PreparedFile{Path: item.path})
			continue
		}
		if !os.IsNotExist(err) || check {
			return result, fmt.Errorf("静态材料未准备：%s：%w", item.path, err)
		}
		cursor := root
		parts := strings.Split(item.path, "/")
		for _, part := range parts[:len(parts)-1] {
			cursor = filepath.Join(cursor, part)
			if err := os.Mkdir(cursor, 0700); err != nil && !os.IsExist(err) {
				return result, err
			}
			actual, err := pathguard.ResolveProspective(cursor)
			if err != nil || !strings.EqualFold(actual, cursor) {
				return result, errors.New("静态材料目录存在重定向")
			}
		}
		file, err := os.OpenFile(filepath.Join(root, filepath.FromSlash(item.path)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return result, err
		}
		_, writeErr := file.Write(expected)
		syncErr := file.Sync()
		if err := errors.Join(writeErr, syncErr, file.Close()); err != nil {
			return result, err
		}
		result = append(result, PreparedFile{Path: item.path, Created: true})
	}
	return result, nil
}

package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/erranliu/SteerLine/internal/manualverification"
)

var commitPattern = regexp.MustCompile("^(?:[0-9a-f]{40}|[0-9a-f]{64})$")

func ValidateDocuments(r Record) error {
	if !r.DocumentsVerified || r.Analysis == nil || r.ConfirmedDigest != ConfirmationDigest(r) {
		return errors.New("未取得与已确认规划绑定、经核实的 Runner 文档结果")
	}
	if !commitPattern.MatchString(r.CodeCommit) {
		return errors.New("文档未固定完整代码提交")
	}
	if _, err := TestProviders(r); err != nil {
		return err
	}
	expected := map[string]bool{}
	for _, t := range r.Analysis.Tasks {
		if t.ExistingID == "" {
			expected[t.Key] = true
		}
	}
	seen := map[string]bool{}
	paths := map[string]bool{}
	total := 0
	for _, d := range r.Documents {
		if !expected[d.Key] || seen[d.Key] {
			return errors.New("文档与本批内部代号不匹配")
		}
		seen[d.Key] = true
		if err := validateDocumentContent(d, paths, &total); err != nil {
			return err
		}
	}

	if len(seen) != len(expected) {
		return errors.New("正式文档集不完整，不能发布")
	}
	return nil
}

// TestsJSON is the single encoding of the committed arrangement.
func TestsJSON(d Documents) (string, error) {
	if d.Tests == nil {
		return "", errors.New("任务缺少测试选择数组")
	}
	raw, err := json.Marshal(struct {
		Tests []string `json:"tests"`
	}{d.Tests})
	if err != nil {
		return "", err
	}
	seen := map[string]bool{}
	for _, id := range d.Tests {
		if id == "" || strings.TrimSpace(id) != id || !utf8.ValidString(id) || seen[id] {
			return "", errors.New("测试ID为空或重复")
		}
		seen[id] = true
	}
	return string(raw) + "\n", nil
}

func validateDocumentContent(d Documents, paths map[string]bool, total *int) error {
	check := func(f Document, prefix string) error {
		*total += len(f.Body)
		if !strings.HasPrefix(f.Path, prefix) || !strings.HasSuffix(f.Path, ".md") || path.Clean(f.Path) != f.Path || strings.ContainsAny(f.Path, "\\:\x00") || strings.TrimSpace(f.Body) == "" || !utf8.ValidString(f.Body) || *total > MaxBytes || paths[f.Path] {
			return errors.New("文档路径、编码、正文或大小无效")
		}
		paths[f.Path] = true
		return nil
	}
	if err := check(d.Spec, "SteerlineFlow/spec/"); err != nil {
		return err
	}
	if err := check(d.Plan, "SteerlineFlow/plans/"); err != nil {
		return err
	}
	raw, err := TestsJSON(d)
	if err != nil {
		return err
	}
	*total += len(raw)
	if len(d.ManualChecks) != 0 {
		name, err := manualverification.CompanionPath(d.Plan.Path)
		if err != nil {
			return err
		}
		body, err := manualverification.Encode(d.ManualChecks)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if paths[name] {
			return errors.New("重复的人工核验文件：" + name)
		}
		paths[name] = true
		*total += len(body)
	}
	if len(d.Acceptance) == 0 {
		return errors.New("任务缺少专属验收材料")
	}
	for _, f := range d.Acceptance {
		if err := check(f, "SteerlineFlow/testing/acceptance/"); err != nil {
			return err
		}
	}
	if *total > MaxBytes {
		return errors.New("完整文档与测试选择超过大小上限")
	}
	return nil
}
func ValidateDocument(d Documents) error {
	total := 0
	return validateDocumentContent(d, map[string]bool{}, &total)
}

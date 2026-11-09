package model

import (
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

var commitPattern = regexp.MustCompile("^(?:[0-9a-f]{40}|[0-9a-f]{64})$")

func ValidateDocuments(r Record) error {
	if !r.DocumentsVerified || r.Analysis == nil || r.ConfirmedDigest != ValueDigest(r.Analysis) {
		return errors.New("未取得与已确认规划绑定、经核实的 Runner 文档结果")
	}
	if !commitPattern.MatchString(r.CodeCommit) {
		return errors.New("文档未固定完整代码提交")
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
	check := func(d Document, prefix string) error {
		total += len(d.Body)
		if !strings.HasPrefix(d.Path, prefix) || !strings.HasSuffix(d.Path, ".md") || path.Clean(d.Path) != d.Path || strings.ContainsAny(d.Path, "\\:\x00") || strings.TrimSpace(d.Body) == "" || !utf8.ValidString(d.Body) || total > MaxBytes {
			return errors.New("文档路径、编码、正文或大小无效")
		}
		if paths[d.Path] {
			return errors.New("文档路径重复")
		}
		paths[d.Path] = true
		return nil
	}
	for _, d := range r.Documents {
		if !expected[d.Key] || seen[d.Key] {
			return errors.New("文档与本批内部代号不匹配")
		}
		seen[d.Key] = true
		if e := check(d.Spec, "SteerlineFlow/spec/"); e != nil {
			return e
		}
		if e := check(d.Plan, "SteerlineFlow/plans/"); e != nil {
			return e
		}
		rawTests, err := TestsJSON(d)
		if err != nil {
			return err
		}
		total += len(rawTests)
		if total > MaxBytes {
			return errors.New("完整文档与测试选择超过大小上限")
		}

		if len(d.Acceptance) == 0 {
			return errors.New("任务缺少专属验收材料")
		}
		for _, a := range d.Acceptance {
			if e := check(a, "SteerlineFlow/testing/acceptance/"); e != nil {
				return e
			}
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
		if id == "" || seen[id] {
			return "", errors.New("测试ID为空或重复")
		}
		seen[id] = true
	}
	return string(raw) + "\n", nil
}

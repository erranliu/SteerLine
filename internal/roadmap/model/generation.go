package model

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type DocumentContext struct {
	Commit, Tree string
	Materials    []runnercontract.Document
}
type Investigation struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Reason string `json:"reason"`
}
type GeneratedDocuments struct {
	SourceDigest    string          `json:"source_digest"`
	ConfirmedDigest string          `json:"confirmed_digest"`
	CodeCommit      string          `json:"code_commit"`
	Design          string          `json:"design"`
	Investigation   []Investigation `json:"investigation"`
	Questions       []string        `json:"questions"`
	Documents       []Documents     `json:"documents"`
}

func PlannedDocuments(r Record) []Documents {
	out := []Documents{}
	for _, n := range r.Analysis.Tasks {
		if n.ExistingID != "" {
			continue
		}
		var slug strings.Builder
		for _, c := range n.Title {
			if unicode.IsLetter(c) || unicode.IsDigit(c) {
				slug.WriteRune(c)
			} else {
				slug.WriteByte('-')
			}
			if slug.Len() > 80 {
				break
			}
		}
		name := strings.Trim(slug.String(), "-")
		if name == "" {
			name = "task"
		}
		name = r.Source.ID[:12] + "/" + name + "-" + Digest([]byte(n.Key))[:8]
		out = append(out, Documents{Key: n.Key,
			Spec:       Document{Path: "SteerlineFlow/spec/roadmaps/" + name + ".spec.md"},
			Plan:       Document{Path: "SteerlineFlow/plans/roadmaps/" + name + ".plan.md"},
			Acceptance: []Document{{Path: "SteerlineFlow/testing/acceptance/roadmaps/" + name + ".md"}}})
	}
	return out
}

func DocumentSchema(r Record) json.RawMessage {
	str := map[string]any{"type": "string"}
	object := func(properties map[string]any) map[string]any {
		required := []string{}
		for key := range properties {
			required = append(required, key)
		}
		sort.Strings(required)
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	file := func(p string) any {
		return object(map[string]any{"path": map[string]any{"const": p}, "body": map[string]any{"type": "string", "minLength": 1}})
	}
	entries := []any{}
	for _, d := range PlannedDocuments(r) {
		entries = append(entries, object(map[string]any{
			"local_key": map[string]any{"const": d.Key}, "spec": file(d.Spec.Path), "plan": file(d.Plan.Path),
			"acceptance": map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "prefixItems": []any{file(d.Acceptance[0].Path)}}}))
	}
	props := map[string]any{
		"source_digest":    map[string]any{"const": r.Source.Digest},
		"confirmed_digest": map[string]any{"const": r.ConfirmedDigest},
		"code_commit":      map[string]any{"const": r.CodeCommit},
		"design":           map[string]any{"type": "string", "minLength": 1},
		"questions":        map[string]any{"type": "array", "items": str},
		"investigation":    map[string]any{"type": "array", "minItems": 1, "items": object(map[string]any{"path": str, "sha256": str, "reason": str})},
		"documents":        map[string]any{"type": "array", "minItems": len(entries), "maxItems": len(entries), "prefixItems": entries},
	}
	schema := object(props)
	// A blocked result must explain the problem and contains no adoptable documents.
	blocked := object(map[string]any{
		"source_digest": props["source_digest"], "confirmed_digest": props["confirmed_digest"], "code_commit": props["code_commit"],
		"design": props["design"], "investigation": props["investigation"],
		"questions": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "minLength": 1}},
		"documents": map[string]any{"type": "array", "maxItems": 0},
	})
	raw, _ := json.Marshal(map[string]any{"oneOf": []any{schema, blocked}})
	return raw
}

func DecodeDocuments(r Record, raw []byte, source *runnercontract.RepositorySource) (Record, error) {
	if len(raw) > runnercontract.MaxGenerateResultBytes {
		return r, errors.New("Runner 文档 JSON 超过 1 MiB")
	}
	canonical, err := runnercontract.CanonicalJSON(raw)
	if err != nil {
		return r, err
	}
	var out GeneratedDocuments
	d := json.NewDecoder(bytes.NewReader(canonical))
	d.DisallowUnknownFields()
	if err = d.Decode(&out); err != nil {
		return r, err
	}
	if out.SourceDigest != r.Source.Digest || out.ConfirmedDigest != r.ConfirmedDigest || out.CodeCommit != r.CodeCommit {
		return r, errors.New("文档与本次确认/来源/完整提交不匹配")
	}
	if source == nil || source.Commit != r.CodeCommit || source.Tree != r.RepositoryTree || len(source.Files) == 0 || len(source.Files) > 4096 {
		return r, errors.New("缺少固定代码版本的 Runner 来源")
	}
	files := map[string]runnercontract.RepositoryFile{}
	for _, f := range source.Files {
		if f.Path == "" || f.Path == "." || f.Path == ".." || path.Clean(f.Path) != f.Path || strings.HasPrefix(f.Path, "/") || strings.HasPrefix(f.Path, "../") || strings.ContainsAny(f.Path, "\\:\x00") || len(f.SHA256) != 64 || f.SizeBytes < 0 || len(f.Blob) != len(r.CodeCommit) {
			return r, errors.New("Runner 来源文件 manifest 无效")
		}
		if _, err := hex.DecodeString(f.SHA256); err != nil {
			return r, errors.New("无效文件摘要")
		}
		if _, err := hex.DecodeString(f.Blob); err != nil {
			return r, errors.New("无效 Git blob")
		}
		key := strings.ToLower(f.Path)
		if _, ok := files[key]; ok {
			return r, errors.New("Runner 来源路径重复")
		}
		files[key] = f
	}
	if strings.TrimSpace(out.Design) == "" || len(out.Investigation) == 0 {
		return r, errors.New("缺少整体设计与代码调查依据")
	}
	for _, guideline := range r.Materials {
		f, ok := files[strings.ToLower(guideline.Path)]
		if !ok || f.Path != guideline.Path || guideline.Content == nil || f.SHA256 != Digest([]byte(*guideline.Content)) || f.SizeBytes != len(*guideline.Content) {
			return r, fmt.Errorf("生成规范不属于固定版本：%s", guideline.Path)
		}
	}
	for _, ref := range out.Investigation {
		f, ok := files[strings.ToLower(ref.Path)]
		if !ok || f.Path != ref.Path || ref.SHA256 != f.SHA256 || strings.TrimSpace(ref.Reason) == "" {
			return r, fmt.Errorf("调查引用不属于固定版本：%s", ref.Path)
		}
	}
	r.Design, r.Investigation, r.Questions = out.Design, out.Investigation, out.Questions
	if len(out.Questions) > 0 {
		for _, q := range out.Questions {
			if strings.TrimSpace(q) == "" {
				return r, errors.New("待澄清问题不得为空")
			}
		}
		if len(out.Documents) > 0 {
			return r, errors.New("存在问题时不得采用生成文档")
		}
		r.State = "documents-blocked"
		return r, nil
	}
	want := PlannedDocuments(r)
	if len(want) != len(out.Documents) {
		return r, errors.New("文档集不完整")
	}
	for i, d := range out.Documents {
		w := want[i]
		if d.Key != w.Key || d.Spec.Path != w.Spec.Path || d.Plan.Path != w.Plan.Path || len(d.Acceptance) != 1 || d.Acceptance[0].Path != w.Acceptance[0].Path {
			return r, errors.New("文档路径或内部身份与固定 Schema 不符")
		}
	}
	r.Documents, r.DocumentsVerified = out.Documents, true
	if err = ValidateDocuments(r); err != nil {
		return r, err
	}
	r.State = "documents-review"
	return r, nil
}

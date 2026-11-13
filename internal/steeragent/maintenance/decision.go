package maintenance

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/erranliu/SteerLine/internal/prompts"
)

type Decision struct {
	Kind     string          `json:"kind"`
	Prompt   string          `json:"prompt,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Question json.RawMessage `json:"question,omitempty"`
	Report   string          `json:"report,omitempty"`
}

func Instruction() (string, error) { return prompts.MaintenanceDecision() }

func Accept(raw []byte, mode string) (Decision, error) {
	var d Decision
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 1<<20 || !json.Valid(raw) || decoder.Decode(&d) != nil {
		return d, errors.New("维护决策不是有效JSON")
	}
	if mode == "report" {
		if d.Question == nil && (d.Kind == "report" && present(d.Report) ||
			d.Kind == "needs_evidence" && present(d.Summary) && d.Report == "" && d.Prompt == "") {
			return d, nil
		}
		return d, errors.New("无效的维护报告结果")
	}
	if !present(d.Summary) || d.Report != "" || d.Kind != "ask" && d.Question != nil || d.Kind != "execute" && d.Prompt != "" {
		return d, errors.New("维护决定字段与用途不匹配")
	}
	if !slices.Contains(allowedKinds(mode), d.Kind) {
		switch d.Kind {
		case "accept":
			return d, errors.New("只有独立审计可以接受成果")
		case "finish":
			return d, errors.New("当前用途不接受收尾建议")
		default:
			return d, errors.New("未知维护决定")
		}
	}
	switch d.Kind {
	case "execute":
		if !present(d.Prompt) {
			return d, errors.New("执行决定缺少提示词")
		}
	case "ask":
		if len(d.Question) == 0 || bytes.Equal(d.Question, []byte("null")) {
			return d, errors.New("人工决定缺少有效问题、选项或推荐")
		}
	}
	return d, nil
}

func present(text string) bool { return strings.TrimSpace(text) != "" }

func allowedKinds(mode string) []string {
	switch mode {
	case "first", "select":
		return []string{"execute", "ask", "finish", "blocked"}
	case "audit":
		return []string{"execute", "ask", "accept", "blocked"}
	case "version_audit":
		return []string{"execute", "ask", "accept", "finish", "blocked"}
	default:
		return []string{"execute", "ask", "blocked"}
	}
}

func Schema(mode string) json.RawMessage {
	kinds, _ := json.Marshal(allowedKinds(mode))
	return json.RawMessage(`{"type":"object","additionalProperties":false,"required":["kind","summary"],"properties":{"kind":{"enum":` + string(kinds) + `},"prompt":{"type":"string"},"summary":{"type":"string"},"question":{"type":"object","additionalProperties":false,"required":["prompt","facts","attempts","options","recommended_option_id","recommendation_reason"],"properties":{"prompt":{"type":"string"},"facts":{"type":"array","items":{"type":"string"}},"attempts":{"type":"array","items":{"type":"string"}},"options":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["id","label","impact"],"properties":{"id":{"type":"string"},"label":{"type":"string"},"impact":{"type":"string"}}}},"recommended_option_id":{"type":"string"},"recommendation_reason":{"type":"string"}}}}}`)
}

func ReportSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"required":["kind"],"properties":{"kind":{"enum":["report","needs_evidence"]},"report":{"type":"string"},"prompt":{"type":"string"},"summary":{"type":"string"}}}`)
}

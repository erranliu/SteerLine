// Package maintenance prepares and validates read-only steering decisions.
// It has no project writer, Runner, persistence or scheduling capability.
package maintenance

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/erranliu/SteerLine/internal/prompts"
)

type Decision struct {
	Kind     string `json:"kind"`
	Prompt   string `json:"prompt"`
	Summary  string `json:"summary"`
	Question string `json:"question"`
}

func Instruction() (string, error) { return prompts.MaintenanceDecision() }
func Rules() ([]byte, error)       { return prompts.MaintenanceRules() }
func Accept(raw []byte, mode string) (Decision, error) {
	var d Decision
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 1<<20 || !json.Valid(raw) || decoder.Decode(&d) != nil {
		return d, errors.New("维护决策不是有效JSON")
	}
	switch d.Kind {
	case "execute":
		if strings.TrimSpace(d.Prompt) == "" {
			return d, errors.New("执行决定缺少提示词")
		}
	case "ask":
		if strings.TrimSpace(d.Question) == "" {
			return d, errors.New("人工决定缺少问题")
		}
	case "accept":
		if mode != "audit" {
			return d, errors.New("只有独立审计可以接受成果")
		}
	case "finish", "blocked":
		if d.Kind == "finish" && mode == "finish" {
			return d, errors.New("收尾决策不能重复请求收尾")
		}
	default:
		return d, errors.New("未知维护决定")
	}
	return d, nil
}

func Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"required":["kind","prompt","summary","question"],"properties":{"kind":{"enum":["execute","ask","accept","finish","blocked"]},"prompt":{"type":"string"},"summary":{"type":"string"},"question":{"type":"string"}}}`)
}

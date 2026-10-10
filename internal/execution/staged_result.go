package execution

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
)

type stageBlocker struct {
	Category string `json:"category"`
	Action   string `json:"action"`
	Error    string `json:"error"`
	Known    string `json:"known,omitempty"`
	Unknown  string `json:"unknown,omitempty"`
}

type stageOutputs struct {
	CandidateSHA string `json:"candidate_sha,omitempty"`
	PRURL        string `json:"pr_url,omitempty"`
	Branch       string `json:"branch,omitempty"`
}

type stageReport struct {
	Outcome  string        `json:"outcome"`
	Outputs  stageOutputs  `json:"outputs,omitempty"`
	Summary  string        `json:"summary"`
	Question string        `json:"question,omitempty"`
	Blocker  *stageBlocker `json:"blocker,omitempty"`
}

const stageResultContract = `Return exactly one JSON object, with no Markdown or surrounding text:
{"outcome":"done|needs_input|blocked","outputs":{"candidate_sha":"optional full SHA","pr_url":"optional actual PR URL","branch":"optional known branch"},"summary":"actual result","question":"required only for needs_input","blocker":{"category":"failure|unknown_external|permission|prerequisite","action":"actual action","error":"actual error","known":"optional known result","unknown":"optional unresolved result"}}
Omit question/blocker when not applicable. done requires candidate_sha for implement.report, pr_url for publish and publish.report. Never fill an invented SHA or URL. A failed original execution cannot be done. Unknown external outcomes must use blocker.category=unknown_external.`

var stageSHA = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Reject duplicate keys at every depth before decoding. encoding/json normally
// accepts last-wins objects, which is unsuitable for a first-wins result.
func uniqueJSON(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate or invalid result key")
			}
			seen[name] = true
			if err := uniqueJSON(dec); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := uniqueJSON(dec); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = dec.Token()
	return err
}

func decodeStageReport(body, step string) (stageReport, error) {
	var value stageReport
	if len(body) == 0 || len(body) > 128*1024 {
		return value, errors.New("result missing or exceeds bound")
	}
	check := json.NewDecoder(strings.NewReader(body))
	if err := uniqueJSON(check); err != nil {
		return value, err
	}
	if _, err := check.Token(); err != io.EOF {
		return value, errors.New("trailing result content")
	}
	dec := json.NewDecoder(bytes.NewBufferString(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&value); err != nil {
		return value, err
	}
	if strings.TrimSpace(value.Summary) == "" {
		return value, errors.New("result summary missing")
	}
	if value.Outputs.CandidateSHA != "" && !stageSHA.MatchString(value.Outputs.CandidateSHA) {
		return value, errors.New("invalid candidate SHA")
	}
	if value.Outputs.PRURL != "" {
		u, err := url.Parse(value.Outputs.PRURL)
		if err != nil || u.Host == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" {
			return value, errors.New("invalid PR URL")
		}
	}
	switch value.Outcome {
	case "done":
		if value.Question != "" || value.Blocker != nil || step == executionmodel.StepReport && value.Outputs.CandidateSHA == "" || executionmodel.PublicationStep(step) && value.Outputs.PRURL == "" {
			return value, errors.New("done result has inconsistent fields")
		}
	case "needs_input":
		if strings.TrimSpace(value.Question) == "" || value.Blocker != nil {
			return value, errors.New("needs_input requires its question")
		}
	case "blocked":
		if value.Question != "" || value.Blocker == nil || value.Blocker.Action == "" || value.Blocker.Error == "" {
			return value, errors.New("blocked requires action and error")
		}
		switch value.Blocker.Category {
		case "failure", "unknown_external", "permission", "prerequisite":
		default:
			return value, errors.New("unsupported blocker category")
		}
	default:
		return value, errors.New("unsupported result outcome")
	}
	return value, nil
}

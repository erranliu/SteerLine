package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
)

type Blocker struct {
	Category string `json:"category"`
	Action   string `json:"action"`
	Error    string `json:"error"`
	Known    string `json:"known,omitempty"`
	Unknown  string `json:"unknown,omitempty"`
}

type Outputs struct {
	PRURL  string `json:"pr_url,omitempty"`
	Branch string `json:"branch,omitempty"`
}

type Report struct {
	Outcome  string   `json:"outcome"`
	Outputs  Outputs  `json:"outputs,omitempty"`
	Summary  string   `json:"summary"`
	Question string   `json:"question,omitempty"`
	Blocker  *Blocker `json:"blocker,omitempty"`
}

const ResultContract = `Return one JSON object:
{"outcome":"done|needs_input|blocked","summary":"result"}
For needs_input add question. For blocked add blocker: {"category":"failure|unknown_external|permission|prerequisite","action":"action","error":"error","known":"optional","unknown":"optional"}.`

func ResultInstruction(step string) string {
	if step == StepReport {
		return ResultContract
	}
	return ResultContract + "\nFor done add outputs: {\"pr_url\":\"actual PR URL\"}."
}

// Reject duplicate keys at every depth before decoding. encoding/json normally
// accepts last-wins objects and case-insensitive struct fields, which is
// unsuitable for a first-wins result.
type resultFields map[string]resultFields

var reportFields = resultFields{
	"outcome": nil, "summary": nil, "question": nil,
	"outputs": {"pr_url": nil, "branch": nil},
	"blocker": {"category": nil, "action": nil, "error": nil, "known": nil, "unknown": nil},
}

func uniqueJSON(dec *json.Decoder, fields resultFields) error {
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
			// Match the struct decoder's aliases only for fields consumed here.
			// Distinct keys in ignored metadata remain valid JSON.
			var child resultFields
			if nested, known := fields[strings.ToLower(name)]; known {
				name, child = strings.ToLower(name), nested
			}
			if !ok || seen[name] {
				return errors.New("duplicate or invalid result key")
			}
			seen[name] = true
			if err := uniqueJSON(dec, child); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := uniqueJSON(dec, nil); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = dec.Token()
	return err
}

func DecodeReport(body, step string) (Report, error) {
	var value Report
	if len(body) == 0 || len(body) > 128*1024 {
		return value, errors.New("result missing or exceeds bound")
	}
	// A complete code fence is presentation. Do not extract JSON from prose
	// or select one of several objects, which could change the intended outcome.
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	for _, prefix := range []string{"```json\n", "```\n"} {
		if strings.HasPrefix(body, prefix) && strings.HasSuffix(body, "\n```") {
			body = strings.TrimSuffix(strings.TrimPrefix(body, prefix), "\n```")
			break
		}
	}
	check := json.NewDecoder(strings.NewReader(body))
	if err := uniqueJSON(check, reportFields); err != nil {
		return value, err
	}
	if _, err := check.Token(); err != io.EOF {
		return value, errors.New("trailing result content")
	}
	dec := json.NewDecoder(bytes.NewBufferString(body))
	// Only the known business fields drive the flow. Extra explanatory fields
	// do not make an otherwise unambiguous result unusable.
	if err := dec.Decode(&value); err != nil {
		return value, err
	}
	value.Outcome = strings.TrimSpace(value.Outcome)
	value.Summary = strings.TrimSpace(value.Summary)
	value.Question = strings.TrimSpace(value.Question)
	value.Outputs.PRURL = strings.TrimSpace(value.Outputs.PRURL)
	if strings.TrimSpace(value.Summary) == "" {
		return value, errors.New("result summary missing")
	}
	if value.Outputs.PRURL != "" {
		u, err := url.Parse(value.Outputs.PRURL)
		if err != nil || u.Host == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" {
			return value, errors.New("invalid PR URL")
		}
	}
	switch value.Outcome {
	case "done":
		if value.Question != "" || value.Blocker != nil || PublicationStep(step) && value.Outputs.PRURL == "" {
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

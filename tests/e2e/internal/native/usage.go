// Package native adapts real native tooling for isolated E2E environments.
// It never manufactures model responses, identities or usage.
package native

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const Model = "gpt-6-luna"
const Effort = "medium"

func TrustProject(home, path string) error {
	quoted, _ := json.Marshal(filepath.Clean(path))
	f, err := os.OpenFile(filepath.Join(home, "config.toml"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(f, "\n[projects.%s]\ntrust_level = \"trusted\"\n", quoted)
	return errors.Join(writeErr, f.Close())
}

type Tokens struct {
	Input     int64 `json:"input_tokens"`
	Cached    int64 `json:"cached_input_tokens"`
	Output    int64 `json:"output_tokens"`
	Reasoning int64 `json:"reasoning_output_tokens"`
	Total     int64 `json:"total_tokens"`
}

func (t Tokens) Validate() error {
	if t.Input <= 0 || t.Output <= 0 || t.Total != t.Input+t.Output || t.Cached < 0 || t.Cached > t.Input || t.Reasoning < 0 || t.Reasoning > t.Output {
		return fmt.Errorf("invalid or zero native token usage: %+v", t)
	}
	return nil
}

type Usage struct {
	Source  string `json:"source"`
	Session string `json:"session_id"`
	Turn    string `json:"turn_id,omitempty"`
	Cwd     string `json:"cwd,omitempty"`
	Model   string `json:"model"`
	Effort  string `json:"reasoning_effort"`
	Tokens  Tokens `json:"tokens"`
}

// AppServerUsage takes the last cumulative thread total for the bound turn.
// Repeated incremental notifications are not independently billed samples.
func AppServerUsage(raw []byte, session, turn string) (Usage, []byte, error) {
	u := Usage{Source: "app-server/thread/tokenUsage/updated", Session: session, Turn: turn, Model: Model, Effort: Effort}
	var evidence bytes.Buffer
	found := false
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for scanner.Scan() {
		var e struct {
			Method string `json:"method"`
			Params struct {
				Thread string `json:"threadId"`
				Turn   string `json:"turnId"`
				Usage  struct {
					Total struct {
						Input     int64 `json:"inputTokens"`
						Cached    int64 `json:"cachedInputTokens"`
						Output    int64 `json:"outputTokens"`
						Reasoning int64 `json:"reasoningOutputTokens"`
						Total     int64 `json:"totalTokens"`
					} `json:"total"`
				} `json:"tokenUsage"`
			} `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return u, nil, err
		}
		if e.Method != "thread/tokenUsage/updated" {
			continue
		}
		if e.Params.Thread != session || e.Params.Turn != turn {
			return u, nil, errors.New("usage notification differs from bound native session/turn")
		}
		n := e.Params.Usage.Total
		if found && n.Total < u.Tokens.Total {
			return u, nil, errors.New("native cumulative usage decreased")
		}
		u.Tokens = Tokens{n.Input, n.Cached, n.Output, n.Reasoning, n.Total}
		found = true
		evidence.Write(scanner.Bytes())
		evidence.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return u, nil, err
	}
	if !found || session == "" || turn == "" {
		return u, nil, errors.New("bound native token usage missing")
	}
	return u, evidence.Bytes(), u.Tokens.Validate()
}

// RolloutUsage reads only native JSONL files in this project's isolated home.
// session_meta/cwd and turn_context bind the selected session and model.
func RolloutUsage(raw []byte, cwd string) (Usage, []byte, error) {
	u := Usage{Source: "codex/session-jsonl", Cwd: cwd}
	var evidence bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 16<<20)
	matched, found, contextFound := false, false, false
	for scanner.Scan() {
		var e struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return u, nil, err
		}
		relevant := false
		switch e.Type {
		case "session_meta":
			var meta struct {
				ID  string `json:"id"`
				Cwd string `json:"cwd"`
			}
			if err := json.Unmarshal(e.Payload, &meta); err != nil {
				return u, nil, err
			}
			matched = samePath(meta.Cwd, cwd)
			u.Session = meta.ID
			relevant = true
		case "turn_context":
			var c struct {
				Model           string `json:"model"`
				Effort          string `json:"effort"`
				ReasoningEffort string `json:"reasoning_effort"`
			}
			if err := json.Unmarshal(e.Payload, &c); err != nil {
				return u, nil, err
			}
			if c.Effort == "" {
				c.Effort = c.ReasoningEffort
			}
			if c.Model != Model || c.Effort != Effort {
				return u, nil, fmt.Errorf("native turn used %s/%s; require %s/%s", c.Model, c.Effort, Model, Effort)
			}
			u.Model, u.Effort = c.Model, c.Effort
			contextFound = true
			relevant = true
		case "event_msg":
			var p struct {
				Type string `json:"type"`
				Info *struct {
					Total Tokens `json:"total_token_usage"`
				} `json:"info"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return u, nil, err
			}
			if p.Type != "token_count" || p.Info == nil {
				continue
			}
			if found && p.Info.Total.Total < u.Tokens.Total {
				return u, nil, errors.New("native cumulative rollout usage decreased")
			}
			u.Tokens = p.Info.Total
			found = true
			relevant = true
		}
		if relevant {
			evidence.Write(scanner.Bytes())
			evidence.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return u, nil, err
	}
	if !matched || u.Session == "" || !contextFound || !found {
		return u, nil, errors.New("native session/cwd/model/usage evidence missing")
	}
	return u, evidence.Bytes(), u.Tokens.Validate()
}
func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(filepath.FromSlash(a)), filepath.Clean(filepath.FromSlash(b)))
}

func SeedAuth(source, destination, windowsSandbox string) error {
	var sandbox map[string][]byte
	switch windowsSandbox {
	case "elevated":
		var err error
		sandbox, err = sandboxState(source)
		if err != nil {
			return fmt.Errorf("noninteractive sandbox preflight failed before model request: %w", err)
		}
	case "unelevated", "full-access":
		// Restricted tokens use the current user, not provisioned accounts.
	default:
		return errors.New("unknown native Windows sandbox mode")
	}
	raw, err := os.ReadFile(filepath.Join(source, "auth.json"))
	if err != nil {
		return fmt.Errorf("live tests require readable Codex auth.json: %w", err)
	}
	var auth map[string]json.RawMessage
	if err := json.Unmarshal(raw, &auth); err != nil || len(auth) == 0 {
		return errors.New("invalid Codex authentication file")
	}
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(destination, "auth.json"), raw, 0600); err != nil {
		return err
	}
	// Reuse the already provisioned Windows sandbox accounts. A fresh home
	// without these files asks for interactive UAC during the first shell call.
	// These are credentials, never artifacts; the native runner still applies
	// and proves the frozen workspace permissions itself.
	for relative, data := range sandbox {
		target := filepath.Join(destination, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			return err
		}
	}
	// No user configuration, session history, provider override or MCP is inherited.
	return os.WriteFile(filepath.Join(destination, "config.toml"), []byte("model = \""+Model+"\"\nmodel_reasoning_effort = \""+Effort+"\"\n"), 0600)
}

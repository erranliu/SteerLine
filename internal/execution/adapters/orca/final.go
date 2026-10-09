package orcaworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// worker-read's text projection drops Codex's message phase. Its exact
// transcript source IDs carry original file:byte-offset identities. Read only
// that source, verify root session/cwd and the injected Task header, and consume
// the provider's final_answer + task_complete facts. Never use worker_done's
// summary, a terminal tail, commentary or tool stdout as the business final.
func (l *ExecutionLifecycle) readNativeFinal(ctx context.Context, snapshot executionbackend.LifecycleSnapshot, turn *NativeTurnState, state *NativeExecutionState) error {
	path := state.ProviderTranscriptPath
	output, _, readErr := ReadWorkerOutput(ctx, l.client.MetaCall, turn.DispatchID, "", state.Coordinator.RuntimeID)
	if readErr == nil {
		if output.Source == "transcript" && output.SourceExact && output.Provider == "codex" && output.SourceIdentity != "" {
			if err := pinNativeTranscript(output, state, snapshot.Ref.WorkspacePath); err != nil {
				return err
			}
			path = state.ProviderTranscriptPath
		} else if path == "" {
			return errors.New("native final has no exact or original pinned transcript source")
		}
	} else if path == "" {
		return readErr
	}
	if path == "" {
		return errors.New("original exact provider transcript is not pinned")
	}
	root, err := transcriptRootSession(path, snapshot.Ref.WorkspacePath)
	if err != nil || root != state.ProviderSessionID {
		return errors.New("pinned original provider session changed or is unavailable")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || stat.Size() > 128<<20 {
		return errors.New("native transcript unavailable or exceeds read bound")
	}
	raw, err := io.ReadAll(io.LimitReader(file, stat.Size()+1))
	if err != nil {
		return err
	}
	session, nativeTurn, final, item, err := codexTranscriptFinal(raw, snapshot.Ref.WorkspacePath, turn.TaskID)
	if err != nil {
		return err
	}
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(stat, current) || current.Size() != stat.Size() || current.ModTime() != stat.ModTime() {
		return errors.New("native transcript changed while verifying final")
	}
	if state.ProviderSessionID != "" && state.ProviderSessionID != session {
		return errors.New("original Codex provider session changed")
	}
	state.ProviderSessionID = session
	turn.FinalSource, turn.FinalItemID, turn.FinalNativeTurnID = "orca.codex.transcript.final_answer", path+":"+item, nativeTurn
	turn.FinalVerified, turn.FinalText = true, final
	return nil
}

func exactTranscriptPath(output WorkerOutput) (string, error) {
	if output.Source != "transcript" || !output.SourceExact || output.Provider != "codex" || output.SourceIdentity == "" {
		return "", errors.New("native final has no exact Codex transcript source")
	}
	var data struct {
		Messages []transcriptMessage `json:"messages"`
	}
	if err := json.Unmarshal(output.Transcript, &data); err != nil {
		return "", err
	}
	var path string
	for _, message := range data.Messages {
		if message.Source != "transcript" {
			continue
		}
		colon := strings.LastIndexByte(message.ID, ':')
		if colon < 1 {
			continue
		}
		if _, err := strconv.ParseInt(message.ID[colon+1:], 10, 64); err != nil {
			continue
		}
		candidate := message.ID[:colon]
		if !filepath.IsAbs(candidate) || filepath.Ext(candidate) != ".jsonl" {
			continue
		}
		if path != "" && path != candidate {
			return "", errors.New("native output changed original transcript file")
		}
		path = candidate
	}
	if path == "" {
		return "", errors.New("native transcript file identity unavailable")
	}
	return path, nil
}

func transcriptRootSession(path, cwd string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 64*1024)
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return "", err
	}
	var event struct {
		Type    string `json:"type"`
		Payload struct {
			ID     string          `json:"id"`
			Cwd    string          `json:"cwd"`
			Parent string          `json:"parent_thread_id"`
			Source json.RawMessage `json:"source"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return "", err
	}
	p := event.Payload
	var origin struct {
		Subagent json.RawMessage `json:"subagent"`
	}
	_ = json.Unmarshal(p.Source, &origin)
	if event.Type != "session_meta" || p.ID == "" || p.Parent != "" || !strings.EqualFold(filepath.Clean(p.Cwd), filepath.Clean(cwd)) || len(origin.Subagent) > 0 && string(origin.Subagent) != "null" {
		return "", errors.New("transcript is not the original root session/workspace")
	}
	return p.ID, nil
}

func pinNativeTranscript(output WorkerOutput, state *NativeExecutionState, cwd string) error {
	path, err := exactTranscriptPath(output)
	if err != nil {
		return err
	}
	session, err := transcriptRootSession(path, cwd)
	if err != nil {
		return err
	}
	if state.ProviderTranscriptPath != "" && state.ProviderTranscriptPath != path || state.ProviderSessionID != "" && state.ProviderSessionID != session {
		return errors.New("original native provider transcript binding changed")
	}
	state.ProviderTranscriptPath, state.ProviderSessionID = path, session
	return nil
}

func codexTranscriptFinal(raw []byte, cwd, task string) (session, turn, final, item string, err error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	bound, stopped, root := false, false, false
	offset := int64(0)
	for scanner.Scan() {
		line := scanner.Bytes()
		current := offset
		offset += int64(len(line)) + 1
		var e struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(line, &e); err != nil {
			return "", "", "", "", err
		}
		var p struct {
			ID      string          `json:"id"`
			Cwd     string          `json:"cwd"`
			Parent  string          `json:"parent_thread_id"`
			Source  json.RawMessage `json:"source"`
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Phase   string          `json:"phase"`
			Turn    string          `json:"turn_id"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return "", "", "", "", err
		}
		switch e.Type {
		case "session_meta":
			root = p.ID != "" && p.Parent == "" && strings.EqualFold(filepath.Clean(p.Cwd), filepath.Clean(cwd))
			session = p.ID
			var origin struct {
				Subagent json.RawMessage `json:"subagent"`
			}
			if json.Unmarshal(p.Source, &origin) == nil && len(origin.Subagent) > 0 && string(origin.Subagent) != "null" {
				root = false
			}
		case "event_msg":
			if p.Type == "task_started" {
				turn, stopped = p.Turn, false
			}
			if p.Type == "task_complete" && p.Turn == turn && turn != "" {
				stopped = true
			}
		case "turn_context":
			if p.Turn != "" {
				turn = p.Turn
			}
		case "response_item":
			if p.Type != "message" {
				continue
			}
			var text strings.Builder
			for _, content := range p.Content {
				if content.Type == "input_text" || content.Type == "output_text" {
					text.WriteString(content.Text)
				}
			}
			if p.Role == "user" {
				bound = false
				for _, line := range strings.Split(text.String(), "\n") {
					if _, value, ok := strings.Cut(line, "Your task ID is: "); ok {
						fields := strings.Fields(value)
						if len(fields) > 0 && fields[0] == task {
							bound = true
						}
					}
				}
				final, item, stopped = "", "", false
			}
			if bound && p.Role == "assistant" && p.Phase == "final_answer" {
				final, item = text.String(), strconv.FormatInt(current, 10)
			}
		}
	}
	if scanner.Err() != nil {
		err = scanner.Err()
		return
	}
	if !root || !bound || !stopped || final == "" || item == "" {
		err = errors.New("original task has no stopped native final_answer with root session binding")
	}
	return
}

func finalEvidence(turn NativeTurnState, session string) executionbackend.TurnEvidence {
	return executionbackend.TurnEvidence{FinalSource: turn.FinalSource, FinalItemID: turn.FinalItemID, FinalVerified: turn.FinalVerified, FinalNativeSessionID: session, FinalNativeTurnID: turn.FinalNativeTurnID}
}

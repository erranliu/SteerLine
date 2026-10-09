package native

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	testprocess "github.com/erranliu/SteerLine/internal/testkit/process"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	orcaworker "github.com/erranliu/SteerLine/internal/execution/adapters/orca"
	remote "github.com/erranliu/SteerLine/internal/execution/adapters/orcaremote"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

// RemoteManifest references an already paired Windows host. It contains no
// credentials and never changes the host's agent launcher or global settings.
type RemoteManifest struct {
	Version            int    `json:"version"`
	SourceCommit       string `json:"source_commit"`
	EnvironmentID      string `json:"environment_id"`
	ConnectionID       string `json:"connection_id"`
	RepositorySelector string `json:"repository_selector"`
	Repository         string `json:"repository"`
}

func LoadRemoteManifest(path, sourceCommit string) (RemoteManifest, error) {
	var m RemoteManifest
	raw, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("remote manifest: %w", err)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil {
		return m, err
	}
	if d.Decode(new(any)) != io.EOF {
		return m, errors.New("remote manifest has trailing JSON")
	}
	if m.Version != 1 || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(m.SourceCommit) || m.SourceCommit != sourceCommit || m.Repository != "erranliu/SteerlineTest" || m.EnvironmentID == "" || m.ConnectionID == "" || !strings.HasPrefix(m.RepositorySelector, "id:") || len(m.RepositorySelector) <= 3 {
		return m, errors.New("remote manifest requires exact source, paired target and authorized SteerlineTest repo ID")
	}
	return m, nil
}

type RemoteOrca struct {
	Manifest   RemoteManifest
	Executable string
	Pair       remote.PreflightResult
	RepoPath   string
	// Record receives only command/response data, never authentication content.
	Record func(string, any) error
}

func (r *RemoteOrca) Raw(ctx context.Context, args ...string) (orcaworker.Envelope, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.Command(r.Executable, args...)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	err := testprocess.Run(ctx, cmd)
	raw := output.Bytes()
	env, parseErr := orcaworker.ParseEnvelope(raw)
	if r.Record != nil {
		err = errors.Join(err, r.Record("orca-call", map[string]any{"args": args, "response": json.RawMessage(raw), "error": fmt.Sprint(err)}))
	}
	return env, errors.Join(err, parseErr)
}

func (r *RemoteOrca) Preflight(ctx context.Context) error {
	config := remote.Config{Repository: "test", Agent: "codex", Model: Model, ReasoningEffort: Effort, CommandTimeout: "30s", Settings: remote.Settings{EnvironmentID: r.Manifest.EnvironmentID, ConnectionID: r.Manifest.ConnectionID, RepositorySelector: r.Manifest.RepositorySelector, PollInterval: "1s"}, Origin: func(context.Context) ([]byte, error) {
		return []byte("https://github.com/erranliu/SteerlineTest.git"), nil
	}, Call: func(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cmd := exec.Command(r.Executable, args...)
		var output bytes.Buffer
		cmd.Stdout = &output
		err := testprocess.Run(ctx, cmd)
		return output.Bytes(), err
	}}
	pair, err := remote.Preflight(ctx, config)
	if err != nil {
		return err
	}
	r.Pair = pair
	env, err := r.Raw(ctx, "repo", "show", "--repo", r.Manifest.RepositorySelector, "--environment", r.Manifest.EnvironmentID, "--json")
	if err != nil {
		return err
	}
	var result struct {
		Repo struct{ ID, Path, Kind string }
	}
	if json.Unmarshal(env.Result, &result) != nil || result.Repo.ID != pair.RepositoryID || result.Repo.Kind != "git" || result.Repo.Path == "" || env.Meta.RuntimeID != pair.RemoteRuntime {
		return errors.New("remote repository path identity missing")
	}
	r.RepoPath = result.Repo.Path
	return nil
}

func remoteCommand(script string) string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(raw[i*2:], u)
	}
	return "powershell.exe -NoProfile -NonInteractive -EncodedCommand " + base64.StdEncoding.EncodeToString(raw)
}

func PSQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

// Shell creates one test-owned helper terminal. It never sends input to an
// existing agent. The marker is inside an encoded command, so echoed command
// text cannot manufacture a result. A failed close is a failed test operation.
func (r *RemoteOrca) Shell(ctx context.Context, worktreeID, nonce, script string) (json.RawMessage, error) {
	marker := "SL_REMOTE_" + nonce + ":"
	wrapped := `$ErrorActionPreference='Stop'; try { $result = & { ` + script + ` }; $receipt=@{ok=$true;value=$result} } catch { $receipt=@{ok=$false;error=$_.Exception.Message} }; $json=$receipt|ConvertTo-Json -Compress -Depth 12; Write-Output (` + PSQuote(marker) + `+[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($json)))`
	env, err := r.Raw(ctx, "terminal", "create", "--worktree", "id:"+worktreeID, "--shell", "powershell.exe", "--title", "SteerLine E2E helper "+nonce, "--command", remoteCommand(wrapped), "--environment", r.Manifest.EnvironmentID, "--json")
	var created struct {
		Terminal struct{ Handle, IncarnationID, WorktreeID string }
	}
	if json.Unmarshal(env.Result, &created) != nil || created.Terminal.Handle == "" || created.Terminal.IncarnationID == "" || !strings.EqualFold(strings.ReplaceAll(created.Terminal.WorktreeID, "\\", "/"), strings.ReplaceAll(worktreeID, "\\", "/")) || env.Meta.RuntimeID != r.Pair.RemoteRuntime {
		return nil, errors.Join(err, errors.New("remote helper creation identity unproven; inspect original receipt"))
	}
	if err != nil {
		return nil, err
	}
	handle := created.Terminal.Handle
	var value json.RawMessage
	var readErr error
	waitCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for waitCtx.Err() == nil {
		read, callErr := r.Raw(waitCtx, "terminal", "read", "--terminal", handle, "--limit", "1000", "--environment", r.Manifest.EnvironmentID, "--json")
		if callErr != nil {
			readErr = callErr
			break
		}
		if read.Meta.RuntimeID != r.Pair.RemoteRuntime {
			readErr = errors.New("remote helper runtime changed")
			break
		}
		// Terminal read returns bounded text rows; collect only string leaves.
		var object any
		if json.Unmarshal(read.Result, &object) != nil {
			readErr = errors.New("malformed remote helper output")
			break
		}
		var leaves []string
		var visit func(any)
		visit = func(v any) {
			switch x := v.(type) {
			case string:
				leaves = append(leaves, x)
			case []any:
				for _, v := range x {
					visit(v)
				}
			case map[string]any:
				for _, v := range x {
					visit(v)
				}
			}
		}
		visit(object)
		joined := strings.Join(leaves, "\n")
		re := regexp.MustCompile(regexp.QuoteMeta(marker) + `([A-Za-z0-9+/=]+)`)
		match := re.FindStringSubmatch(joined)
		if len(match) == 2 {
			raw, decodeErr := base64.StdEncoding.DecodeString(match[1])
			var receipt struct {
				OK    bool            `json:"ok"`
				Value json.RawMessage `json:"value"`
				Error string          `json:"error"`
			}
			if decodeErr != nil || json.Unmarshal(raw, &receipt) != nil {
				readErr = errors.New("invalid remote helper marker receipt")
			} else if !receipt.OK {
				readErr = errors.New(receipt.Error)
			} else {
				value = receipt.Value
			}
			break
		}
		select {
		case <-waitCtx.Done():
		case <-time.After(250 * time.Millisecond):
		}
	}
	if value == nil && readErr == nil {
		readErr = errors.New("remote helper output deadline exceeded")
	}
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer closeCancel()
	closeEnv, closeErr := r.Raw(closeCtx, "terminal", "close", "--terminal", handle, "--environment", r.Manifest.EnvironmentID, "--json")
	var closed struct {
		Close struct {
			Handle    string
			PtyKilled bool
		}
	}
	if closeErr == nil && (json.Unmarshal(closeEnv.Result, &closed) != nil || closeEnv.Meta.RuntimeID != r.Pair.RemoteRuntime || closed.Close.Handle != handle || !closed.Close.PtyKilled) {
		closeErr = errors.New("remote helper close unconfirmed")
	}
	return value, errors.Join(readErr, closeErr)
}

func RemoteProfileConfigs(executable string, m RemoteManifest) ([]executionbackend.ImplementationConfigInput, error) {
	peer, err := remote.EncodeProfileConfig(remote.ProfileConfig{Executable: executable, Shell: "powershell.exe", WaitTimeout: "4m", Model: Model, ReasoningEffort: Effort, ConnectionID: m.ConnectionID, EnvironmentID: m.EnvironmentID, RepositorySelector: m.RepositorySelector, PollInterval: "1s", CapabilityReportVersion: 1})
	return []executionbackend.ImplementationConfigInput{{ImplementationKey: remote.ProfileCodecKey, CodecVersion: remote.ProfileCodecVersion, Payload: peer}}, err
}

// RemoteOutputEvidence uses only Orca's public, identity-pinned output pages.
// Source identity is opaque; provider files and session formats are never read.
type RemoteOutputEvidence struct {
	DispatchID, Source, SourceIdentity, Provider string
	SourceExact                                  bool
	Messages                                     int
	UsageStatus                                  string
}

func (r *RemoteOrca) Output(ctx context.Context, dispatchID, prURL string) (RemoteOutputEvidence, error) {
	evidence := RemoteOutputEvidence{DispatchID: dispatchID, UsageStatus: "not_exposed_by_orca"}
	cursor := ""
	foundPR := false
	for page := 0; page < 100; page++ {
		args := []string{"orchestration", "worker-read", "--dispatch", dispatchID, "--source", "auto", "--limit", "100", "--json"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		env, err := r.Raw(ctx, args...)
		if err != nil {
			return evidence, err
		}
		var output struct {
			DispatchID, Source, SourceIdentity, Provider string
			SourceExact                                  bool
			Transcript                                   struct {
				Messages []struct {
					Role   string
					Blocks []struct{ Type, Text string }
				}
				Limited    bool
				NextCursor string
			}
		}
		if json.Unmarshal(env.Result, &output) != nil || env.Meta.RuntimeID != r.Pair.CenterRuntime || output.DispatchID != dispatchID || output.Source != "transcript" || !output.SourceExact || output.SourceIdentity == "" {
			return evidence, errors.New("Orca exact public transcript unavailable")
		}
		if evidence.SourceIdentity != "" && (evidence.SourceIdentity != output.SourceIdentity || evidence.Provider != output.Provider) {
			return evidence, errors.New("Orca public output changed source")
		}
		evidence.Source, evidence.SourceIdentity, evidence.Provider, evidence.SourceExact = output.Source, output.SourceIdentity, output.Provider, output.SourceExact
		evidence.Messages += len(output.Transcript.Messages)
		for _, message := range output.Transcript.Messages {
			if message.Role == "assistant" {
				for _, block := range message.Blocks {
					if block.Type == "text" && strings.Contains(block.Text, prURL) {
						foundPR = true
					}
				}
			}
		}
		if !output.Transcript.Limited {
			if evidence.Messages == 0 || !foundPR {
				return evidence, errors.New("public worker output lacks original assistant PR report")
			}
			return evidence, nil
		}
		next := output.Transcript.NextCursor
		if next == "" || next == cursor {
			return evidence, errors.New("Orca public transcript cursor did not advance")
		}
		cursor = next
	}
	return evidence, errors.New("public transcript pagination exceeded bound")
}

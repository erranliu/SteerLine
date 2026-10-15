package driver

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	productquery "github.com/erranliu/SteerLine/internal/product/query"
	"github.com/erranliu/SteerLine/internal/runnerbridge"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
	"os"
	"path/filepath"
	"regexp"
	"time"

	steerlineapp "github.com/erranliu/SteerLine/internal/app"
	steerline "github.com/erranliu/SteerLine/internal/app/host"
	"github.com/erranliu/SteerLine/internal/storage/sqlite"
)

type CandidateCommand struct {
	Action, RequestID, TaskID, ProfileID, RunID, ExecutionID, DecisionID, Option, Note, InstanceID string
	TaskVersion, GraphRevision, ControlRevision, DecisionVersion, RunRevision                      uint64
	AttemptID                                                                                      string
	RecoveryVersion                                                                                uint64
}
type CandidateReceipt struct {
	PreAcceptRejection string                      `json:"pre_accept_rejection,omitempty"`
	Request            *productapi.ControlRequest  `json:"request,omitempty"`
	Control            *productapi.ControlReceipt  `json:"control,omitempty"`
	Command            *storageports.CommandRecord `json:"command,omitempty"`
}

func (c *Client) Candidate(ctx context.Context, helper string, in CandidateCommand) (CandidateReceipt, error) {
	var result CandidateReceipt
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(in.RequestID) {
		return result, fmt.Errorf("candidate request requires exact immutable ID")
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return result, err
	}
	path := filepath.Join(c.Home, "candidate-"+in.RequestID+".json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return result, err
	}
	defer os.Remove(path)
	raw, err = c.runWithTimeout(ctx, 150*time.Second, helper, "candidate", "-config", c.Config, "-input", path)
	if len(raw) > 0 {
		if decodeErr := json.Unmarshal(raw, &result); decodeErr != nil {
			return result, decodeErr
		}
	}
	return result, err
}

type CandidateConversation struct {
	Events []productapi.ConversationEvent    `json:"events"`
	Bodies []productapi.ConversationBodyPage `json:"bodies"`
}

func ReadOriginalOperations(ctx context.Context, database, project string) ([]runnerbridge.Candidate, error) {
	reader, err := sqlite.OpenSQLiteReader(database)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return reader.RunnerCandidates(ctx, project, "")
}

func ReadCandidateConversation(ctx context.Context, database, project, runID, executionID string) (CandidateConversation, error) {
	var result CandidateConversation
	reader, err := sqlite.OpenSQLiteReader(database)
	if err != nil {
		return result, err
	}
	defer reader.Close()
	q := productquery.NewConversationService(reader, project)
	filter := productapi.ConversationFilter{WorkflowID: runID, ExecutionID: executionID}
	cursor := ""
	for n := 0; n < 100; n++ {
		page, err := q.ReadConversation(ctx, filter, cursor, 100)
		if err != nil {
			return result, err
		}
		for _, event := range page.Events {
			if event.WorkflowID != runID || event.ExecutionID != executionID {
				return result, fmt.Errorf("canonical conversation crossed candidate")
			}
			result.Events = append(result.Events, event)
			if event.Type == productapi.ConversationMessageComplete && event.Body != nil {
				body, err := q.ReadMessageBody(ctx, *event.Body, 0, 256<<10)
				if err != nil {
					return result, err
				}
				if !body.Complete || body.SHA256 != event.Body.SHA256 {
					return result, fmt.Errorf("canonical candidate body missing/truncated")
				}
				result.Bodies = append(result.Bodies, body)
			}
		}
		if !page.HasMore {
			return result, nil
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			return result, fmt.Errorf("canonical candidate cursor did not advance")
		}
		cursor = page.NextCursor
	}
	return result, fmt.Errorf("canonical candidate evidence exceeded bound")
}

// MainCandidate submits through the same public APIs as the product UI, while
// retaining the allocated request and exact feedback command for receipt lookup.
func MainCandidate(args []string) error {
	fs := flag.NewFlagSet("E2E candidate", flag.ContinueOnError)
	config := fs.String("config", "", "project config")
	input := fs.String("input", "", "immutable command")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	var in CandidateCommand
	if err = json.Unmarshal(raw, &in); err != nil {
		return err
	}
	c, err := steerlineapp.LoadConfig(*config)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 145*time.Second)
	defer cancel()
	if err = c.ResolveIdentity(ctx); err != nil {
		return err
	}
	hostConfig, err := steerlineapp.BuildHostConfig(ctx, c)
	if err != nil {
		return err
	}
	if in.Action == "answer_decision" {
		reader, err := steerlineapp.OpenNativeTaskReader(ctx, &c)
		if err != nil {
			return err
		}
		candidate, readErr := reader.Candidate(ctx, c.Identity(), in.RunID)
		_ = reader.Close()
		if readErr != nil {
			return readErr
		}
		if candidate.RunID != in.ExecutionID {
			return fmt.Errorf("feedback candidate/Runner identity mismatch")
		}
		client, err := steerlineapp.OpenClient(ctx, *config)
		if err != nil {
			return err
		}
		defer client.Close()
		cmd, _, err := client.SubmitDecision(ctx, productapi.DecisionInput{CommandID: in.RequestID, RequestedBy: "e2e", Answer: productapi.DecisionAnswer{RunID: in.RunID, DecisionID: in.DecisionID, OptionID: in.Option, Note: in.Note, AnsweredBy: "e2e", ExpectedDecisionVersion: in.DecisionVersion, ExpectedRunRevision: in.RunRevision, ServiceInstanceID: in.InstanceID, SourceAttemptID: in.AttemptID, ExpectedRecoveryVersion: in.RecoveryVersion}})

		if err != nil {
			// A fresh ID is safe only after proving this original request was
			// never persisted. Transport/unknown outcomes retain their ID.
			var rejected *taskmodel.Error
			if errors.As(err, &rejected) && (rejected.Code == "stale_version" || rejected.Code == "stale_recovery") {
				if _, lookupErr := steerline.ReadControlRequest(hostConfig, in.RequestID); os.IsNotExist(lookupErr) {
					if encodeErr := json.NewEncoder(os.Stdout).Encode(CandidateReceipt{PreAcceptRejection: rejected.Code}); encodeErr != nil {
						return encodeErr
					}
				}
			}
			return err
		}
		if cmd.Status == "pending" || cmd.Status == "in_flight" {
			cmd, err = client.WaitCommand(ctx, in.RequestID, 140*time.Second)
			if err != nil {
				return err
			}
		}
		result, _ := json.Marshal(map[string]string{"code": cmd.ResultCode, "error": cmd.ResultError})
		display := storageports.CommandRecord{ProjectIdentity: cmd.ProjectIdentity, CommandID: cmd.CommandID, Kind: cmd.Kind, TargetID: cmd.TargetID, ExpectedVersion: cmd.ExpectedVersion, RequestedBy: cmd.RequestedBy, Status: cmd.Status, Result: result, CreatedAt: cmd.CreatedAt, UpdatedAt: cmd.UpdatedAt}
		if err := json.NewEncoder(os.Stdout).Encode(CandidateReceipt{Command: &display}); err != nil {
			return err
		}
		if cmd.Status != "accepted" {
			return fmt.Errorf("answer command %s %s: %s", cmd.CommandID, cmd.Status, cmd.ResultError)
		}
		return nil
	}
	if in.Action == "receipt" {
		receipt, err := steerline.ReadControlReceipt(hostConfig, in.RequestID)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(CandidateReceipt{Control: receipt})
	}
	if in.Action != "start_candidate" && in.Action != "stop_candidate" && in.Action != "start_task" {
		return fmt.Errorf("unsupported candidate control")
	}
	payload := productapi.ControlPayload{TaskID: in.TaskID}
	if in.Action == "stop_candidate" {
		payload.RunID, payload.ExpectedRunRevision = in.RunID, in.RunRevision
	} else {
		payload.ExpectedTaskVersion, payload.ExpectedGraphRevision = in.TaskVersion, in.GraphRevision
		profile, err := c.RunnerExecution.Profile(in.ProfileID, "")
		if err != nil {
			return err
		}
		payload.ExecutionProfileID, payload.ExecutionProfileDigest = profile.ID, profile.Digest()
	}
	var revision *uint64
	if in.ControlRevision != 0 {
		revision = &in.ControlRevision
	}
	req, err := steerline.SubmitControlTargeted(hostConfig, in.Action, payload, "e2e", in.RequestID, in.InstanceID, revision)
	if err != nil {
		return err
	}
	receipt, err := steerline.WaitControlReceipt(ctx, hostConfig, req.RequestID, 100*time.Millisecond)
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(CandidateReceipt{Request: &req})
		return err
	}
	// Rejection is a real receipt (CN8 asserts it); never erase it with an error.
	return json.NewEncoder(os.Stdout).Encode(CandidateReceipt{Request: &req, Control: receipt})
}

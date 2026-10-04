package driver

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	executionmodel "github.com/erranliu/SteerLine/internal/execution/model"
	productapi "github.com/erranliu/SteerLine/internal/product/api"
	storageports "github.com/erranliu/SteerLine/internal/storage/ports"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/erranliu/SteerLine/internal/steerline"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"github.com/erranliu/SteerLine/internal/steerline/sqlite"
	"github.com/erranliu/SteerLine/internal/steerlineapp"
)

type CandidateCommand struct {
	Action, RequestID, TaskID, ProfileID, RunID, ExecutionID, DecisionID, Option, Note, InstanceID string
	TaskVersion, GraphRevision, CandidateVersion, DecisionVersion, RunRevision                     uint64
}
type CandidateReceipt struct {
	Request *productapi.ControlRequest  `json:"request,omitempty"`
	Control *productapi.ControlReceipt  `json:"control,omitempty"`
	Command *storageports.CommandRecord `json:"command,omitempty"`
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
	Events []contracts.ConversationEvent    `json:"events"`
	Bodies []contracts.ConversationBodyPage `json:"bodies"`
}

func ReadOriginalOperations(ctx context.Context, database, project string) ([]executionmodel.OperationRecord, error) {
	reader, err := (sqlite.Factory{}).OpenReader(database)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	domain, err := reader.Query(ctx, project)
	return domain.Operations, err
}

func ReadCandidateConversation(ctx context.Context, database, project, runID, executionID string) (CandidateConversation, error) {
	var result CandidateConversation
	reader, err := (sqlite.Factory{}).OpenReader(database)
	if err != nil {
		return result, err
	}
	defer reader.Close()
	q := steerline.NewConversationQueryService(reader, project)
	filter := contracts.ConversationFilter{WorkflowID: runID, ExecutionID: executionID}
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
			if event.Type == contracts.ConversationMessageComplete && event.Body != nil {
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
	if in.Action == "answer_decision" {
		reader, err := steerline.OpenNativeTaskReader(ctx, &c)
		if err != nil {
			return err
		}
		defer reader.Close()
		d, err := steerline.NewQueryService(reader, c.Identity()).Workflow(ctx, "")
		if err != nil {
			return err
		}
		matched := false
		for _, e := range d.Executions {
			if e.ExecutionID == in.ExecutionID && e.WorkflowRunID == in.RunID {
				matched = true
			}
		}
		if !matched {
			return fmt.Errorf("feedback execution/run identity mismatch")
		}
		cmd, _, err := steerline.SubmitAnswerDecision(ctx, c, in.RunID, in.DecisionID, in.Option, in.Note, "e2e", in.DecisionVersion, in.RunRevision, in.InstanceID, in.RequestID)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(CandidateReceipt{Command: &cmd})
	}
	if in.Action == "receipt" {
		receipt, err := steerline.ReadControlReceipt(c, in.RequestID)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(CandidateReceipt{Control: receipt})
	}
	if in.Action != "start_candidate" && in.Action != "abandon_candidate" && in.Action != "start_task" {
		return fmt.Errorf("unsupported candidate control")
	}
	payload := productapi.ControlPayload{TaskID: in.TaskID, WorkflowRunID: in.RunID, ExpectedTaskVersion: in.TaskVersion, ExpectedGraphRevision: in.GraphRevision, ExpectedCandidateVersion: in.CandidateVersion, Reason: in.Note}
	if in.Action != "abandon_candidate" {
		_, profile, err := c.ResolveExecutionProfile(in.ProfileID, "")
		if err != nil {
			return err
		}
		payload.ExecutionProfileID, payload.ExecutionProfileDigest = profile.ID, profile.Digest()
	}
	req, err := steerline.SubmitControlTargeted(c, in.Action, payload, "e2e", in.RequestID, in.InstanceID, nil)
	if err != nil {
		return err
	}
	receipt, err := steerline.WaitControlReceipt(ctx, c, req.RequestID, 100*time.Millisecond)
	if err != nil {
		_ = json.NewEncoder(os.Stdout).Encode(CandidateReceipt{Request: &req})
		return err
	}
	// Rejection is a real receipt (CN8 asserts it); never erase it with an error.
	return json.NewEncoder(os.Stdout).Encode(CandidateReceipt{Request: &req, Control: receipt})
}

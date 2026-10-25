package runnerclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type ExecutionWaitCapability struct {
	Contract     string          `json:"contract"`
	SameSnapshot bool            `json:"same_snapshot"`
	RunProperty  string          `json:"run_property"`
	Schema       json.RawMessage `json:"schema"`
}

func (c *Catalog) compileExecutionWait() {
	c.executionWaitSchema = nil
	p := c.Capabilities.ExecutionWait
	c.executionWaitError = errors.New("当前 Runner 不支持等待状态契约")
	if p == nil || p.Contract != runnercontract.ExecutionWaitContract || !p.SameSnapshot || p.RunProperty != "execution_wait" || len(p.Schema) == 0 {
		return
	}
	schema, err := compilePublicSchema(p.Schema, "execution-wait")
	if err == nil {
		err = validatePublicValue(schema, nil)
	}
	if err != nil {
		c.executionWaitError = fmt.Errorf("Runner 等待状态 schema 不可用: %w", err)
		return
	}
	c.executionWaitSchema, c.executionWaitError = schema, nil
}

// ExecutionWaitUnavailable gates only new Orca candidates, not existing reads,
// stop obligations, content workflows or other adapters.
func (c *Catalog) ExecutionWaitUnavailable(execution ExecutionInput) string {
	if execution.Adapter != "orca-terminal" {
		return ""
	}
	if c == nil || c.executionWaitSchema == nil {
		if c != nil && c.executionWaitError != nil {
			return c.executionWaitError.Error()
		}
		return "当前 Runner 不支持等待状态契约"
	}
	observation := c.Capabilities.RunObservation
	if observation == nil || observation.Contract != RunObservationContract || !observation.SameSnapshot || observation.MaxEvents < 100 {
		return "当前 Runner 不支持同快照等待状态观察"
	}
	return ""
}

func (c Client) validateRunWait(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return errors.New("Runner Run is not an object")
	}
	value, present := fields["execution_wait"]
	if c.executionWaitSchema != nil && !present {
		return errors.New("Runner did not return execution_wait or explicit null")
	}
	if present {
		return c.validateWaitValue(value)
	}
	return nil
}

func (c Client) validateWaitValue(value json.RawMessage) error {
	var wait *runnercontract.ExecutionWait
	if err := json.Unmarshal(value, &wait); err != nil {
		return err
	}
	if c.executionWaitSchema != nil {
		var input any
		if json.Unmarshal(value, &input) != nil || c.executionWaitSchema.Validate(input) != nil {
			return errors.New("Runner preparation wait differs from its published schema")
		}
	}
	return nil
}

func (c Client) validateWaitEvent(event Event) error {
	if _, err := event.Wait(); err != nil {
		return err
	}
	if event.Type == "execution_wait_changed" {
		return c.validateWaitValue(event.ExecutionWait)
	}
	return nil
}

func waitFailure(err error) error {
	return &Failure{Code: "runner_unavailable", Message: "等待信息暂时不可核实: " + err.Error(), Exit: 4}
}

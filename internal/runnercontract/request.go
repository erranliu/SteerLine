package runnercontract

import "encoding/json"

const Protocol = "steerrunner-cli/v1"

type Request struct {
	Protocol  string          `json:"protocol"`
	RequestID string          `json:"request_id"`
	Input     json.RawMessage `json:"input"`
}

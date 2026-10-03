package orcaworker

import "encoding/json"

func SavedTerminal(payload []byte) (Terminal, error) {
	var receipt struct {
		Terminal Terminal `json:"terminal"`
	}
	err := json.Unmarshal(payload, &receipt)
	return receipt.Terminal, err
}

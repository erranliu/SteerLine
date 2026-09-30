package orcaworker

import "encoding/json"

func SavedAgentHandle(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", nil
	}
	var receipt struct {
		AgentTerminalHandle string `json:"agentTerminalHandle"`
		StartupTerminal     struct {
			Handle string `json:"handle"`
		} `json:"startupTerminal"`
	}
	if err := json.Unmarshal(payload, &receipt); err != nil {
		return "", err
	}
	if receipt.AgentTerminalHandle != "" {
		return receipt.AgentTerminalHandle, nil
	}
	return receipt.StartupTerminal.Handle, nil
}

func SavedTerminal(payload []byte) (Terminal, error) {
	var receipt struct {
		Terminal Terminal `json:"terminal"`
	}
	err := json.Unmarshal(payload, &receipt)
	return receipt.Terminal, err
}

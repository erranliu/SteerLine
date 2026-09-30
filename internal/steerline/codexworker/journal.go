package codexworker

import (
	"errors"
	"os"
	"path/filepath"
)

type TurnPaths struct {
	TurnDir     string
	JournalPath string
	ServerDir   string
}

// PrepareTurn creates the fixed journal locations before any native send.
func PrepareTurn(runDir, runID, commandID, serverDir, prompt string, writeAtomic func(string, []byte) error) (TurnPaths, error) {
	dir := filepath.Join(runDir, "runs", runID, "turns", commandID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return TurnPaths{}, err
	}
	journalPath := filepath.Join(dir, "appserver.jsonl")
	if _, err := os.Stat(journalPath); err == nil {
		return TurnPaths{}, errors.New("original app-server turn journal exists; send must be reconciled, not repeated")
	} else if !os.IsNotExist(err) {
		return TurnPaths{}, err
	}
	if err := writeAtomic(filepath.Join(dir, "prompt.txt"), []byte(prompt)); err != nil {
		return TurnPaths{}, err
	}
	if serverDir == "" {
		return TurnPaths{}, errors.New("app-server directory is missing")
	}
	if err := os.MkdirAll(serverDir, 0700); err != nil {
		return TurnPaths{}, err
	}
	return TurnPaths{TurnDir: dir, JournalPath: journalPath, ServerDir: serverDir}, nil
}

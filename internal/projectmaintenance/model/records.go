// Package model contains maintenance facts shared by rules and persistence.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const RulesVersion = "project-maintenance/1"

func Digest(b []byte) string     { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func TextDigest(b []byte) string { return Digest([]byte(strings.ReplaceAll(string(b), "\r\n", "\n"))) }
func Hash(v any) string          { b, _ := json.Marshal(v); return Digest(b) }

type Approval struct {
	Project   string    `json:"project"`
	Plan      string    `json:"plan"`
	Revision  uint64    `json:"revision"`
	Inputs    string    `json:"inputs"`
	Output    string    `json:"output"`
	Commit    string    `json:"commit"`
	By        string    `json:"by"`
	RequestID string    `json:"request_id"`
	At        time.Time `json:"at"`
}

type Observation struct {
	Commit      string    `json:"commit"`
	Previous    string    `json:"previous,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
	LastSuccess time.Time `json:"last_success,omitempty"`
	Diagnostic  string    `json:"diagnostic,omitempty"`
	Rewritten   bool      `json:"rewritten"`
}

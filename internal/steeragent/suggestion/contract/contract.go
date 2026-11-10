// Package contract defines the value-only advisory boundary. It has no host or execution dependencies.
package contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

const Protocol = "steerline-main-agent/advisory-v1"
const Capability = "advisory"
const MaxContextBytes = 1 << 20
const MaxPromptBytes = 16 << 10

var ID = regexp.MustCompile(`^[0-9a-f]{32}$`)
var ErrNotFound = errors.New("advisory not found")
var ErrConflict = errors.New("advisory revision conflict")

type Material struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Body     string `json:"body"`
	Source   string `json:"source,omitempty"`
	Revision string `json:"revision,omitempty"`
}
type Context struct {
	Goal      string     `json:"goal"`
	UserInput string     `json:"user_input"`
	Materials []Material `json:"materials"`
}
type Request struct {
	Protocol   string  `json:"protocol"`
	RequestID  string  `json:"request_id"`
	Capability string  `json:"capability"`
	Context    Context `json:"context"`
}
type Suggestion struct {
	Protocol  string `json:"protocol"`
	RequestID string `json:"request_id"`
	Prompt    string `json:"prompt"`
}

func Digest(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func Decode(raw []byte) (Request, error) {
	var r Request
	if !utf8.Valid(raw) {
		return r, errors.New("advisory request is not UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return r, errors.New("exactly one advisory request required")
	}
	return r, Validate(r)
}
func Validate(r Request) error {
	if r.Protocol != Protocol || r.Capability != Capability || !ID.MatchString(r.RequestID) {
		return errors.New("unsupported advisory protocol, capability or request identity")
	}
	if strings.TrimSpace(r.Context.Goal) == "" || r.Context.Materials == nil || len(r.Context.Materials) > 64 {
		return errors.New("advisory requires a goal and at most 64 explicit materials")
	}
	seen := map[string]bool{}
	for _, m := range r.Context.Materials {
		if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.Label) == "" || seen[m.ID] {
			return errors.New("advisory material requires unique ID and label")
		}
		seen[m.ID] = true
		for _, s := range []string{m.ID, m.Label, m.Body, m.Source, m.Revision} {
			if !utf8.ValidString(s) {
				return errors.New("advisory material is not UTF-8")
			}
		}
	}
	if !utf8.ValidString(r.Context.Goal) || !utf8.ValidString(r.Context.UserInput) {
		return errors.New("advisory context is not UTF-8")
	}
	raw, err := json.Marshal(r.Context)
	if err != nil || len(raw) > MaxContextBytes {
		return errors.New("advisory context exceeds 1 MiB")
	}
	return nil
}
func ValidateSuggestion(s Suggestion) error {
	if s.Protocol != Protocol || !ID.MatchString(s.RequestID) || strings.TrimSpace(s.Prompt) == "" || !utf8.ValidString(s.Prompt) || len(s.Prompt) > MaxPromptBytes {
		return errors.New("advisory requires one nonempty UTF-8 prompt, at most 16 KiB")
	}
	return nil
}

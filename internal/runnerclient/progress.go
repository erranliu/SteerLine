package runnerclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/erranliu/SteerLine/internal/runnercontract"
)

type ProgressCapability struct {
	Contract            string `json:"contract"`
	Command             string `json:"command"`
	SubmitProperty      string `json:"submit_property"`
	EnvironmentVariable string `json:"environment_variable"`
	MaxBytes            int    `json:"max_bytes"`
	Encoding            string `json:"encoding"`
	Digest              string `json:"digest"`
}

func (c *Catalog) ValidateProgress() error {
	if c == nil {
		return errors.New("Runner progress-file capability unavailable")
	}
	p := c.Capabilities.ProgressFile
	if p == nil || p.Contract != runnercontract.ProgressContract || p.Command != "progress-file --run-id R --json" || p.SubmitProperty != "progress_file" || p.EnvironmentVariable != "STEERLINE_PROGRESS_FILE" || p.MaxBytes != runnercontract.MaxProgressBytes || p.Encoding != "base64" || p.Digest != "sha256" || len(c.Capabilities.ProgressFileSchema) == 0 {
		return errors.New("Runner progress-file capability is incomplete")
	}
	var doc struct {
		ID         string                     `json:"$id"`
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(c.Capabilities.ProgressFileSchema, &doc) != nil || doc.ID != runnercontract.ProgressContract {
		return errors.New("Runner progress-file schema contract differs")
	}
	for _, name := range []string{"contract", "run_id", "version", "encoding", "size_bytes", "collection_status"} {
		found := false
		for _, key := range doc.Required {
			found = found || key == name
		}
		if !found || len(doc.Properties[name]) == 0 {
			return errors.New("Runner progress-file schema is incomplete")
		}
	}
	_, err := compilePublicSchema(c.Capabilities.ProgressFileSchema, "progress-file")
	return err
}

// ValidateProgressFile verifies transport identity and bytes, without interpreting their format.
func ValidateProgressFile(p runnercontract.ProgressFile, run string) ([]byte, error) {
	bad := func() ([]byte, error) {
		return nil, errors.New("Runner progress snapshot identity, version or bytes are invalid")
	}
	if p.Contract != runnercontract.ProgressContract || p.RunID != run || run == "" || p.Encoding != "base64" {
		return bad()
	}
	switch p.CollectionStatus {
	case "disabled", "pending", "initializing", "observing", "unavailable", "closed", "detached":
	default:
		return bad()
	}
	if p.Version == 0 {
		if p.ContentBase64 != nil || p.ObservedAt != nil || p.SHA256 != "" || p.SizeBytes != 0 {
			return bad()
		}
		return nil, nil
	}
	if p.ContentBase64 == nil || p.ObservedAt == nil || p.ObservedAt.IsZero() || p.SizeBytes < 0 || p.SizeBytes > runnercontract.MaxProgressBytes || len(*p.ContentBase64) > base64.StdEncoding.EncodedLen(runnercontract.MaxProgressBytes) {
		return bad()
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(*p.ContentBase64)
	if err != nil || base64.StdEncoding.EncodeToString(raw) != *p.ContentBase64 || len(raw) != p.SizeBytes {
		return bad()
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != p.SHA256 {
		return bad()
	}
	return raw, nil
}

func (c Client) ProgressFile(ctx context.Context, run string) (runnercontract.ProgressFile, error) {
	var out runnercontract.ProgressFile
	response, err := c.Call(ctx, "progress-file", []string{"--run-id", run}, nil)
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal(response.Data, &out); err != nil {
		return out, err
	}
	_, err = ValidateProgressFile(out, run)
	return out, err
}

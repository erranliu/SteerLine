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

type Artifact = runnercontract.Artifact
type ArtifactDiscovery = runnercontract.ArtifactDiscovery
type ArtifactList = runnercontract.ArtifactList
type WorkspaceGit = runnercontract.WorkspaceGit

func (c *Catalog) ValidateEvidence() error {
	if c == nil || c.Capabilities.RunEvidence == nil || c.Capabilities.RunEvidence.Version != 1 {
		return errors.New("Runner run_evidence/v1 unavailable")
	}
	required := map[string]bool{"artifacts discover": false, "artifacts collect": false, "artifacts list": false, "artifacts read": false, "workspace-git": false}
	for _, command := range c.Capabilities.RunEvidence.Commands {
		if _, ok := required[command]; ok {
			required[command] = true
		}
	}
	for _, present := range required {
		if !present {
			return errors.New("incomplete run_evidence commands")
		}
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(c.Capabilities.RunEvidenceSchema, &doc) != nil {
		return errors.New("missing evidence schema")
	}
	var defs map[string]json.RawMessage
	if json.Unmarshal(doc["$defs"], &defs) != nil {
		return errors.New("missing evidence definitions")
	}
	for _, name := range []string{"collect_input", "discover", "collect", "list", "read", "workspace_git"} {
		if len(defs[name]) == 0 {
			return errors.New("incomplete evidence schema: " + name)
		}
		doc["$ref"], _ = json.Marshal("#/$defs/" + name)
		raw, _ := json.Marshal(doc)
		if _, err := compilePublicSchema(raw, "run-evidence-"+name); err != nil {
			return err
		}
	}
	return nil
}
func (c Client) DiscoverArtifacts(ctx context.Context, run string) (ArtifactDiscovery, error) {
	var out ArtifactDiscovery
	response, err := c.Call(ctx, "artifacts discover", []string{"--run-id", run}, nil)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(response.Data, &out) != nil || out.RunID != run || out.Directory != runnercontract.ArtifactDirectory || out.Candidates == nil || len(out.Candidates) > runnercontract.MaxDiscoveryEntries {
		return out, errors.New("invalid artifact discovery")
	}
	seen := map[string]bool{}
	for _, v := range out.Candidates {
		if !runnercontract.ValidArtifactPath(v.RelativePath) || v.SizeBytes < 0 || v.SizeBytes > runnercontract.MaxArtifactBytes || seen[v.RelativePath] {
			return out, errors.New("invalid artifact path/size")
		}
		seen[v.RelativePath] = true
	}
	return out, nil
}
func (c Client) ListArtifacts(ctx context.Context, run string) (ArtifactList, error) {
	var out ArtifactList
	response, err := c.Call(ctx, "artifacts list", []string{"--run-id", run}, nil)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(response.Data, &out) != nil || out.RunID != run || out.Artifacts == nil || len(out.Artifacts) > runnercontract.MaxRunArtifacts {
		return out, errors.New("invalid artifact list")
	}
	seen := map[string]bool{}
	for _, a := range out.Artifacts {
		if err := validateArtifact(a, run); err != nil {
			return out, err
		}
		if seen[a.ArtifactID] {
			return out, errors.New("duplicate artifact identity")
		}
		seen[a.ArtifactID] = true
	}
	return out, nil
}
func validateArtifact(a Artifact, run string) error {
	sum, err := hex.DecodeString(a.SHA256)
	if err != nil || len(sum) != 32 || a.RunID != run || a.ArtifactID == "" || !runnercontract.ValidArtifactPath(a.RelativePath) || a.SizeBytes < 0 || a.SizeBytes > runnercontract.MaxArtifactBytes || a.CollectedAt.IsZero() {
		return errors.New("invalid artifact metadata")
	}
	return nil
}
func (c Client) ReadArtifact(ctx context.Context, run, id string) (Artifact, []byte, error) {
	response, err := c.Call(ctx, "artifacts read", []string{"--run-id", run, "--artifact-id", id}, nil)
	if err != nil {
		return Artifact{}, nil, err
	}
	var out runnercontract.ArtifactContent
	if json.Unmarshal(response.Data, &out) != nil || out.ArtifactID != id || out.Encoding != "base64" {
		return out.Artifact, nil, errors.New("invalid artifact content")
	}
	if err = validateArtifact(out.Artifact, run); err != nil {
		return out.Artifact, nil, err
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(out.ContentBase64)
	if err != nil || len(raw) != out.SizeBytes || len(raw) > runnercontract.MaxArtifactBytes {
		return out.Artifact, nil, errors.New("artifact byte length differs")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != out.SHA256 {
		return out.Artifact, nil, errors.New("artifact SHA256 differs")
	}
	return out.Artifact, raw, nil
}
func (c Client) CollectArtifact(ctx context.Context, run string, request Request) (Artifact, error) {
	response, err := c.Call(ctx, "artifacts collect", []string{"--run-id", run}, &request)
	if err != nil {
		return Artifact{}, err
	}
	var out struct {
		Receipt
		Artifact Artifact `json:"artifact"`
	}
	if json.Unmarshal(response.Data, &out) != nil || out.RunID != run || out.CommandID == "" || out.ReceiptStatus != "accepted" || out.ActionStatus != "completed" {
		return out.Artifact, &Failure{Code: "request_outcome_unknown", Unknown: true, Exit: 6, Message: "artifact collection receipt unverified"}
	}
	return out.Artifact, validateArtifact(out.Artifact, run)
}
func (c Client) WorkspaceGit(ctx context.Context, run string) (WorkspaceGit, error) {
	var out WorkspaceGit
	response, err := c.Call(ctx, "workspace-git", []string{"--run-id", run}, nil)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(response.Data, &out) != nil || out.RunID != run || out.ObservedAt.IsZero() || out.Verified && (out.Commit == nil || out.Clean == nil || len(*out.Commit) != 40 && len(*out.Commit) != 64) || !out.Verified && (out.Commit != nil || out.Clean != nil) {
		return out, errors.New("invalid workspace Git observation")
	}
	if out.Verified {
		if _, err := hex.DecodeString(*out.Commit); err != nil {
			return out, errors.New("invalid workspace commit")
		}
	}
	return out, nil
}

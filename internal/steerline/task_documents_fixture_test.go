package steerline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	execution "github.com/erranliu/SteerLine/internal/execution"
	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// Git access stays in the app adapter; task sees bytes of an exact regular blob.
type taskDocuments struct {
	processHost executionbackend.ManagedProcessHost
	target      func(context.Context) (string, error)
	repository  string
	git         executionbackend.Command
	timeout     time.Duration
}

func (a taskDocuments) ReadCommittedRegularFile(ctx context.Context, ref taskmodel.FileReference) ([]byte, error) {
	if err := taskmodel.ValidateFileReference(ref); err != nil {
		return nil, err
	}
	r := execution.Runner{Dir: a.repository, Timeout: a.timeout, ProcessHost: a.processHost}
	resolved, err := r.Output(ctx, a.git, "rev-parse", "--verify", ref.Commit+"^{commit}")
	if err != nil || strings.TrimSpace(string(resolved)) != ref.Commit {
		return nil, taskDocumentError("pinned Git commit is missing or is not a full canonical commit: " + ref.Commit)
	}
	tree, err := r.Output(ctx, a.git, "ls-tree", "-z", ref.Commit, "--", ref.Path)
	if err != nil {
		return nil, taskDocumentError("cannot resolve committed contract file: " + ref.Path)
	}
	entries := bytes.Split(bytes.TrimSuffix(tree, []byte{0}), []byte{0})
	if len(entries) != 1 || len(entries[0]) == 0 {
		return nil, taskDocumentError("pinned contract path is missing or ambiguous: " + ref.Path)
	}
	parts := bytes.SplitN(entries[0], []byte{'\t'}, 2)
	if len(parts) != 2 || string(parts[1]) != ref.Path {
		return nil, taskDocumentError("pinned contract path is not an exact repository file: " + ref.Path)
	}
	meta := strings.Fields(string(parts[0]))
	if len(meta) != 3 || meta[1] != "blob" || (meta[0] != "100644" && meta[0] != "100755") {
		return nil, taskDocumentError("contract references must resolve to regular committed files, not links or directories")
	}
	body, err := r.Output(ctx, a.git, "cat-file", "blob", meta[2])
	if err != nil {
		return nil, taskDocumentError("cannot read committed blob: " + ref.Path)
	}
	return body, nil
}
func taskDocumentError(message string) error {
	return &taskmodel.Error{Code: "contract_mismatch", Message: message}
}
func taskPayloadDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (a taskDocuments) RepositoryTarget(ctx context.Context) (string, error) { return a.target(ctx) }

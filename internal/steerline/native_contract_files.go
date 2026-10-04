package steerline

import (
	"bytes"
	"context"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

// VerifyTaskContractFiles validates pinned committed blobs. TASK-DAG must also
// check the actual execution workspace before claim/implementation; this check
// alone is never a startup permission.
func VerifyTaskContractFiles(ctx context.Context, c Config, v taskmodel.TaskContract) error {
	if err := requireNativeTaskAuthority(c); err != nil {
		return err
	}
	if err := taskmodel.ValidateContract(v); err != nil {
		return err
	}
	target, err := configuredRepositoryTarget(ctx, c)
	if err != nil {
		return err
	}
	if !sameRepositoryTarget(v.Repository, target) || v.Remote != c.Remote || v.Branch != c.Branch {
		return nativeErr("contract_mismatch", "contract repository, remote or branch differs from the configured project")
	}
	r := Runner{Dir: c.Repository, Timeout: duration(c.CommandTimeout)}
	refs := []taskmodel.FileReference{v.SpecRef, v.PlanRef}
	refs = append(refs, v.CommonRefs...)
	refs = append(refs, v.AcceptanceRefs...)
	for _, ref := range refs {
		b, err := readCommittedRegularFile(ctx, r, c.Git, ref)
		if err != nil {
			return err
		}
		if err := taskmodel.ValidateDocumentContent(ref, b, taskmodel.IsAcceptanceReference(v.AcceptanceRefs, ref)); err != nil {
			return err
		}
	}
	return nil
}

func readCommittedRegularFile(ctx context.Context, r Runner, git executionbackend.Command, ref taskmodel.FileReference) ([]byte, error) {
	if err := taskmodel.ValidateFileReference(ref); err != nil {
		return nil, err
	}
	resolved, err := r.Output(ctx, git, "rev-parse", "--verify", ref.Commit+"^{commit}")
	if err != nil || strings.TrimSpace(string(resolved)) != ref.Commit {
		return nil, nativeErr("contract_mismatch", "pinned Git commit is missing or is not a full canonical commit: "+ref.Commit)
	}
	tree, err := r.Output(ctx, git, "ls-tree", "-z", ref.Commit, "--", ref.Path)
	if err != nil {
		return nil, nativeErr("contract_mismatch", "cannot resolve committed contract file: "+ref.Path)
	}
	entries := bytes.Split(bytes.TrimSuffix(tree, []byte{0}), []byte{0})
	if len(entries) != 1 || len(entries[0]) == 0 {
		return nil, nativeErr("contract_mismatch", "pinned contract path is missing or ambiguous: "+ref.Path)
	}
	parts := bytes.SplitN(entries[0], []byte{'\t'}, 2)
	if len(parts) != 2 || string(parts[1]) != ref.Path {
		return nil, nativeErr("contract_mismatch", "pinned contract path is not an exact repository file: "+ref.Path)
	}
	meta := strings.Fields(string(parts[0]))
	if len(meta) != 3 || meta[1] != "blob" || (meta[0] != "100644" && meta[0] != "100755") {
		return nil, nativeErr("contract_mismatch", "contract references must resolve to regular committed files, not links or directories")
	}
	b, err := r.Output(ctx, git, "cat-file", "blob", meta[2])
	if err != nil {
		return nil, nativeErr("contract_mismatch", "cannot read committed blob: "+ref.Path)
	}
	return b, nil
}

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
	taskmodel "github.com/erranliu/SteerLine/internal/task/model"
)

type materialReadKey struct{}

// Each assessment uses one request-scoped cache of immutable Git blobs. It is
// never shared between requests or used for mutable workspace files.
func materialReads(ctx context.Context) context.Context {
	if _, ok := ctx.Value(materialReadKey{}).(map[string][]byte); ok {
		return ctx
	}
	return context.WithValue(ctx, materialReadKey{}, map[string][]byte{})
}

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
	files, err := a.readCommittedFiles(ctx, ref.Commit, []string{ref.Path})
	return files[ref.Path], err
}

// readCommittedFile is also used for optional, commit-pinned testing JSON. The
// callers validate the path/commit before entering this shared Git reader.
func (a taskDocuments) readCommittedFile(ctx context.Context, commit, name string) ([]byte, error) {
	files, err := a.readCommittedFiles(ctx, commit, []string{name})
	return files[name], err
}

// Read a finite set with one commit check, one exact tree lookup and one batch
// blob read. Process startup cost must not grow by three commands per file.
func (a taskDocuments) readCommittedFiles(ctx context.Context, commit string, names []string) (map[string][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(names))
	cache, _ := ctx.Value(materialReadKey{}).(map[string][]byte)
	pending := []string{}
	requested := map[string]bool{}
	for _, name := range names {
		if requested[name] {
			continue
		}
		requested[name] = true
		if body, ok := cache[a.repository+":"+commit+":"+name]; ok {
			files[name] = body
		} else {
			pending = append(pending, name)
		}
	}
	if len(pending) == 0 {
		return files, nil
	}
	r := commandRunner{Dir: a.repository, Timeout: a.timeout, ProcessHost: a.processHost}
	resolved, err := r.Output(ctx, a.git, "rev-parse", "--verify", commit+"^{commit}")
	if err != nil {
		return nil, fmt.Errorf("cannot verify pinned Git commit %s: %w", commit, err)
	}
	if strings.TrimSpace(string(resolved)) != commit {
		return nil, taskDocumentError("pinned Git commit is not a full canonical commit: " + commit)
	}
	args := append([]string{"--literal-pathspecs", "ls-tree", "-z", commit, "--"}, pending...)
	tree, err := r.Output(ctx, a.git, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot read pinned Git tree %s: %w", commit, err)
	}
	objects := map[string]string{}
	for _, entry := range bytes.Split(bytes.TrimSuffix(tree, []byte{0}), []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		parts := bytes.SplitN(entry, []byte{'\t'}, 2)
		if len(parts) != 2 || !requested[string(parts[1])] || objects[string(parts[1])] != "" {
			return nil, taskDocumentError("pinned paths must resolve to exact repository files")
		}
		name := string(parts[1])
		meta := strings.Fields(string(parts[0]))
		if len(meta) != 3 || meta[1] != "blob" || (meta[0] != "100644" && meta[0] != "100755") {
			return nil, &nonRegularCommittedFileError{taskDocumentError("pinned path must be a regular file, not a link or directory: " + name)}
		}
		objects[name] = meta[2]
	}
	var input strings.Builder
	for _, name := range pending {
		if objects[name] == "" {
			return nil, &missingCommittedFileError{fmt.Errorf("pinned path missing %s: %w", name, os.ErrNotExist)}
		}
		input.WriteString(objects[name] + "\n")
	}
	r.Input = []byte(input.String())
	body, err := r.Output(ctx, a.git, "cat-file", "--batch")
	if err != nil {
		return nil, fmt.Errorf("cannot read pinned Git blobs %s: %w", commit, err)
	}
	for _, name := range pending {
		header, rest, ok := bytes.Cut(body, []byte{'\n'})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 3 || fields[0] != objects[name] || fields[1] != "blob" {
			return nil, taskDocumentError("invalid Git batch blob header: " + name)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size >= len(rest) || rest[size] != '\n' {
			return nil, taskDocumentError("invalid Git batch blob length: " + name)
		}
		files[name], body = rest[:size], rest[size+1:]
	}
	if len(body) != 0 {
		return nil, taskDocumentError("unexpected trailing Git batch output")
	}
	for _, name := range pending {
		if cache != nil {
			cache[a.repository+":"+commit+":"+name] = files[name]
		}
	}
	return files, nil
}
func taskDocumentError(message string) error {
	return &taskmodel.Error{Code: "contract_mismatch", Message: message}
}
func taskPayloadDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (a taskDocuments) RepositoryTarget(ctx context.Context) (string, error) { return a.target(ctx) }

type nonRegularCommittedFileError struct{ error }

func (e *nonRegularCommittedFileError) Unwrap() error { return e.error }

// Only an absent path in a successfully read Git tree proves material deletion.
// A missing executable or working directory is a read failure, not revocation.
type missingCommittedFileError struct{ error }

func (e *missingCommittedFileError) Unwrap() error { return e.error }

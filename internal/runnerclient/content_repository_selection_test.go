package runnerclient

import (
	"github.com/erranliu/SteerLine/internal/runnercontract"
	"testing"
)

func TestRepositorySelectedManifestIsExact(t *testing.T) {
	repo := runnercontract.GenerateRepository{LocalPath: "fixed", Commit: "commit", Paths: []string{"plan.md"}}
	source := &runnercontract.RepositorySource{LocalPath: "fixed", Commit: "commit", Tree: "tree", Files: []runnercontract.RepositoryFile{{Path: "plan.md", Blob: "blob", SHA256: "digest", SizeBytes: 7}}}
	if err := VerifyRepositoryFiles(source, repo, map[string]string{"plan.md": "digest"}); err != nil {
		t.Fatal(err)
	}
	for _, files := range [][]runnercontract.RepositoryFile{nil, append(append([]runnercontract.RepositoryFile{}, source.Files...), runnercontract.RepositoryFile{Path: "extra"}), {{Path: "other", Blob: "blob", SHA256: "digest", SizeBytes: 7}}} {
		changed := *source
		changed.Files = files
		if VerifyRepositoryFiles(&changed, repo, map[string]string{"plan.md": "digest"}) == nil {
			t.Fatal("selection drift accepted")
		}
	}
}

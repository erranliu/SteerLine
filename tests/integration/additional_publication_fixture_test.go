package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// The production publication client queries this isolated provider. Only the
// provider merge fact changes; no test inserts a center TaskDelivery row.
type additionalProvider struct {
	mu           sync.Mutex
	server       *httptest.Server
	branch, head string
	merged       bool
	mergedAt     time.Time
	queries      int
}

func newAdditionalProvider(t *testing.T) *additionalProvider {
	t.Helper()
	p := &additionalProvider{}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if r.URL.Path == "/api/v1/repos/owner/repo" {
			_ = json.NewEncoder(w).Encode(map[string]string{"full_name": "owner/repo"})
			return
		}
		if r.URL.Path != "/api/v1/repos/owner/repo/pulls/901" {
			http.NotFound(w, r)
			return
		}
		p.queries++
		state := "open"
		var mergedAt any
		var mergeCommit any
		if p.merged {
			state = "closed"
			mergedAt = p.mergedAt
			mergeCommit = p.head
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"number": 901, "html_url": p.server.URL + "/owner/repo/pulls/901", "state": state, "merged": p.merged, "merged_at": mergedAt, "merge_commit_sha": mergeCommit, "base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "owner/repo"}}, "head": map[string]any{"ref": p.branch, "sha": p.head, "repo": map[string]string{"full_name": "owner/repo"}}})
	}))
	t.Cleanup(p.server.Close)
	return p
}

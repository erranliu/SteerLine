package publication

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/erranliu/SteerLine/internal/execution/model"
)

func TestGiteaQueryPreservesExplicitDraftState(t *testing.T) {
	for _, test := range []struct {
		name    string
		draft   any
		headRef string
	}{
		{name: "draft", draft: true, headRef: "candidate"},
		{name: "ready", draft: false, headRef: "candidate"},
		{name: "unknown", headRef: "candidate"},
		{name: "deleted branch", draft: false, headRef: "refs/pull/7/head"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/repos/owner/repo/pulls/7" {
					http.NotFound(w, r)
					return
				}
				fact := map[string]any{
					"html_url": "http://" + r.Host + "/owner/repo/pulls/7", "number": 7, "state": "open", "merged": false,
					"base": map[string]any{"ref": "main", "repo": map[string]string{"full_name": "owner/repo"}},
					"head": map[string]any{"ref": test.headRef, "label": "candidate", "repo": map[string]string{"full_name": "owner/repo"}},
				}
				if test.draft != nil {
					fact["draft"] = test.draft
				}
				_ = json.NewEncoder(w).Encode(fact)
			}))
			defer server.Close()
			url := server.URL + "/owner/repo/pulls/7"
			query := QueryClient{Timeout: time.Second}
			fact, err := query.Query(context.Background(), model.PRProviderConfig{Kind: "gitea", BaseURL: server.URL, Repository: "owner/repo"}, url)
			if err != nil {
				t.Fatal(err)
			}
			if want, known := test.draft.(bool); known && (fact.Draft == nil || *fact.Draft != want) || !known && fact.Draft != nil {
				t.Fatalf("draft fact = %v, expected %v", fact.Draft, test.draft)
			}
			if fact.HeadBranch != "candidate" {
				t.Fatalf("head branch = %q", fact.HeadBranch)
			}
		})
	}
}

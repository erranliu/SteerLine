package testenv

import (
	"net/url"
	"testing"
)

func TestValidationPullsQueryUsesProviderBaseFilter(t *testing.T) {
	base := "codex/e2e/original/validation/base"
	for _, provider := range []string{"https://github.com", "http://gitea.test"} {
		u, err := url.Parse(validationPullsPath(provider, "owner/project", base))
		if err != nil {
			t.Fatal(err)
		}
		query := u.Query()
		filter, limit, wrong := "base_branch", "limit", "base"
		if provider == "https://github.com" {
			filter, limit, wrong = "base", "per_page", "base_branch"
		}
		if query.Get(filter) != base || query.Get(limit) != "50" || query.Get(wrong) != "" || query.Get("state") != "all" {
			t.Fatal("provider would ignore the base filter and return unrelated PRs", u)
		}
	}
}

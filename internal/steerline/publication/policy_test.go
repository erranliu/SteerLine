package publication

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"strings"
	"testing"
)

func TestPublicationPolicySupportMatrix(t *testing.T) {
	github := Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: contracts.Command{Executable: "gh"}}
	gitea := Config{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "owner/repo"}
	for _, row := range []struct {
		host, publisher string
		provider        Config
		supported       bool
	}{
		{"local", "host_service", github, true}, {"local", "worker", github, false}, {"local", "host_service", gitea, false},
		{"orca", "worker", github, true}, {"orca", "worker", gitea, true}, {"orca", "host_service", github, false},
		{"orca_supervised", "worker", gitea, true}, {"orca_supervised", "host_service", github, false},
		{"other", "worker", github, false}, {"orca", "", github, false}, {"orca", "automatic", github, false},
		{"orca", "worker", Config{Kind: "command", BaseURL: github.BaseURL, Repository: github.Repository}, false},
	} {
		t.Run(row.host+"/"+row.publisher+"/"+row.provider.Kind, func(t *testing.T) {
			if err := CheckPolicy(executionapi.PublicationPolicy{Publisher: row.publisher}, row.host, "codex", row.provider); (err == nil) != row.supported {
				t.Fatalf("support=%t error=%v", row.supported, err)
			}
		})
	}
}

func TestPublicationSecretsRejectedBeforeFreeze(t *testing.T) {
	for _, raw := range []string{"https://user:secret-canary@forge.invalid/o/r.git", "https://forge.invalid/o/r.git?token=secret-canary", "ssh://git:secret-canary@forge.invalid/o/r.git", "https://forge.invalid/o/r.git#secret-canary"} {
		err := ValidateURL(raw)
		if err == nil || strings.Contains(err.Error(), "secret-canary") {
			t.Fatalf("secret URL not rejected/redacted: %v", err)
		}
	}
	for _, args := range [][]string{{"--token", "secret-canary"}, {"-c", "http.extraHeader=Authorization: secret-canary"}, {"--url=https://user:secret-canary@forge.invalid"}, {"--header", "X-API-Key: a9N2v7L8p4Q6"}, {"--api-key", "a9N2v7L8p4Q6"}, {"--auth-key=a9N2v7L8p4Q6"}} {
		err := ValidateNonSecret(Config{Query: contracts.Command{Executable: "gh", Args: args}}, contracts.Command{})
		if err == nil || strings.Contains(err.Error(), "secret-canary") || strings.Contains(err.Error(), "a9N2v7L8p4Q6") {
			t.Fatalf("secret argv not rejected/redacted: %v", err)
		}
	}
	for _, raw := range []string{"ssh://git@forge.invalid/o/r.git", "git@forge.invalid:o/r.git", "https://forge.invalid/o/r.git"} {
		if err := ValidateURL(raw); err != nil {
			t.Fatal(err)
		}
	}
}

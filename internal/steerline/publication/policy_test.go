package publication

import (
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	executionapi "github.com/erranliu/SteerLine/internal/steerline/execution"
	"strings"
	"testing"
)

func TestPublicationPolicyValidatesProviderValues(t *testing.T) {
	github := Config{Kind: "command", BaseURL: "https://github.com", Repository: "owner/repo", Query: contracts.Command{Executable: "gh"}}
	gitea := Config{Kind: "gitea", BaseURL: "https://forge.invalid", Repository: "owner/repo"}
	for _, provider := range []Config{github, gitea} {
		for _, publisher := range []string{string(executionapi.PublisherWorker)} {
			if err := CheckPolicy(executionapi.PublicationPolicy{Publisher: publisher}, provider); err != nil {
				t.Fatalf("valid policy/provider was rejected: %s/%s: %v", publisher, provider.Kind, err)
			}
		}
	}
	if err := CheckPolicy(executionapi.PublicationPolicy{Publisher: "host_service"}, github); err == nil {
		t.Fatal("retired host publisher was accepted")
	}
	if err := CheckPolicy(executionapi.PublicationPolicy{Publisher: "automatic"}, github); err == nil {
		t.Fatal("unknown publisher was accepted")
	}
	if err := CheckPolicy(executionapi.PublicationPolicy{Publisher: executionapi.PublisherWorker}, Config{Kind: "gitea", BaseURL: "file://invalid"}); err == nil {
		t.Fatal("invalid provider was accepted")
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

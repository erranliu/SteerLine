package model

import (
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	executionbackend "github.com/erranliu/SteerLine/internal/execution/backend"
)

type PRProviderConfig struct {
	Kind       string                   `json:"kind"`
	BaseURL    string                   `json:"base_url"`
	Repository string                   `json:"repository"`
	TokenEnv   string                   `json:"token_env"`
	Query      executionbackend.Command `json:"query"`
	Close      executionbackend.Command `json:"close,omitempty"`
}

type PublicationSettings struct {
	Provider           PRProviderConfig         `json:"provider"`
	Git                executionbackend.Command `json:"git"`
	Remote             string                   `json:"remote"`
	RemoteURL          string                   `json:"remote_url"`
	BaseBranch         string                   `json:"base_branch"`
	AuthenticationRefs []string                 `json:"authentication_refs"`
}

// ValidateRemote checks that the frozen PR target is bound to the selected
// Git remote. It is a value rule; provider I/O remains in execution/publication.
func (c PRProviderConfig) ValidateRemote(remote string) error {
	if c.Kind != "gitea" && c.Kind != "command" {
		return errors.New("pr.kind must be gitea or command")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid PR base_url; credentials must use token_env")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(c.Repository) || strings.Contains(c.Repository, "..") {
		return errors.New("PR repository must be owner/name")
	}
	if strings.Contains(remote, "://") {
		r, parseErr := url.Parse(remote)
		if parseErr != nil {
			return parseErr
		}
		hostMatches := strings.EqualFold(r.Host, u.Host)
		if r.Scheme == "ssh" {
			hostMatches = strings.EqualFold(r.Hostname(), u.Hostname())
		}
		expectedPath := strings.Trim(strings.TrimSuffix(u.Path, "/")+"/"+c.Repository, "/")
		if !hostMatches || strings.TrimSuffix(strings.TrimPrefix(r.Path, "/"), ".git") != expectedPath {
			return errors.New("configured PR host/repository does not match Git remote")
		}
	} else {
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) != 2 {
			return errors.New("unsupported Git remote URL")
		}
		host := parts[0]
		if i := strings.LastIndex(host, "@"); i >= 0 {
			host = host[i+1:]
		}
		if !strings.EqualFold(host, u.Hostname()) || strings.TrimSuffix(parts[1], ".git") != c.Repository {
			return errors.New("configured PR host/repository does not match SSH remote")
		}
	}
	return nil
}

func (c PRProviderConfig) PRNumber(raw string) (int, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return 0, err
	}
	base, _ := url.Parse(c.BaseURL)
	if base == nil || !strings.EqualFold(u.Host, base.Host) || u.Scheme != base.Scheme || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return 0, errors.New("PR URL host/scheme or format mismatch")
	}
	prefix := strings.TrimSuffix(base.Path, "/") + "/" + c.Repository + "/pulls/"
	if c.Kind == "command" && strings.HasPrefix(u.Path, strings.TrimSuffix(base.Path, "/")+"/"+c.Repository+"/pull/") {
		prefix = strings.TrimSuffix(base.Path, "/") + "/" + c.Repository + "/pull/"
	}
	if !strings.HasPrefix(u.Path, prefix) {
		return 0, errors.New("PR URL repository/path mismatch")
	}
	tail := strings.TrimPrefix(u.Path, prefix)
	number, err := strconv.Atoi(tail)
	if err != nil || number <= 0 || strconv.Itoa(number) != tail {
		return 0, errors.New("invalid PR number")
	}
	return number, nil
}

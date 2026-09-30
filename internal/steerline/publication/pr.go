package publication

import (
	"errors"
	"github.com/erranliu/SteerLine/internal/steerline/contracts"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var ErrPRIdentity = errors.New("PR identity mismatch")
var commitHash = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func validCommit(value string) bool { return commitHash.MatchString(value) }

type Config struct {
	Kind       string            `json:"kind"`
	BaseURL    string            `json:"base_url"`
	Repository string            `json:"repository"`
	TokenEnv   string            `json:"token_env"`
	Query      contracts.Command `json:"query"`
	Close      contracts.Command `json:"close,omitempty"`
}

func (c Config) ValidateRemote(remote string) error {
	if c.Kind != "gitea" && c.Kind != "command" {
		return errors.New("pr.kind must be gitea or command")
	}
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("invalid PR base_url; credentials must use token_env")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(c.Repository) || strings.Contains(c.Repository, "..") {
		return errors.New("PR repository must be owner/name")
	}
	if strings.Contains(remote, "://") {
		r, e := url.Parse(remote)
		if e != nil {
			return e
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
func (c Config) PRNumber(raw string) (int, error) {
	u, e := url.Parse(raw)
	if e != nil {
		return 0, e
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
	n, e := strconv.Atoi(tail)
	if e != nil || n <= 0 || strconv.Itoa(n) != tail {
		return 0, errors.New("invalid PR number")
	}
	return n, nil
}

// ValidateWorkerPRIdentity binds a provider-confirmed PR to the execution's
// branch. A URL or title supplied by a worker is only a clue until this check
// succeeds. The head SHA deliberately remains free to advance as the worker
// pushes more commits to the same PR.
func ValidateWorkerPRIdentity(info contracts.PRInfo, url, repository, baseBranch, executionBranch string) error {
	if url == "" || repository == "" || baseBranch == "" || executionBranch == "" ||
		info.URL != url || info.Repository != repository || info.BaseBranch != baseBranch ||
		info.HeadRepository != repository || info.HeadBranch != executionBranch ||
		!validCommit(info.HeadRevision) {
		return ErrPRIdentity
	}
	if info.State != "open" && info.State != "closed_unmerged" && info.State != "merged" {
		return ErrPRIdentity
	}
	return nil
}

package testprotocol

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// StagedBusiness is the existing executable fixture's deterministic model
// substitute. Its artifacts are real Git facts; each native input does only
// the action for its bound step. It is never reachable from production code.
func StagedBusiness(cwd, prompt, provider, remote string) (string, error) {
	git := func(args ...string) (string, error) {
		data, err := exec.Command("git", append([]string{"-C", cwd}, args...)...).CombinedOutput()
		if err != nil {
			return "", errors.New(string(data))
		}
		return strings.TrimSpace(string(data)), nil
	}
	switch {
	case strings.HasPrefix(strings.TrimSpace(prompt), "Current step: implement.execute"):
		if err := os.WriteFile(filepath.Join(cwd, "stage-result.txt"), []byte("staged fixture result\n"), 0600); err != nil {
			return "", err
		}
		if _, err := git("add", "--", "stage-result.txt"); err != nil {
			return "", err
		}
		if _, err := git("-c", "user.name=Stage Fixture", "-c", "user.email=stage@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "staged candidate"); err != nil {
			return "", err
		}
		return "Implemented, verified the fixture result and committed the local candidate; no publication performed.", nil
	case strings.HasPrefix(strings.TrimSpace(prompt), "Current step: implement.report"):
		head, err := git("rev-parse", "HEAD")
		if err != nil {
			return "", err
		}
		return `{"outcome":"done","outputs":{"candidate_sha":"` + head + `"},"summary":"source implementation completed"}`, nil
	case strings.HasPrefix(strings.TrimSpace(prompt), "Current step: publish.report."):
		// Report only the supplied reply; no Git or provider writes/checks.
		const marker = "\nFacts:\n"
		_, facts, ok := strings.Cut(prompt, marker)
		if !ok {
			return "", errors.New("publication report has no facts")
		}
		var input struct {
			Source struct {
				Reply string `json:"reply"`
			} `json:"source_execution"`
		}
		if err := json.NewDecoder(strings.NewReader(facts)).Decode(&input); err != nil {
			return "", err
		}
		url := strings.TrimPrefix(input.Source.Reply, "Published PR: ")
		if url == input.Source.Reply || url == "" {
			return "", errors.New("publication report has no original PR URL")
		}
		body, err := json.Marshal(map[string]any{"outcome": "done", "outputs": map[string]string{"pr_url": url}, "summary": "original publication reported"})
		return string(body), err
	case strings.HasPrefix(strings.TrimSpace(prompt), "Current step: publish."):
		head, err := git("rev-parse", "HEAD")
		if err != nil {
			return "", err
		}
		branch, err := git("symbolic-ref", "--short", "HEAD")
		if err != nil {
			return "", err
		}
		if _, err := git("push", remote, "HEAD:refs/heads/"+branch); err != nil {
			return "", err
		}
		body, _ := json.Marshal(map[string]any{"url": "https://github.com/owner/repo/pull/84", "repository": "owner/repo", "base_branch": "main", "head_repository": "owner/repo", "head_branch": branch, "head_revision": head, "state": "open", "merged": false})
		if err := os.WriteFile(provider, body, 0600); err != nil {
			return "", err
		}
		if os.Getenv("STEERLINE_STAGE_PUBLISH_PLAIN") == "1" {
			return "Published PR: https://github.com/owner/repo/pull/84", nil
		}
		return `{"outcome":"done","outputs":{"pr_url":"https://github.com/owner/repo/pull/84"},"summary":"candidate pushed and PR created"}`, nil
	}
	return "", errors.New("fixture received an unknown stage")
}

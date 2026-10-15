package feedback

import (
	"errors"
	"regexp"
	"strings"
)

// FinalSource binds an opaque canonical final to its original public Run.
type FinalSource struct {
	RunID       string `json:"run_id"`
	CandidateID string `json:"candidate_id"`
	Key         string `json:"key"`
}

// LiveDeliveryProof uses public Runner, Git and TaskDelivery evidence.
// It never requires native logs or token counters.
type LiveDeliveryProof struct {
	RunnerVersion      string        `json:"runner_version"`
	RunnerSHA256       string        `json:"runner_sha256"`
	RunID              string        `json:"run_id"`
	CandidateID        string        `json:"candidate_id"`
	FinalSources       []FinalSource `json:"final_sources"`
	ArtifactCommit     string        `json:"artifact_commit"`
	Artifact           string        `json:"artifact"`
	PRURL              string        `json:"pr_url"`
	Repository         string        `json:"repository"`
	Base               string        `json:"base"`
	Head               string        `json:"head"`
	HeadSHA            string        `json:"head_sha"`
	MergeSHA           string        `json:"merge_sha"`
	Merged             bool          `json:"merged"`
	DeliveryCandidate  string        `json:"delivery_candidate"`
	DeliveryPR         string        `json:"delivery_pr"`
	DeliveryCount      int           `json:"delivery_count"`
	SuccessorReady     bool          `json:"successor_ready"`
	SuccessorUnstarted bool          `json:"successor_unstarted"`
}

func (p LiveDeliveryProof) Validate() error {
	hex := func(s string, n int) bool {
		return len(s) == n && regexp.MustCompile(`^[0-9a-f]+$`).MatchString(s)
	}
	if p.RunnerVersion == "" || !hex(p.RunnerSHA256, 64) || p.RunID == "" || p.CandidateID == "" {
		return errors.New("live Runner identity missing")
	}
	if len(p.FinalSources) != 3 {
		return errors.New("three public stage finals required")
	}
	seen := map[string]bool{}
	for _, source := range p.FinalSources {
		if source.RunID != p.RunID || source.CandidateID != p.CandidateID || source.Key == "" || seen[source.Key] {
			return errors.New("public stage source missing or duplicated")
		}
		seen[source.Key] = true
	}
	if !hex(p.ArtifactCommit, 40) || p.ArtifactCommit != p.HeadSHA || p.Artifact != "real model delivery" {
		return errors.New("committed task artifact missing or changed")
	}
	if p.Repository != "erranliu/SteerlineTest" || !regexp.MustCompile(`^https://github.com/erranliu/SteerlineTest/pull/[1-9][0-9]*$`).MatchString(p.PRURL) || !strings.HasPrefix(p.Base, "codex/e2e/") || !strings.HasSuffix(p.Base, "/smoke.local-codex/base") || p.Head == "" || p.Head == "main" || p.Head == p.Base || !p.Merged || !hex(p.HeadSHA, 40) || !hex(p.MergeSHA, 40) {
		return errors.New("exact real merged PR evidence missing")
	}
	if p.DeliveryCount != 1 || p.DeliveryCandidate != p.CandidateID || p.DeliveryPR != p.PRURL || !p.SuccessorReady || !p.SuccessorUnstarted {
		return errors.New("unique TaskDelivery and successor evidence missing")
	}
	return nil
}

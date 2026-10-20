package feedback

import "errors"

// TestManagementProof belongs to the isolated testing scenario, which does not publish a PR.
type TestManagementProof struct {
	Package          string `json:"package"`
	RunnerSHA256     string `json:"runner_sha256"`
	GenerationRun    string `json:"generation_run"`
	GenerationSource string `json:"generation_source"`
	CandidateRun     string `json:"candidate_run"`
	CandidateSource  string `json:"candidate_source"`
	Commit           string `json:"commit"`
	PassReport       string `json:"pass_report"`
	FailReport       string `json:"fail_report"`
	BrowserPassed    bool   `json:"browser_passed"`
	BrowserFailed    bool   `json:"browser_failed"`
	Stopped          bool   `json:"stopped"`
	Cleanup          bool   `json:"cleanup_complete"`
}

func (p TestManagementProof) Validate() error {
	if p.Package == "" || len(p.RunnerSHA256) != 64 || p.GenerationRun == "" || p.GenerationSource == "" || p.CandidateRun == "" || p.CandidateSource == "" || p.CandidateRun == p.GenerationRun || len(p.Commit) != 40 || p.PassReport == "" || p.FailReport == "" || p.PassReport == p.FailReport || !p.BrowserPassed || !p.BrowserFailed || !p.Stopped || !p.Cleanup {
		return errors.New("test management proof is incomplete")
	}
	return nil
}

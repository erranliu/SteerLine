package contracts

import "time"

type PRInfo struct {
	URL            string     `json:"url"`
	Repository     string     `json:"repository"`
	BaseBranch     string     `json:"base_branch"`
	HeadRepository string     `json:"head_repository,omitempty"`
	HeadBranch     string     `json:"head_branch,omitempty"`
	HeadRevision   string     `json:"head_revision,omitempty"`
	State          string     `json:"state"`
	Merged         bool       `json:"merged"`
	MergedAt       *time.Time `json:"merged_at"`
	MergeCommit    *string    `json:"merge_commit"`
}

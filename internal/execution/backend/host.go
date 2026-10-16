package backend

type ProbeIdentity struct {
	Execution     string `json:"execution"`
	Turn          string `json:"turn"`
	Nonce         string `json:"nonce"`
	ProfileDigest string `json:"profile_digest"`
	Runtime       string `json:"runtime"`
	Terminal      string `json:"terminal"`
	Incarnation   string `json:"incarnation"`
	Worktree      string `json:"worktree"`
}

package contracts

// PRProviderConfig contains command descriptions and authentication references,
// never authentication values. publication owns its validation and execution.
type PRProviderConfig struct {
	Kind       string  `json:"kind"`
	BaseURL    string  `json:"base_url"`
	Repository string  `json:"repository"`
	TokenEnv   string  `json:"token_env"`
	Query      Command `json:"query"`
	Close      Command `json:"close,omitempty"`
}

type PublicationSettings struct {
	Provider           PRProviderConfig `json:"provider"`
	Create             Command          `json:"create"`
	Git                Command          `json:"git"`
	Remote             string           `json:"remote"`
	RemoteURL          string           `json:"remote_url"`
	BaseBranch         string           `json:"base_branch"`
	AuthenticationRefs []string         `json:"authentication_refs"`
}

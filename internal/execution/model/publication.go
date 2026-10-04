package model

import (
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

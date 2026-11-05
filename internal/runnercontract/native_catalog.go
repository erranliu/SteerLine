package runnercontract

import (
	"encoding/json"
	"time"
)

const NativeCatalogContract = "steerrunner-native-catalog/v1"

type NativeCatalogRequest struct {
	Kind       string           `json:"kind"`
	Adapter    string           `json:"adapter,omitempty"`
	Target     *ExecutionTarget `json:"target,omitempty"`
	Agent      string           `json:"agent,omitempty"`
	Executable string           `json:"executable,omitempty"`
}

type NativeCatalogSource struct {
	Version          string `json:"version"`
	ExecutableSHA256 string `json:"executable_sha256"`
	ResourceDigest   string `json:"resource_digest"`
	NativeVersion    string `json:"native_version,omitempty"`
	RuntimeID        string `json:"runtime_id,omitempty"`
	Protocol         int    `json:"protocol,omitempty"`
}
type NativeEnvironment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type NativeAgent struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
}
type NativeEffort struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type NativeModel struct {
	ID            string         `json:"id"`
	Label         string         `json:"label"`
	Provider      string         `json:"provider,omitempty"`
	Default       bool           `json:"is_default"`
	DefaultEffort string         `json:"default_effort,omitempty"`
	Efforts       []NativeEffort `json:"efforts"`
}
type NativeCatalogResult struct {
	Contract          string               `json:"contract"`
	Kind              string               `json:"kind"`
	Adapter           string               `json:"adapter,omitempty"`
	Target            *ExecutionTarget     `json:"target,omitempty"`
	Agent             string               `json:"agent,omitempty"`
	State             string               `json:"state"`
	Complete          bool                 `json:"complete"`
	ObservedAt        time.Time            `json:"observed_at"`
	ContextUnverified bool                 `json:"context_unverified"`
	Source            NativeCatalogSource  `json:"source"`
	Origin            string               `json:"origin"`
	NativeFetchedAt   *time.Time           `json:"native_fetched_at,omitempty"`
	ListingInProgress bool                 `json:"listing_in_progress"`
	Environments      *[]NativeEnvironment `json:"environments,omitempty"`
	Agents            *[]NativeAgent       `json:"agents,omitempty"`
	Models            *[]NativeModel       `json:"models,omitempty"`
}

type NativeCatalogCapability struct {
	Contract        string `json:"contract"`
	Command         string `json:"command"`
	QueryOnly       bool   `json:"query_only"`
	RequiresService *bool  `json:"requires_service"`
	Queries         []struct {
		Kind       string `json:"kind"`
		Adapter    string `json:"adapter,omitempty"`
		TargetKind string `json:"target_kind,omitempty"`
	} `json:"queries"`
	RequestSchema  json.RawMessage `json:"request_schema"`
	ResponseSchema json.RawMessage `json:"response_schema"`
	Limits         struct {
		InputBytes       int `json:"input_bytes"`
		OutputBytes      int `json:"output_bytes"`
		TimeoutMS        int `json:"timeout_ms"`
		CleanupTimeoutMS int `json:"cleanup_timeout_ms"`
	} `json:"limits"`
}

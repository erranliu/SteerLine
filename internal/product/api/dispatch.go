package api

type DispatchObservation struct {
	Mode                         string           `json:"mode"`
	Revision                     uint64           `json:"revision"`
	State                        string           `json:"state"`
	DefaultProfileID             string           `json:"default_profile_id"`
	DefaultProfileName           string           `json:"default_profile_name"`
	EnvironmentReadinessRequired bool             `json:"environment_readiness_required"`
	Blocker                      *DispatchBlocker `json:"blocker,omitempty"`
}

type DispatchBlocker struct {
	Category   string `json:"category"`
	Code       string `json:"code"`
	Diagnostic string `json:"diagnostic"`
}

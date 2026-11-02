package model

// AdmissionPolicy controls whether execution requires environment readiness.
// It is immutable after composition; its zero value requires readiness.
type AdmissionPolicy struct {
	allowUnready bool
}

func NewAdmissionPolicy(requireReadiness bool) AdmissionPolicy {
	return AdmissionPolicy{allowUnready: !requireReadiness}
}

func (p AdmissionPolicy) RequireReadiness() bool { return !p.allowUnready }

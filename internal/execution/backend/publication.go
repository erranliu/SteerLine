package backend

import (
	"errors"
)

// PublicationPolicy assigns commit, push and PR creation as one responsibility.
type PublicationPolicy struct {
	Publisher string `json:"publisher"`
}

const PublisherWorker = "worker"

func (p PublicationPolicy) Validate() error {
	if p.Publisher != PublisherWorker {
		return errors.New("publication_policy.publisher must explicitly be worker; host_service and profile delegation are retired")
	}
	return nil
}

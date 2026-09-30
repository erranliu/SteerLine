package execution

import "errors"

// PublicationPolicy assigns commit, push and PR creation as one responsibility.
type PublicationPolicy struct {
	Publisher string `json:"publisher"`
}

const PublisherWorker = "worker"
const PublisherHostService = "host_service"

func (p PublicationPolicy) Validate() error {
	if p.Publisher != PublisherWorker && p.Publisher != PublisherHostService {
		return errors.New("publication_policy.publisher must explicitly be worker or host_service")
	}
	return nil
}

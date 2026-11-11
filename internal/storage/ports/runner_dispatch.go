package ports

type RunnerDispatch struct {
	Project                  string `json:"project"`
	RequestID                string `json:"request_id"`
	ExpectedAuthorityVersion uint64 `json:"expected_authority_version"`
	Mode                     string `json:"mode"`
	RequestedBy              string `json:"requested_by"`
}

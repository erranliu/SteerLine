package backend

type ProbeReceipt struct {
	ProbeIdentity
	InputSHA256 string `json:"input_sha256"`
	GitHead     string `json:"git_head"`
	Read        bool   `json:"read"`
	Write       bool   `json:"write"`
	Delete      bool   `json:"delete"`
}

type ProbeToolEvidence struct {
	SourceIdentity string `json:"source_identity"`
	CallID         string `json:"call_id"`
	MessageID      string `json:"message_id"`
	Command        string `json:"command"`
	Marker         string `json:"marker"`
	Verified       bool   `json:"verified"`
	FailureCode    string `json:"failure_code,omitempty"`
}

type HostFact struct {
	Reason      string `json:"reason"`
	Confidence  string `json:"confidence"`
	Source      string `json:"source"`
	Detail      string `json:"detail,omitempty"`
	Runtime     string `json:"runtime"`
	Terminal    string `json:"terminal"`
	Incarnation string `json:"incarnation"`
	Turn        string `json:"turn,omitempty"`
}

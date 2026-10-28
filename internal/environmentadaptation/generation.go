package environmentadaptation

import (
	"encoding/json"
)

const GenerationSchema = `{"type":"object","additionalProperties":false,"required":["document","script","gaps"],"properties":{"document":{"type":"string","minLength":1,"maxLength":262144},"script":{"type":"string","minLength":1,"maxLength":262144},"gaps":{"type":"array","items":{"type":"string"}}}}`

// PreviousFacts is a deterministic prompt-only projection. Neither the record
// nor its report is changed; every unsuccessful reason remains verbatim.
func PreviousFacts(r Record, last Round) []byte {
	var report *Report
	diagnostic := r.Diagnostic
	if last.Report != nil {
		copy := *last.Report
		copy.Checks = append([]Check(nil), copy.Checks...)
		copy.L2 = append([]Item(nil), copy.L2...)
		for i := range copy.Checks {
			if copy.Checks[i].Status == "passed" {
				copy.Checks[i].Message = ""
			}
		}
		for i := range copy.L2 {
			if copy.L2[i].Status == "passed" {
				copy.L2[i].Message = ""
			}
		}
		if diagnostic == copy.Message {
			diagnostic = ""
		}
		report = &copy
	}
	body, _ := json.Marshal(struct {
		Profile         string  `json:"profile"`
		Activity        string  `json:"activity_id"`
		Round           int     `json:"round"`
		Diagnostic      string  `json:"diagnostic,omitempty"`
		RoundDiagnostic string  `json:"round_diagnostic,omitempty"`
		Report          *Report `json:"report"`
	}{r.Profile, r.ID, last.Number, diagnostic, last.Diagnostic, report})
	return body
}

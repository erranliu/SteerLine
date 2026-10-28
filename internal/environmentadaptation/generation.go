package environmentadaptation

import (
	"encoding/json"
)

const GenerationSchema = `{"type":"object","additionalProperties":false,"required":["document","script","gaps"],"properties":{"document":{"type":"string","minLength":1,"maxLength":262144},"script":{"type":"string","minLength":1,"maxLength":262144},"gaps":{"type":"array","items":{"type":"string"}}}}`

// PreviousFacts preserves the complete source report and diagnostics for an
// explicitly prepared fixed file. It never compacts or changes the record.
func PreviousFacts(r Record, last Round) []byte {
	body, _ := json.Marshal(struct {
		Profile         string  `json:"profile"`
		Activity        string  `json:"activity_id"`
		Round           int     `json:"round"`
		Diagnostic      string  `json:"diagnostic,omitempty"`
		RoundDiagnostic string  `json:"round_diagnostic,omitempty"`
		Report          *Report `json:"report"`
	}{r.Profile, r.ID, last.Number, r.Diagnostic, last.Diagnostic, last.Report})
	return body
}

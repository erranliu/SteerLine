package runnercontract

import (
	"bytes"
	"encoding/json"
)

// Preserve omitted versus explicitly supplied optional JSON. In particular,
// null must reach the published schema rather than disappearing as a nil
// pointer before validation. This codec supplies no workflow/model defaults.
func (w *Workflow) UnmarshalJSON(raw []byte) error {
	type fields Workflow
	var value fields
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return err
	}
	var presence map[string]json.RawMessage
	if err := json.Unmarshal(raw, &presence); err != nil {
		return err
	}
	*w = Workflow(value)
	_, w.promptOverridesPresent = presence["prompt_overrides"]
	_, w.auditModelPresent = presence["audit_model"]
	return nil
}
func (w Workflow) MarshalJSON() ([]byte, error) {
	value := map[string]any{"id": w.ID, "version": w.Version}
	if w.PromptOverrides != nil || w.promptOverridesPresent {
		value["prompt_overrides"] = w.PromptOverrides
	}
	if w.AuditModel != nil || w.auditModelPresent {
		value["audit_model"] = w.AuditModel
	}
	return json.Marshal(value)
}
func (m *ModelSettings) UnmarshalJSON(raw []byte) error {
	type fields ModelSettings
	var value fields
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return err
	}
	var presence map[string]json.RawMessage
	if err := json.Unmarshal(raw, &presence); err != nil {
		return err
	}
	*m = ModelSettings(value)
	effort, ok := presence["reasoning_effort"]
	m.effortPresent = ok
	m.effortNull = bytes.Equal(bytes.TrimSpace(effort), []byte("null"))
	return nil
}
func (m ModelSettings) MarshalJSON() ([]byte, error) {
	value := map[string]any{"model": m.Model}
	if m.ReasoningEffort != "" || m.effortPresent {
		value["reasoning_effort"] = m.ReasoningEffort
		if m.ReasoningEffort == "" && m.effortNull {
			value["reasoning_effort"] = nil
		}
	}
	return json.Marshal(value)
}

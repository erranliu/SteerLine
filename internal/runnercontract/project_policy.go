package runnercontract

import (
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
)

type ProjectPolicySlot struct {
	Slot string `json:"slot"`
	Path string `json:"path"`
}

var policySlotID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// ProjectPolicySlots reads only the published descriptor, never prompt text.
func ProjectPolicySlots(raw json.RawMessage) ([]ProjectPolicySlot, error) {
	var contract struct {
		Workflow struct {
			ProjectPolicies struct {
				Slots []ProjectPolicySlot `json:"slots"`
			} `json:"project_policies"`
		} `json:"workflow"`
	}
	invalid := errors.New("Runner 项目策略目录缺失或不完整")
	canonical, err := CanonicalJSON(raw)
	if err != nil || json.Unmarshal(canonical, &contract) != nil || len(contract.Workflow.ProjectPolicies.Slots) == 0 {
		return nil, invalid
	}
	slots := contract.Workflow.ProjectPolicies.Slots
	ids, paths := map[string]bool{}, map[string]bool{}
	for _, slot := range slots {
		if !policySlotID.MatchString(slot.Slot) || ids[slot.Slot] || slot.Path == "" || path.Clean(slot.Path) != slot.Path || strings.HasPrefix(slot.Path, "/") || slot.Path == ".." || strings.HasPrefix(slot.Path, "../") || strings.ContainsAny(slot.Path, "\\:\x00\r\n") || paths[slot.Path] {
			return nil, invalid
		}
		ids[slot.Slot] = true
		paths[slot.Path] = true
	}
	return slots, nil
}

package runnercontract

import "testing"

func TestProjectPolicySlotsPublishedOrderAndInvalidDescriptors(t *testing.T) {
	valid := `{"workflow":{"project_policies":{"slots":[{"slot":"custom","path":"rules/custom.md"},{"slot":"testing","path":"project/tests.md"}]}}}`
	slots, err := ProjectPolicySlots([]byte(valid))
	if err != nil || len(slots) != 2 || slots[0].Slot != "custom" || slots[0].Path != "rules/custom.md" {
		t.Fatal(slots, err)
	}
	for _, raw := range []string{`{}`, `{"workflow":null}`, `{"workflow":{"project_policies":{"slots":[]}}}`, `{"workflow":{"project_policies":{"slots":[{"slot":"x","path":"../escape"}]}}}`, `{"workflow":{"project_policies":{"slots":[{"slot":"x","path":"a"},{"slot":"x","path":"b"}]}}}`, `{"workflow":{"project_policies":{"slots":[{"slot":"x","path":"a"},{"slot":"y","path":"a"}]}}}`, `{"workflow":{"project_policies":{"slots":[{"slot":"x","path":"C:/x"}]}}}`} {
		if _, err := ProjectPolicySlots([]byte(raw)); err == nil {
			t.Fatal("invalid descriptor accepted", raw)
		}
	}
}

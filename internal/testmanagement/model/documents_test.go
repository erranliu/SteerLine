package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCallerPromptConvergenceCompactArrangement(t *testing.T) {
	full := Snapshot{Status: "ready", PlanPath: "task.plan.md", Commit: strings.Repeat("a", 40), Digest: strings.Repeat("b", 64), Definitions: []Definition{
		{ID: "z.last", Command: "original command", Note: strings.Repeat("长 note 必须保存在固定清单。", 100)},
		{ID: "a.first", Command: "second command", Note: strings.Repeat("独立约束", 100)},
	}}
	raw, _ := json.Marshal(full)
	original := string(raw)
	facts, err := TestingFacts(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(facts), `"tests":["z.last","a.first"]`) || !strings.Contains(string(facts), full.Commit) || !strings.Contains(string(facts), full.Digest) || strings.Contains(string(facts), full.Definitions[0].Note) || len(facts) >= len(raw) {
		t.Fatal("compact view changed IDs/order/identity or duplicated definitions")
	}
	if string(raw) != original {
		t.Fatal("snapshot mutated")
	}
	for _, invalid := range [][]byte{nil, []byte("{"), []byte(`{"status":"missing"}`)} {
		if _, err := TestingFacts(invalid); err == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
}

package backend

import (
	"bytes"
	"testing"
)

func TestQuestionRouteVersionIdentityDigestAndLimit(t *testing.T) {
	identity := QuestionRouteIdentity{ProjectID: "project", WorkflowRunID: "workflow", ExecutionID: "execution", AttemptID: "attempt", Generation: "generation", TurnID: "turn", QuestionID: "question"}
	payload := []byte(`{"request":"original"}`)
	route, err := NewQuestionRoute("example.impl", 1, identity, payload)
	if err != nil {
		t.Fatal(err)
	}
	payload[0] = '!'
	if !bytes.Equal(route.Payload, []byte(`{"request":"original"}`)) {
		t.Fatal("route retained a mutable payload alias")
	}
	if err := route.ValidateIdentity(identity); err != nil {
		t.Fatal(err)
	}
	wrong := identity
	wrong.TurnID = "old-turn"
	if err := route.ValidateIdentity(wrong); err == nil {
		t.Fatal("route accepted a different turn")
	}
	for name, mutate := range map[string]func(*ImplementationDataEnvelope){
		"version": func(r *ImplementationDataEnvelope) { r.Version++ },
		"purpose": func(r *ImplementationDataEnvelope) { r.Purpose = "other" },
		"digest":  func(r *ImplementationDataEnvelope) { r.Digest = "forged" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := route.Clone()
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("invalid route was accepted")
			}
		})
	}
	if _, err := NewQuestionRoute("example.impl", 1, identity, bytes.Repeat([]byte("x"), QuestionRouteMaxBytes+1)); err == nil {
		t.Fatal("oversized question route was accepted")
	}
}

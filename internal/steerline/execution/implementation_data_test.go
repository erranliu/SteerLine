package execution

import (
	"encoding/json"
	"testing"
)

func TestCD63CodecIdentityAndVersion(t *testing.T) {
	t.Run("profile envelope scope and deep copy", TestImplementationDataEnvelopeBindsScopeAndCopiesPayload)
	t.Run("resource envelope identity and bounds", TestImplementationDataEnvelopeRejectsBadIdentityVersionAndSize)
}

func TestImplementationDataEnvelopeBindsScopeAndCopiesPayload(t *testing.T) {
	payload := []byte(`{"settings":{"executable":"agent"}}`)
	envelope, err := NewImplementationDataEnvelope(ImplementationDataEnvelope{
		CodecVersion: 1, Purpose: ImplementationConfigPurpose, ImplementationKey: "worker",
		ProjectID: "project", ProfileID: "default", AcceptedConfigVersion: "accepted-v2",
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	payload[2] = 'X'
	if err := envelope.ValidateProfileIdentity("project", "default", "accepted-v2"); err != nil {
		t.Fatal(err)
	}
	if err := envelope.ValidateProfileIdentity("project", "default", "accepted-v1"); err == nil {
		t.Fatal("accepted version mismatch was accepted")
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var restored ImplementationDataEnvelope
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.ValidateProfileIdentity("project", "default", "accepted-v2"); err != nil {
		t.Fatal(err)
	}
	if string(restored.Payload) != `{"settings":{"executable":"agent"}}` {
		t.Fatalf("payload was not isolated from its caller: %s", restored.Payload)
	}
	clone := restored.Clone()
	clone.Payload[0] = '['
	if string(restored.Payload) != `{"settings":{"executable":"agent"}}` {
		t.Fatal("clone shares mutable payload bytes")
	}
}

func TestImplementationDataEnvelopeRejectsBadIdentityVersionAndSize(t *testing.T) {
	base := ImplementationDataEnvelope{
		CodecVersion: 1, Purpose: ImplementationResourcePurpose, ImplementationKey: "worker",
		ProjectID: "project", ExecutionID: "execution", AttemptID: "attempt", Generation: "generation",
		ConfigDigest: "config", Payload: []byte(`{"binding":"fixed"}`),
	}
	envelope, err := NewImplementationDataEnvelope(base, base.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := envelope.ValidateResourceIdentity("project", "execution", "attempt", "generation", "config"); err != nil {
		t.Fatal(err)
	}
	if err := envelope.ValidateResourceIdentity("project", "execution", "attempt", "other-generation", "config"); err == nil {
		t.Fatal("generation mismatch was accepted")
	}
	unknownVersion := envelope
	unknownVersion.Version++
	if err := unknownVersion.Validate(); err == nil {
		t.Fatal("unknown envelope version was accepted")
	}
	oversized := base
	oversized.Payload = make([]byte, ImplementationDataMaxBytes+1)
	oversized.Payload[0] = '{'
	oversized.Payload[len(oversized.Payload)-1] = '}'
	if _, err := NewImplementationDataEnvelope(oversized, oversized.Payload); err == nil {
		t.Fatal("oversized envelope payload was accepted")
	}
}

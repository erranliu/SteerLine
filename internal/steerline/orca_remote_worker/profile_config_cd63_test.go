package orca_remote_worker

import "testing"

func TestCD63CodecIdentityAndVersion(t *testing.T) {
	payload := []byte(`{"connection_id":"peer","environment_id":"env","repository_selector":"id:repo","coordinator_terminal":"term","poll_interval":"1s","capability_report_version":1,"remote_cancel_available":false}`)
	settings, err := DecodeProfileConfig(payload)
	if err != nil || settings.ConnectionID != "peer" || settings.EnvironmentID != "env" {
		t.Fatalf("valid remote profile payload did not decode: %+v / %v", settings, err)
	}
	if _, err := DecodeProfileConfig([]byte(`{"connection_id":"peer","environment_id":"env","repository_selector":"id:repo","coordinator_terminal":"term","poll_interval":"1s","capability_report_version":1}`)); err == nil {
		t.Fatal("remote cancellation default was inferred instead of requiring explicit evidence")
	}
	if _, err := DecodeProfileConfig([]byte(`{"connection_id":"peer","environment_id":"env","repository_selector":"id:repo","coordinator_terminal":"term","poll_interval":"1s","capability_report_version":1,"remote_cancel_available":true}`)); err == nil {
		t.Fatal("unsupported remote cancellation was accepted")
	}
}

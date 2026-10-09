package orcaworker

import "testing"

func TestCD63CodecIdentityAndVersion(t *testing.T) {
	payload := []byte(`{"executable":"orca.exe","args":[],"shell":"powershell.exe","wait_timeout":"15m"}`)
	settings, err := DecodeProfileConfig(payload)
	if err != nil || settings.Executable != "orca.exe" || settings.Shell != "powershell.exe" {
		t.Fatalf("valid Orca profile payload did not decode: %+v / %v", settings, err)
	}
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeProfileConfig([]byte(`{"executable":"orca.exe","shell":"cmd.exe","wait_timeout":"15m","remote_cancel_available":false}`)); err == nil {
		t.Fatal("unknown Orca profile field was accepted by the local codec")
	}
	if err := ValidateLocalTargetArgs([]string{"--environment=other"}); err == nil {
		t.Fatal("local target redirect was accepted")
	}
}

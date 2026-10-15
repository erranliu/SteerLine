package runnerclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestSteerRunnerReleasePinBlocksBeforeInvocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selected.exe")
	initial := []byte("selected release")
	sum := sha256.Sum256(initial)
	pin := hex.EncodeToString(sum[:])
	if err := os.WriteFile(path, initial, 0700); err != nil {
		t.Fatal(err)
	}
	if err := VerifyExecutable(path, pin); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"version", "capabilities", "service start", "submit"} {
		_, err := (Client{Executable: path, SHA256: pin}).Call(context.Background(), command, nil, nil)
		failure, ok := err.(*Failure)
		if !ok || failure.Unknown || failure.Exit != 4 {
			t.Fatalf("%s replacement was not rejected before process startup: %v", command, err)
		}
	}
}

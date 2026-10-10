package integration

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func require(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func mustJSON(value any) []byte {
	wire, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return wire
}

func AtomicWrite(path string, value []byte) error {
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, value, 0600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

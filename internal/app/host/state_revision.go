package host

import (
	"crypto/sha256"
	"encoding/hex"
)

func recordFingerprint(value any) string { return operationDigest(mustJSON(value)) }

func operationDigest(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

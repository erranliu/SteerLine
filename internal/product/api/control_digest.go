package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ControlRequestDigest binds retries to the original serialized request value.
func ControlRequestDigest(request ControlRequest) string {
	raw, _ := json.Marshal(request)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

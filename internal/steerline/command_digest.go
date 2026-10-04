package steerline

import (
	"crypto/sha256"
	"encoding/hex"
)

func digestCanonical(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

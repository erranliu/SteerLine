package orcatransport

import (
	"crypto/sha256"
	"encoding/hex"
)

const NativeReplySource = "orca.terminal.native-state-reply.v1"
const maxReplyBytes = 32 << 10

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

package execution

import (
	contracts "github.com/erranliu/SteerLine/internal/steerline/contracts"
)

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func conversationItemKey(e contracts.ConversationEvent, itemID string) string {
	if itemID == "" {
		return ""
	}
	e.ItemID = itemID
	sum := sha256.Sum256([]byte(e.ItemIdentity()))
	return hex.EncodeToString(sum[:])
}

func sourceEventFallback(e contracts.ConversationEvent, raw []byte) string {
	if e.SourceSeq != 0 {
		return fmt.Sprintf("seq:%d", e.SourceSeq)
	}
	if e.Evidence != nil {
		if start, ok := e.Evidence["byte_start"]; ok {
			if end, found := e.Evidence["byte_end"]; found {
				return "bytes:" + start + ":" + end
			}
		}
	}

	return "unidentified:" + newID()
}

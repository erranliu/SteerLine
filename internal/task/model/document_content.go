package model

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// ValidateDocumentContent is shared by task admission and execution's check of
// its pinned workspace inputs. It validates bytes only and performs no I/O.
func ValidateDocumentContent(ref FileReference, body []byte, acceptance bool) error {
	sum := sha256.Sum256(body)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), ref.SHA256) {
		return newError("contract_mismatch", "committed file SHA-256 does not match pinned reference: "+ref.Path)
	}
	if acceptance && !HasAcceptanceHeading(body) {
		return newError("contract_mismatch", "acceptance reference has no acceptance heading: "+ref.Path)
	}
	return nil
}

func IsAcceptanceReference(refs []FileReference, target FileReference) bool {
	for _, ref := range refs {
		if ref == target {
			return true
		}
	}
	return false
}

func HasAcceptanceHeading(body []byte) bool {
	return regexp.MustCompile(`(?im)^#{1,6}[ \t]+[^\r\n]*(验收|acceptance)[^\r\n]*$`).Match(body)
}
